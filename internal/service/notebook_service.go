package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/models"
	"NotebookAI/internal/observability"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/repository"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/textsplitter"
	"go.uber.org/zap"
)

// NotebookService defines operations for NotebookLM notebooks
type NotebookService interface {
	// Notebook CRUD
	CreateNotebook(ctx context.Context, userID, title, description string) (*models.Notebook, error)
	GetNotebook(ctx context.Context, userID, notebookID string) (*models.Notebook, error)
	ListNotebooks(ctx context.Context, userID string) ([]models.Notebook, error)
	UpdateNotebook(ctx context.Context, notebook *models.Notebook) error
	DeleteNotebook(ctx context.Context, userID, notebookID string) error

	// Document management within notebook
	AddDocumentToNotebook(ctx context.Context, notebookID, documentID string) error
	RemoveDocumentFromNotebook(ctx context.Context, notebookID, documentID string) error
	ListNotebookDocuments(ctx context.Context, notebookID string) ([]models.Document, error)

	// Document guide (summary & FAQ)
	GetDocumentGuide(ctx context.Context, documentID string) (*models.DocumentGuide, error)

	// Process document and generate guide asynchronously
	ProcessDocumentWithGuide(ctx context.Context, userID, notebookID, documentID string, content string) error
	IndexParsedChunks(ctx context.Context, userID, notebookID, documentID string, chunks []*parser.Chunk) error
}

// notebookService implements NotebookService
type notebookService struct {
	notebookRepo repository.NotebookRepository
	vectorStore  repository.NotebookVectorStore
	embedder     embeddings.Embedder
	llm          llms.Model
	cfg          *configs.LLMConfig
	bm25Index    *BM25Index
}

// IndexParsedChunks stores structured parser chunks in the notebook retrieval index.
func (s *notebookService) IndexParsedChunks(ctx context.Context, userID, notebookID, documentID string, chunks []*parser.Chunk) error {
	notebookChunks := make([]repository.NotebookChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}
		notebookChunks = append(notebookChunks, parserChunkToNotebookChunk(notebookID, documentID, chunk, content))
	}
	if len(notebookChunks) == 0 {
		return fmt.Errorf("no structured chunks to index")
	}

	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteByDocument(ctx, documentID); err != nil {
			return fmt.Errorf("delete old notebook chunks: %w", err)
		}
	}
	if s.bm25Index != nil {
		s.bm25Index.RemoveByDocument(documentID)
	}

	vectors, err := s.embedNotebookChunks(ctx, notebookChunks)
	if err != nil {
		return err
	}
	if s.vectorStore != nil {
		if err := s.vectorStore.InsertChunks(ctx, notebookChunks, vectors); err != nil {
			return fmt.Errorf("insert structured notebook chunks: %w", err)
		}
	}
	if s.bm25Index != nil {
		for _, chunk := range notebookChunks {
			chunkID := fmt.Sprintf("nb_%s_%d", documentID, chunk.ChunkIndex)
			s.bm25Index.IndexDocumentWithMetadata(chunkID, documentID, chunk.Content, notebookChunkMetadata(chunk))
		}
	}

	go s.generateGuideAsync(context.Background(), notebookID, documentID, notebookChunks)
	return nil
}

func parserChunkToNotebookChunk(notebookID, documentID string, chunk *parser.Chunk, content string) repository.NotebookChunk {
	pageNumber := int64(chunk.PageNum - 1)
	if pageNumber < 0 {
		pageNumber = 0
	}
	return repository.NotebookChunk{
		NotebookID:  notebookID,
		DocumentID:  documentID,
		PageNumber:  pageNumber,
		ChunkIndex:  int64(chunk.ChunkIndex),
		Content:     content,
		ChunkType:   string(chunk.ChunkType),
		ChunkRole:   metadataAnyString(chunk.Metadata, "chunk_role"),
		ParentID:    chunk.ParentID,
		SectionPath: mustJSON(chunk.SectionPath),
		BBox:        bboxJSON(chunk.BBox),
	}
}

func metadataAnyString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key]; ok && value != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	return ""
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	if string(data) == "null" {
		return ""
	}
	return string(data)
}

