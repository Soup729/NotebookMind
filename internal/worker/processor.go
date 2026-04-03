package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

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
}

func NewDocumentProcessor(llmService service.LLMService) *DocumentProcessor {
	return &DocumentProcessor{llmService: llmService}
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
		zap.String("file_path", payload.FilePath),
	)

	content, err := extractPDFText(payload.FilePath)
	if err != nil {
		return fmt.Errorf("extract pdf text: %w", err)
	}

	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	chunks, err := splitter.SplitText(content)
	if err != nil {
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
		return fmt.Errorf("document contains no indexable chunks")
	}

	if err := p.llmService.IndexDocuments(ctx, docs); err != nil {
		return fmt.Errorf("index documents into milvus: %w", err)
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
