package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"NotebookAI/internal/models"
	"NotebookAI/internal/observability"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/service"
	"NotebookAI/internal/worker/tasks"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	pdf "github.com/ledongthuc/pdf"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

type DocumentProcessor struct {
	llmService      service.LLMService
	documents       repository.DocumentRepository
	notebookService service.NotebookService
	parser          parser.ParserService
}

func NewDocumentProcessor(llmService service.LLMService, documents repository.DocumentRepository, notebookService service.NotebookService, parserSvc parser.ParserService) *DocumentProcessor {
	return &DocumentProcessor{
		llmService:      llmService,
		documents:       documents,
		notebookService: notebookService,
		parser:          parserSvc,
	}
}

func (p *DocumentProcessor) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(tasks.TypeProcessDocument, p.ProcessDocumentTask)
}

// ProcessDocumentTask 使用新的结构化解析链路处理文档
func (p *DocumentProcessor) ProcessDocumentTask(ctx context.Context, task *asynq.Task) error {
	var payload tasks.ProcessDocumentPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal process document task payload: %w", err)
	}

	totalTimer := observability.NewStopwatch()
	metrics := observability.NewProcessingMetrics(payload.TaskID, payload.DocumentID, payload.UserID, payload.FileName)

	zap.L().Info("start processing document task (structured parser)",
		zap.String("task_id", payload.TaskID),
		zap.String("user_id", payload.UserID),
		zap.String("document_id", payload.DocumentID),
		zap.String("file_path", payload.FilePath),
	)

	// ========== 阶段1: 结构化文档解析 ==========
	extractTimer := observability.NewStopwatch()

	parseResult, parentChunks, childChunks, err := p.parseAndBuild(ctx, &payload)
	metrics.ExtractLatencyMs = extractTimer.ElapsedMs()

	if err != nil {
		metrics.Status = "failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogDocumentProcessing(metrics)
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("parse document: %w", err)
	}

	zap.L().Info("document parsed successfully",
		zap.String("document_id", payload.DocumentID),
		zap.Int("total_pages", parseResult.TotalPages),
		zap.Int("blocks", len(parseResult.Blocks)),
		zap.Int("tables", parseResult.TableCount),
		zap.Bool("has_ocr", parseResult.HasOCR),
		zap.Int64("extract_latency_ms", metrics.ExtractLatencyMs),
	)

	// ========== 阶段2: 向量化入库（父子 chunk） ==========
	indexTimer := observability.NewStopwatch()

	if err := p.llmService.DeleteDocumentChunks(ctx, payload.UserID, payload.DocumentID); err != nil {
		metrics.Status = "failed"
		metrics.ErrorMsg = err.Error()
		metrics.IndexLatencyMs = indexTimer.ElapsedMs()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogDocumentProcessing(metrics)
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// 合并 parent + child chunks 并转为 schema.Document
	allChunks := append(parentChunks, childChunks...)
	docs := make([]schema.Document, 0, len(allChunks))
	for idx := range allChunks {
		chunk := allChunks[idx]
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		
		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    chunk.ToMetadata(),
		})
	}

	if len(docs) == 0 {
		err := fmt.Errorf("document contains no indexable chunks after structured parsing")
		metrics.Status = "failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogDocumentProcessing(metrics)
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return err
	}

	metrics.ChunkCount = len(docs)

	if err := p.llmService.IndexDocuments(ctx, docs); err != nil {
		metrics.Status = "failed"
		metrics.ErrorMsg = err.Error()
		metrics.IndexLatencyMs = indexTimer.ElapsedMs()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogDocumentProcessing(metrics)
		_ = p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusFailed, 0, err.Error())
		return fmt.Errorf("index documents into vector store: %w", err)
	}
	metrics.IndexLatencyMs = indexTimer.ElapsedMs()

	if err := p.documents.UpdateProcessingResult(ctx, payload.DocumentID, models.DocumentStatusCompleted, len(docs), ""); err != nil {
		return fmt.Errorf("update document status: %w", err)
	}

	metrics.Status = "success"
	metrics.TotalLatencyMs = totalTimer.ElapsedMs()
	observability.LogDocumentProcessing(metrics)

	// ========== 阶段3: 异步生成 Document Guide ==========
	p.triggerGuideGeneration(ctx, payload, parseResult.RawText)

	zap.L().Info("document task processed with structured parsing",
		zap.String("task_id", payload.TaskID),
		zap.String("document_id", payload.DocumentID),
		zap.Int("parent_chunks", len(parentChunks)),
		zap.Int("child_chunks", len(childChunks)),
		zap.Int("total_indexed_chunks", len(docs)),
		zap.Int64("total_latency_ms", metrics.TotalLatencyMs),
	)

	return nil
}