func bboxJSON(b parser.BoundingBox) string {
	if b == (parser.BoundingBox{}) {
		return ""
	}
	return fmt.Sprintf("[%g,%g,%g,%g]", b.X0, b.Y0, b.X1, b.Y1)
}

func (s *notebookService) embedNotebookChunks(ctx context.Context, chunks []repository.NotebookChunk) ([][]float32, error) {
	vectors := make([][]float32, len(chunks))
	var embedErr error
	var wg sync.WaitGroup
	embedChan := make(chan struct {
		index  int
		vector []float32
		err    error
	}, len(chunks))

	for i := range chunks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			vector, err := s.embedder.EmbedQuery(ctx, chunks[idx].Content)
			embedChan <- struct {
				index  int
				vector []float32
				err    error
			}{idx, vector, err}
		}(i)
	}

	go func() {
		wg.Wait()
		close(embedChan)
	}()

	for item := range embedChan {
		if item.err != nil {
			embedErr = item.err
			continue
		}
		vectors[item.index] = item.vector
	}
	if embedErr != nil {
		return nil, fmt.Errorf("embed structured notebook chunks: %w", embedErr)
	}
	return vectors, nil
}

// NewNotebookService creates a new NotebookService
func NewNotebookService(
	notebookRepo repository.NotebookRepository,
	vectorStore repository.NotebookVectorStore,
	embedder embeddings.Embedder,
	llm llms.Model,
	cfg *configs.LLMConfig,
	bm25Index *BM25Index,
) NotebookService {
	return &notebookService{
		notebookRepo: notebookRepo,
		vectorStore:  vectorStore,
		embedder:     embedder,
		llm:          llm,
		cfg:          cfg,
		bm25Index:    bm25Index,
	}
}

// Notebook CRUD operations

func (s *notebookService) CreateNotebook(ctx context.Context, userID, title, description string) (*models.Notebook, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled Notebook"
	}

	notebook := &models.Notebook{
		ID:          uuid.NewString(),
		UserID:      userID,
		Title:       title,
		Description: strings.TrimSpace(description),
		Status:      models.NotebookStatusActive,
		DocumentCnt: 0,
	}

	if err := s.notebookRepo.Create(ctx, notebook); err != nil {
		return nil, fmt.Errorf("create notebook: %w", err)
	}

	zap.L().Info("created notebook",
		zap.String("notebook_id", notebook.ID),
		zap.String("user_id", userID),
	)

	return notebook, nil
}

func (s *notebookService) GetNotebook(ctx context.Context, userID, notebookID string) (*models.Notebook, error) {
	notebook, err := s.notebookRepo.GetByID(ctx, userID, notebookID)
	if err != nil {
		return nil, fmt.Errorf("get notebook: %w", err)
	}
	return notebook, nil
}

func (s *notebookService) ListNotebooks(ctx context.Context, userID string) ([]models.Notebook, error) {
	notebooks, err := s.notebookRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list notebooks: %w", err)
	}
	return notebooks, nil
}

func (s *notebookService) UpdateNotebook(ctx context.Context, notebook *models.Notebook) error {
	if err := s.notebookRepo.Update(ctx, notebook); err != nil {
		return fmt.Errorf("update notebook: %w", err)
	}
	return nil
}

func (s *notebookService) DeleteNotebook(ctx context.Context, userID, notebookID string) error {
	// Delete vector chunks from Milvus
	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteByNotebook(ctx, notebookID); err != nil {
			zap.L().Error("delete notebook vectors failed", zap.Error(err))
			// Continue with deletion even if vector cleanup fails
		}
	}

	if err := s.notebookRepo.Delete(ctx, userID, notebookID); err != nil {
		return fmt.Errorf("delete notebook: %w", err)
	}

	zap.L().Info("deleted notebook", zap.String("notebook_id", notebookID))
	return nil
}

// Document management

func (s *notebookService) AddDocumentToNotebook(ctx context.Context, notebookID, documentID string) error {
	if err := s.notebookRepo.AddDocument(ctx, notebookID, documentID); err != nil {
		return fmt.Errorf("add document to notebook: %w", err)
	}
	return nil
}

