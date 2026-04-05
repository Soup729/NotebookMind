package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
	"enterprise-pdf-ai/internal/service"
	"enterprise-pdf-ai/internal/worker/tasks"
	"github.com/hibiken/asynq"
	pdf "github.com/ledongthuc/pdf"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
	"go.uber.org/zap"
)

type DocumentProcessor struct {
	llmService service.LLMService
	documents  repository.DocumentRepository
}

func NewDocumentProcessor(llmService service.LLMService, documents repository.DocumentRepository) *DocumentProcessor {
	return &DocumentProcessor{
		llmService: llmService,
		documents:  documents,
	}
}

func (p *DocumentProcessor) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(tasks.TypeProcessDocument, p.ProcessDocumentTask)
}

func (p *DocumentProcessor) ProcessDocumentTask(ctx context.Context, task *asynq.Task) error {
	var payload tasks.ProcessDocumentPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal process document task payload: %w", err)
	}

	zap.L().Info("start processing document task",
		zap.String("task_id", payload.TaskID),
		zap.String("user_id", payload.UserID),
		zap.String("document_id", payload.DocumentID),
	)

	content, err := extractPDFText(payload.FilePath)
	if err != nil {
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("extract pdf text: %w", err)
	}

	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	chunks, err := splitter.SplitText(content)
	if err != nil {
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("split content into chunks: %w", err)
	}

	docs := make([]schema.Document, 0, len(chunks))
	for index, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}

		docs = append(docs, schema.Document{
			PageContent: trimmed,
			Metadata: map[string]any{
				"user_id":     payload.UserID,
				"task_id":     payload.TaskID,
				"document_id": payload.DocumentID,
				"file_name":   payload.FileName,
				"chunk_index": index,
			},
		})
	}

	if len(docs) == 0 {
		err := fmt.Errorf("document contains no indexable chunks")
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return err
	}

	if err := p.llmService.DeleteDocumentChunks(ctx, payload.UserID, payload.DocumentID); err != nil {
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("delete old chunks: %w", err)
	}

	if err := p.llmService.IndexDocuments(ctx, docs); err != nil {
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("index documents into vector store: %w", err)
	}

	if err := p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusCompleted, len(docs), ""); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	zap.L().Info("document task processed",
		zap.String("task_id", payload.TaskID),
		zap.String("document_id", payload.DocumentID),
		zap.Int("chunks", len(docs)),
	)

	return nil
}

func extractPDFText(filePath string) (string, error) {
	f, reader, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open pdf file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	pageCount := reader.NumPage()
	for i := 1; i <= pageCount; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		pageText, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("read page %d text: %w", i, err)
		}

		if _, err = io.Copy(&buf, strings.NewReader(pageText)); err != nil {
			return "", fmt.Errorf("copy page %d text: %w", i, err)
		}
		buf.WriteString("\n")
	}

	content := strings.TrimSpace(buf.String())
	if content == "" {
		return "", errors.New("pdf content is empty")
	}
	return content, nil
}