// parseAndBuild 执行结构化解析和分块构建
func (p *DocumentProcessor) parseAndBuild(ctx context.Context, payload *tasks.ProcessDocumentPayload) (*parser.ParseResult, []*parser.Chunk, []*parser.Chunk, error) {
	if p.parser == nil {
		return p.fallbackParse(payload.FilePath)
	}

	// 分步调用接口方法（ParseAndBuild 是便捷方法，不在接口上）
	result, err := p.parser.ParseDocument(ctx, payload.FilePath, payload.UserID, payload.DocumentID)
	if err != nil {
		return nil, nil, nil, err
	}

	parents, children := p.parser.BuildChunks(result, payload.UserID, payload.DocumentID)

	// 设置 document_id 和 user_id 到所有 chunks
	for _, c := range parents {
		c.DocumentID = payload.DocumentID
		c.UserID = payload.UserID
	}
	for _, c := range children {
		c.DocumentID = payload.DocumentID
		c.UserID = payload.UserID
	}

	return result, parents, children, nil
}

// fallbackParse 当 parser 服务不可用时降级为旧的纯文本提取 + 简单分块
func (p *DocumentProcessor) fallbackParse(filePath string) (*parser.ParseResult, []*parser.Chunk, []*parser.Chunk, error) {
	zap.L().Warn("parser not available, falling back to legacy text extraction")

	content, err := extractPDFTextLegacy(filePath)
	if err != nil {
		return nil, nil, nil, err
	}

	result := &parser.ParseResult{
		RawText:     content,
		ParseErrors: []string{"using fallback legacy text extraction"},
	}

	// 创建降级的单一父块+子块
	chunkID := uuid.NewString()
	fallbackParent := &parser.Chunk{
		ID:         chunkID,
		Content:    content,
		PageNum:    1,
		ChunkIndex: 0,
		ChunkType:  parser.BlockTypeText,
		Metadata:   map[string]any{"chunk_role": "parent", "fallback": "true"},
	}

	fallbackChild := &parser.Chunk{
		ID:         uuid.NewString(),
		ParentID:   chunkID,
		Content:    content,
		PageNum:    1,
		ChunkIndex: 0,
		ChunkType:  parser.BlockTypeText,
		Metadata:   map[string]any{"chunk_role": "child", "fallback": "true"},
	}

	parents := []*parser.Chunk{fallbackParent}
	children := []*parser.Chunk{fallbackChild}

	return result, parents, children, nil
}

// triggerGuideGeneration 异步触发 Guide（摘要/FAQ/关键点）生成
func (p *DocumentProcessor) triggerGuideGeneration(ctx context.Context, payload tasks.ProcessDocumentPayload, rawText string) {
	doc, docErr := p.documents.GetByIDForWorker(ctx, payload.DocumentID)
	if docErr != nil {
		zap.L().Warn("failed to get document for guide generation",
			zap.String("document_id", payload.DocumentID), zap.Error(docErr))
		return
	}

	if doc.NotebookID == "" || p.notebookService == nil {
		return
	}

	go func() {
		guideCtx := context.Background()
		if err := p.notebookService.ProcessDocumentWithGuide(
			guideCtx, payload.UserID, doc.NotebookID, payload.DocumentID, rawText,
		); err != nil {
			zap.L().Error("failed to generate document guide",
				zap.String("document_id", payload.DocumentID),
				zap.String("notebook_id", doc.NotebookID),
				zap.Error(err),
			)
		} else {
			zap.L().Info("document guide generated successfully",
				zap.String("document_id", payload.DocumentID),
				zap.String("notebook_id", doc.NotebookID),
			)
		}
	}()
}

// extractPDFTextLegacy 旧版 PDF 纯文本提取（保留用于降级）
func extractPDFTextLegacy(filePath string) (string, error) {
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

		buf.WriteString(pageText)
		buf.WriteString("\n")
	}

	content := strings.TrimSpace(buf.String())
	if content == "" {
		return "", errors.New("pdf content is empty")
	}
	return content, nil
}