func (s *notebookService) RemoveDocumentFromNotebook(ctx context.Context, notebookID, documentID string) error {
	// 清理BM25索引
	if s.bm25Index != nil {
		s.bm25Index.RemoveByDocument(documentID)
	}
	// 清理向量存储
	if s.vectorStore != nil {
		if err := s.vectorStore.DeleteByDocument(ctx, documentID); err != nil {
			zap.L().Warn("delete document vectors failed", zap.String("document_id", documentID), zap.Error(err))
		}
	}
	if err := s.notebookRepo.RemoveDocument(ctx, notebookID, documentID); err != nil {
		return fmt.Errorf("remove document from notebook: %w", err)
	}
	return nil
}

func (s *notebookService) ListNotebookDocuments(ctx context.Context, notebookID string) ([]models.Document, error) {
	documents, err := s.notebookRepo.ListDocuments(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("list notebook documents: %w", err)
	}
	return documents, nil
}

func (s *notebookService) GetDocumentGuide(ctx context.Context, documentID string) (*models.DocumentGuide, error) {
	guide, err := s.notebookRepo.GetGuide(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("get document guide: %w", err)
	}
	return guide, nil
}

// ProcessDocumentWithGuide processes document content, generates chunks, vectors, and creates summary/FAQ
func (s *notebookService) ProcessDocumentWithGuide(ctx context.Context, userID, notebookID, documentID string, content string) error {
	// 初始化处理计时器
	processTimer := observability.NewStopwatch()

	// Step 1: Split content into chunks with page numbers
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithChunkSize(1000),
		textsplitter.WithChunkOverlap(200),
	)

	splitTimer := observability.NewStopwatch()
	chunks, err := splitter.SplitText(content)
	splitLatency := splitTimer.ElapsedMs()
	if err != nil {
		zap.L().Error("document processing: split failed",
			zap.String("document_id", documentID),
			zap.String("notebook_id", notebookID),
			zap.Int64("split_latency_ms", splitLatency),
			zap.Error(err),
		)
		return s.failGuide(ctx, documentID, fmt.Errorf("split content: %w", err))
	}

	if len(chunks) == 0 {
		return s.failGuide(ctx, documentID, fmt.Errorf("no chunks generated"))
	}

	// Step 2: Create notebook chunks with metadata
	notebookChunks := make([]repository.NotebookChunk, 0, len(chunks))
	for idx, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		notebookChunks = append(notebookChunks, repository.NotebookChunk{
			NotebookID: notebookID,
			DocumentID: documentID,
			PageNumber: int64(idx / 5), // Approximate page number based on chunk index
			ChunkIndex: int64(idx),
			Content:    trimmed,
		})
	}

	// Step 3: Embed chunks in parallel using goroutines
	embedTimer := observability.NewStopwatch()
	vectors := make([][]float32, len(notebookChunks))
	var embedErr error
	var wg sync.WaitGroup
	embedChan := make(chan struct {
		index  int
		vector []float32
		err    error
	}, len(notebookChunks))

	for i := range notebookChunks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			vector, err := s.embedder.EmbedQuery(ctx, notebookChunks[idx].Content)
			embedChan <- struct {
				index  int
				vector []float32
				err    error
			}{idx, vector, err}
		}(i)
	}

	go func() {
		wg.Wait()
		close(embedChan)
	}()

	for item := range embedChan {
		if item.err != nil {
			embedErr = item.err
			continue
		}
		vectors[item.index] = item.vector
	}

	if embedErr != nil {
		zap.L().Error("document processing: embed failed",
			zap.String("document_id", documentID),
			zap.Int64("embed_latency_ms", embedTimer.ElapsedMs()),
			zap.Error(embedErr),
		)
		return s.failGuide(ctx, documentID, fmt.Errorf("embed chunks: %w", embedErr))
	}

	zap.L().Info("document processing: embed completed",
		zap.String("document_id", documentID),
		zap.Int64("embed_latency_ms", embedTimer.ElapsedMs()),
		zap.Int("chunk_count", len(notebookChunks)),
	)

	// 记录 Embed LLM 调用指标
	observability.LogLLMCall("embed", 0, 0, len(notebookChunks)*100, embedTimer.ElapsedMs(), "text-embedding-3-small", nil)

	// Step 4: Insert vectors into Milvus
	if s.vectorStore != nil {
		if err := s.vectorStore.InsertChunks(ctx, notebookChunks, vectors); err != nil {
			return s.failGuide(ctx, documentID, fmt.Errorf("insert vectors: %w", err))
		}
	}

	// Step 4.5: 同步写入BM25索引
	if s.bm25Index != nil {
		for _, chunk := range notebookChunks {
			chunkID := fmt.Sprintf("nb_%s_%d", documentID, chunk.ChunkIndex)
			s.bm25Index.IndexDocument(chunkID, documentID, chunk.Content)
		}
		zap.L().Debug("indexed notebook chunks into BM25",
			zap.String("document_id", documentID),
			zap.Int("chunk_count", len(notebookChunks)),
		)
	}

	// Step 5: Generate summary and FAQ asynchronously using goroutine
	go s.generateGuideAsync(context.Background(), notebookID, documentID, notebookChunks)

	zap.L().Info("document processed with guide generation started",
		zap.String("document_id", documentID),
		zap.String("notebook_id", notebookID),
		zap.Int("chunks", len(notebookChunks)),
		zap.Int64("total_process_ms", processTimer.ElapsedMs()),
		zap.Int64("split_latency_ms", splitLatency),
	)

	return nil
}

// generateGuideAsync generates summary and FAQ for document in background
func (s *notebookService) generateGuideAsync(ctx context.Context, notebookID, documentID string, chunks []repository.NotebookChunk) {
	guideTimer := observability.NewStopwatch()
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("generateGuideAsync panicked", zap.Any("error", r))
			_ = s.failGuide(ctx, documentID, fmt.Errorf("panic during guide generation"))
		}
	}()

	// Combine all chunk content for analysis
	var fullContent strings.Builder
	for _, chunk := range chunks {
		fullContent.WriteString(chunk.Content)
		fullContent.WriteString("\n\n")
	}
	content := fullContent.String()

	// Truncate if too long (LLM context limit)
	if len(content) > 15000 {
		content = content[:15000]
	}

	// Generate Summary
	summaryTimer := observability.NewStopwatch()
	summaryPrompt := fmt.Sprintf(`You are an AI assistant that generates concise summaries of documents.

Analyze the following document content and provide:
1. A comprehensive summary (2-3 paragraphs)
2. Key topics covered (bullet points)

Document Content:
---
%s
---

Summary:`, content)

	summary, err := s.generateLLMResponse(ctx, summaryPrompt)
	summaryLatency := summaryTimer.ElapsedMs()
	if err != nil {
		zap.L().Error("generate summary failed", zap.Error(err), zap.Int64("latency_ms", summaryLatency))
		observability.LogLLMCall("guide_summary", estimateTokens(summaryPrompt), estimateTokens(summary), estimateTokens(summaryPrompt)+estimateTokens(summary), summaryLatency, "gpt-4o-mini", err)
		summary = "Summary generation failed"
	} else {
		observability.LogLLMCall("guide_summary", estimateTokens(summaryPrompt), estimateTokens(summary), estimateTokens(summaryPrompt)+estimateTokens(summary), summaryLatency, "gpt-4o-mini", nil)
	}

	// Generate FAQ
	faqTimer := observability.NewStopwatch()
	faqPrompt := fmt.Sprintf(`You are an AI assistant that generates FAQ sections for documents.

Based on the following document content, generate 5 frequently asked questions and their answers.
Format as JSON array: [{"question": "...", "answer": "..."}]

Document Content:
---
%s
---

FAQ:`, content)

	faqJSON, err := s.generateLLMResponse(ctx, faqPrompt)
	faqLatency := faqTimer.ElapsedMs()
	if err != nil {
		zap.L().Error("generate FAQ failed", zap.Error(err), zap.Int64("latency_ms", faqLatency))
		observability.LogLLMCall("guide_faq", estimateTokens(faqPrompt), 0, estimateTokens(faqPrompt), faqLatency, "gpt-4o-mini", err)
		faqJSON = "[]"
	} else {
		observability.LogLLMCall("guide_faq", estimateTokens(faqPrompt), estimateTokens(faqJSON), estimateTokens(faqPrompt)+estimateTokens(faqJSON), faqLatency, "gpt-4o-mini", nil)
	}

	// Generate Key Points
	kpTimer := observability.NewStopwatch()
	keyPointsPrompt := fmt.Sprintf(`You are an AI assistant that extracts key points from documents.

Extract 5-7 key takeaways from the document. Format as JSON array of strings.

Document Content:
---
%s
---

Key Points:`, content)

	keyPoints, err := s.generateLLMResponse(ctx, keyPointsPrompt)
	kpLatency := kpTimer.ElapsedMs()
	if err != nil {
		zap.L().Error("generate key points failed", zap.Error(err), zap.Int64("latency_ms", kpLatency))
		observability.LogLLMCall("guide_keypoints", estimateTokens(keyPointsPrompt), 0, estimateTokens(keyPointsPrompt), kpLatency, "gpt-4o-mini", err)
		keyPoints = "[]"
	} else {
		observability.LogLLMCall("guide_keypoints", estimateTokens(keyPointsPrompt), estimateTokens(keyPoints), estimateTokens(keyPointsPrompt)+estimateTokens(keyPoints), kpLatency, "gpt-4o-mini", nil)
	}

	// 记录总 Guide 生成耗时
	zap.L().Info("guide generation completed",
		zap.String("document_id", documentID),
		zap.Int64("total_guide_latency_ms", guideTimer.ElapsedMs()),
	)

	// Save guide to database
	guide := &models.DocumentGuide{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		Summary:    summary,
		FaqJSON:    faqJSON,
		KeyPoints:  keyPoints,
		Status:     models.GuideStatusCompleted,
	}

	if err := s.notebookRepo.UpsertGuide(ctx, guide); err != nil {
		zap.L().Error("save document guide failed", zap.Error(err))
		return
	}

	zap.L().Info("document guide generated",
		zap.String("document_id", documentID),
		zap.String("summary_len", fmt.Sprintf("%d", len(summary))),
	)
}

// generateLLMResponse calls LLM with the given prompt
func (s *notebookService) generateLLMResponse(ctx context.Context, prompt string) (string, error) {
	if s.llm == nil {
		return "", fmt.Errorf("LLM not initialized")
	}

	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// failGuide marks guide generation as failed
func (s *notebookService) failGuide(ctx context.Context, documentID string, err error) error {
	guide := &models.DocumentGuide{
		ID:         uuid.NewString(),
		DocumentID: documentID,
		Status:     models.GuideStatusFailed,
		ErrorMsg:   err.Error(),
	}

	if writeErr := s.notebookRepo.UpsertGuide(ctx, guide); writeErr != nil {
		zap.L().Error("failed to write guide error", zap.Error(writeErr))
	}

	return err
}

// NewNotebookLLMService creates LLM service for notebook operations
func NewNotebookLLMService(ctx context.Context, cfg *configs.LLMConfig) (embeddings.Embedder, llms.Model, error) {
	// Create embedding client
	embeddingClient, err := openai.New(
		openai.WithToken(cfg.Providers.OpenAI.APIKey),
		openai.WithBaseURL(cfg.Providers.OpenAI.BaseURL),
		openai.WithEmbeddingModel(cfg.Providers.OpenAI.EmbeddingModel),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create embedding client: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embeddingClient)
	if err != nil {
		return nil, nil, fmt.Errorf("create embedder: %w", err)
	}

	// Create chat LLM client
	chatClient, err := openai.New(
		openai.WithToken(cfg.Providers.OpenAI.APIKey),
		openai.WithBaseURL(cfg.Providers.OpenAI.BaseURL),
		openai.WithModel(cfg.Providers.OpenAI.ChatModel),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create chat client: %w", err)
	}

	return embedder, chatClient, nil
}
