package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"NotebookAI/internal/models"
	"NotebookAI/internal/observability"
	"NotebookAI/internal/repository"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"go.uber.org/zap"
)

// NotebookChatSource represents a source chunk for chat response
type NotebookChatSource struct {
	NotebookID   string  `json:"notebook_id"`
	DocumentID   string  `json:"document_id"`
	DocumentName string  `json:"document_name"`
	PageNumber   int64   `json:"page_number"`
	ChunkIndex   int64   `json:"chunk_index"`
	Content      string  `json:"content"`
	Score        float32 `json:"score"`
}

// NotebookChatReply represents a streaming chat response
type NotebookChatReply struct {
	SessionID    string               `json:"session_id"`
	MessageID    string               `json:"message_id"`
	Content      string               `json:"content"`
	Sources      []NotebookChatSource `json:"sources"`
	PromptTokens int                  `json:"prompt_tokens"`
}

// NotebookChatService defines operations for notebook-based chat
type NotebookChatService interface {
	// Session management
	CreateSession(ctx context.Context, userID, notebookID, title string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error)
	DeleteSession(ctx context.Context, userID, sessionID string) error
	GetSession(ctx context.Context, userID, sessionID string) (*models.ChatSession, error)

	// Streaming chat
	StreamChat(ctx context.Context, userID, sessionID, question string, documentIDs []string, send func(reply *NotebookChatReply) bool) error
	// StreamChatWithSession 接受预加载的 session 对象，避免重复查询数据库
	StreamChatWithSession(ctx context.Context, userID string, session *models.ChatSession, question string, documentIDs []string, send func(reply *NotebookChatReply) bool) error

	// Search within notebook
	SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error)
}

// notebookChatService implements NotebookChatService
type notebookChatService struct {
	notebookRepo  repository.NotebookRepository
	docRepo       repository.DocumentRepository
	vectorStore   repository.NotebookVectorStore
	chatRepo      repository.ChatRepository
	llm           llms.Model
	embedder      embeddings.Embedder
	retrievalTopK int
}

// NewNotebookChatService creates a new NotebookChatService
func NewNotebookChatService(
	notebookRepo repository.NotebookRepository,
	docRepo repository.DocumentRepository,
	vectorStore repository.NotebookVectorStore,
	chatRepo repository.ChatRepository,
	llm llms.Model,
	embedder embeddings.Embedder,
	retrievalTopK int,
) NotebookChatService {
	return &notebookChatService{
		notebookRepo:  notebookRepo,
		docRepo:       docRepo,
		vectorStore:   vectorStore,
		chatRepo:      chatRepo,
		llm:           llm,
		embedder:      embedder,
		retrievalTopK: retrievalTopK,
	}
}

func (s *notebookChatService) CreateSession(ctx context.Context, userID, notebookID, title string) (*models.ChatSession, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New conversation"
	}
	now := time.Now()
	session := &models.ChatSession{
		ID:            uuid.NewString(),
		UserID:        userID,
		NotebookID:   notebookID,
		Title:         title,
		LastMessageAt: now,
	}
	if err := s.chatRepo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func (s *notebookChatService) ListSessions(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error) {
	sessions, err := s.chatRepo.ListSessionsByNotebook(ctx, userID, notebookID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

func (s *notebookChatService) DeleteSession(ctx context.Context, userID, sessionID string) error {
	// Get session to verify ownership
	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}

	// Delete all messages in the session first (cascade)
	if delErr := s.chatRepo.DeleteMessagesBySession(ctx, session.ID); delErr != nil {
		zap.L().Error("failed to delete session messages", zap.Error(delErr), zap.String("sessionID", session.ID))
	}

	// Delete the session itself
	if err := s.chatRepo.DeleteSession(ctx, session.ID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// GetSession 获取单个会话（供预检查使用）
func (s *notebookChatService) GetSession(ctx context.Context, userID, sessionID string) (*models.ChatSession, error) {
	return s.chatRepo.GetSession(ctx, userID, sessionID)
}

// StreamChat performs RAG-based chat with SSE streaming response
func (s *notebookChatService) StreamChat(ctx context.Context, userID, sessionID, question string, documentIDs []string, send func(reply *NotebookChatReply) bool) error {
	totalTimer := observability.NewStopwatch()
	requestID := uuid.NewString()
	metrics := observability.NewChatMetrics(requestID, sessionID, userID, "notebook_chat")

	// Step 1: Load session
	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		metrics.ErrorType = "session_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return fmt.Errorf("load session: %w", err)
	}

	// Step 2: Get conversation history
	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		metrics.ErrorType = "history_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return fmt.Errorf("load history: %w", err)
	}

	// Step 3: Determine RAG mode
	// 如果前端传入了 document_ids，则在指定文档中做 RAG 检索；否则跳过检索，降级为纯 AI 对话
	isRAGMode := len(documentIDs) > 0
	var docIDs []string
	if isRAGMode {
		var err error
		docIDs, err = s.notebookRepo.GetDocumentIDs(ctx, session.NotebookID)
		if err != nil {
			metrics.ErrorType = "get_doc_ids_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return fmt.Errorf("get notebook documents: %w", err)
		}

		// 构建白名单集合，过滤出 notebook 下同时被用户选中的文档
		allowedSet := make(map[string]struct{}, len(documentIDs))
		for _, id := range documentIDs {
			allowedSet[id] = struct{}{}
		}
		var filtered []string
		for _, id := range docIDs {
			if _, ok := allowedSet[id]; ok {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) > 0 {
			docIDs = filtered
		}
	}

	// Step 4: Retrieve relevant chunks from Milvus (仅 RAG 模式下执行)
	retrievalTimer := observability.NewStopwatch()
	var sources []NotebookChatSource

	if isRAGMode {
		// First embed the query
		queryVector, err := s.embedder.EmbedQuery(ctx, question)
		if err != nil {
			metrics.ErrorType = "embed_query_failed"
			metrics.ErrorMsg = err.Error()
			metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return fmt.Errorf("embed query: %w", err)
		}

		chunks, scores, err := s.vectorStore.Search(ctx, queryVector, s.retrievalTopK, session.NotebookID, docIDs)
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "vector_search_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return fmt.Errorf("search chunks: %w", err)
		}

		// Step 5: Build sources with document names
		sources = buildSourcesFromChunks(s, ctx, chunks, scores, userID)
	}

	// Step 6: Build prompt with strict format
	prompt := s.buildPrompt(history, sources, question)
	promptTokens := estimateTokens(prompt)

	// 记录检索事件
	notebookSourceDocIDs := getNotebookSourceDocumentIDs(sources)
	observability.LogRetrievalEvent(question, s.retrievalTopK, len(sources), metrics.RetrievalLatency, notebookSourceDocIDs)
	metrics.RetrievedChunks = len(sources)
	metrics.TopK = s.retrievalTopK

	// Step 7: Save user message FIRST (before LLM call) — 确保历史记录完整，即使后续AI回答失败也能保留问题
	messageID := uuid.NewString()
	now := time.Now()
	userMessage := &models.ChatMessage{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   question,
		CreatedAt: now,
	}
	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		zap.L().Error("save user message failed", zap.Error(err), zap.String("session_id", sessionID))
		// 不返回错误 — 继续尝试生成回答
	}

	// Step 8: Generate LLM response
	// Initial response with sources (通知客户端已开始处理)
	initialReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      "",
		Sources:      sources,
		PromptTokens: promptTokens,
	}
	if !send(initialReply) {
		return fmt.Errorf("client disconnected")
	}

	// LLM 生成阶段计时
	llmTimer := observability.NewStopwatch()
	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	metrics.LLMLatencyMs = llmTimer.ElapsedMs()
	if err != nil {
		metrics.ErrorType = "llm_generation_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return fmt.Errorf("generate response: %w", err)
	}

	completionTokens := estimateTokens(response)
	metrics.PromptTokens = promptTokens
	metrics.CompletionTokens = completionTokens
	metrics.TotalTokens = promptTokens + completionTokens

	// 记录 LLM 调用指标
	observability.LogLLMCall("generate", promptTokens, completionTokens, metrics.TotalTokens, metrics.LLMLatencyMs, "openai", nil)

	// Send final response
	finalReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      strings.TrimSpace(response),
		Sources:      sources,
		PromptTokens: estimateTokens(prompt),
	}
	if !send(finalReply) {
		return fmt.Errorf("client disconnected")
	}

	// Step 9: Save assistant message
	sourcesJSON, _ := json.Marshal(sources)
	assistantMessage := &models.ChatMessage{
		ID:               messageID,
		SessionID:        sessionID,
		UserID:           userID,
		Role:             "assistant",
		Content:          strings.TrimSpace(response),
		SourcesJSON:      string(sourcesJSON),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		CreatedAt:        now,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		zap.L().Error("save assistant message failed", zap.Error(err), zap.String("session_id", sessionID))
	}

	// Update session activity
	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(question)
	}
	_ = s.chatRepo.UpdateSessionActivity(ctx, userID, sessionID, title, now)

	// 记录完整的流式聊天指标 (StreamChat)
	metrics.TotalLatencyMs = totalTimer.ElapsedMs()
	metrics.SourceCount = len(sources)
	metrics.SourceDocumentIDs = notebookSourceDocIDs
	observability.LogChatRequest(metrics)

	return nil
}

// StreamChatWithSession 与 StreamChat 功能相同，但接受预加载的 session 对象，避免重复查询数据库
func (s *notebookChatService) StreamChatWithSession(ctx context.Context, userID string, session *models.ChatSession, question string, documentIDs []string, send func(reply *NotebookChatReply) bool) error {
	sessionID := session.ID

	// Step 1: Get conversation history（session 已预加载，跳过）
	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	// Step 2: Determine RAG mode
	// 如果前端传入了 document_ids，则在指定文档中做 RAG 检索；否则跳过检索，降级为纯 AI 对话
	isRAGMode := len(documentIDs) > 0
	var docIDs []string
	if isRAGMode {
		var err error
		docIDs, err = s.notebookRepo.GetDocumentIDs(ctx, session.NotebookID)
		if err != nil {
			return fmt.Errorf("get notebook documents: %w", err)
		}

		// 构建白名单集合，过滤出 notebook 下同时被用户选中的文档
		allowedSet := make(map[string]struct{}, len(documentIDs))
		for _, id := range documentIDs {
			allowedSet[id] = struct{}{}
		}
		var filtered []string
		for _, id := range docIDs {
			if _, ok := allowedSet[id]; ok {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) > 0 {
			docIDs = filtered
		}
	}

	// Step 3: Retrieve relevant chunks from Milvus（仅 RAG 模式）
	var sources []NotebookChatSource

	if isRAGMode {
		queryVector, err := s.embedder.EmbedQuery(ctx, question)
		if err != nil {
			return fmt.Errorf("embed query: %w", err)
		}

		chunks, scores, err := s.vectorStore.Search(ctx, queryVector, s.retrievalTopK, session.NotebookID, docIDs)
		if err != nil {
			return fmt.Errorf("search chunks: %w", err)
		}

		// Step 4: Build sources with batch document name lookup
		sources = buildSourcesFromChunks(s, ctx, chunks, scores, userID)
	}

	// Step 5: Build prompt
	prompt := s.buildPrompt(history, sources, question)

	// Step 6: Save user message FIRST (before LLM call) — 确保历史记录完整
	messageID := uuid.NewString()
	now := time.Now()
	userMessage := &models.ChatMessage{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   question,
		CreatedAt: now,
	}
	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		zap.L().Error("save user message failed", zap.Error(err), zap.String("session_id", sessionID))
	}

	// Step 7: Generate LLM response
	initialReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      "",
		Sources:      sources,
		PromptTokens: estimateTokens(prompt),
	}
	if !send(initialReply) {
		return fmt.Errorf("client disconnected")
	}

	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}

	finalReply := &NotebookChatReply{
		SessionID:    session.ID,
		MessageID:    messageID,
		Content:      strings.TrimSpace(response),
		Sources:      sources,
		PromptTokens: estimateTokens(prompt),
	}
	if !send(finalReply) {
		return fmt.Errorf("client disconnected")
	}

	// Step 8: Save assistant message
	sourcesJSON, _ := json.Marshal(sources)
	assistantMessage := &models.ChatMessage{
		ID:               messageID,
		SessionID:        sessionID,
		UserID:           userID,
		Role:             "assistant",
		Content:          strings.TrimSpace(response),
		SourcesJSON:      string(sourcesJSON),
		PromptTokens:     estimateTokens(prompt),
		CompletionTokens: estimateTokens(response),
		TotalTokens:      estimateTokens(prompt) + estimateTokens(response),
		CreatedAt:        now,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		zap.L().Error("save assistant message failed", zap.Error(err), zap.String("session_id", sessionID))
	}

	// Update session activity
	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(question)
	}
	_ = s.chatRepo.UpdateSessionActivity(ctx, userID, sessionID, title, now)

	return nil
}

func (s *notebookChatService) SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error) {
	if topK <= 0 {
		topK = s.retrievalTopK
	}

	if s.vectorStore == nil {
		return nil, fmt.Errorf("vector store not available")
	}

	docIDs, err := s.notebookRepo.GetDocumentIDs(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("get notebook documents: %w", err)
	}

	// Embed query
	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	chunks, scores, err := s.vectorStore.Search(ctx, queryVector, topK, notebookID, docIDs)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	sources := make([]NotebookChatSource, 0, len(chunks))
	for i, chunk := range chunks {
		doc, _ := s.docRepo.GetByID(ctx, userID, chunk.DocumentID)
		docName := "Unknown Document"
		if doc != nil {
			docName = doc.FileName
		}

		sources = append(sources, NotebookChatSource{
			NotebookID:   chunk.NotebookID,
			DocumentID:   chunk.DocumentID,
			DocumentName: docName,
			PageNumber:   chunk.PageNumber,
			ChunkIndex:   chunk.ChunkIndex,
			Content:      chunk.Content,
			Score:        scores[i],
		})
	}

	return sources, nil
}

// buildPrompt constructs the prompt with strict format for source citations
func (s *notebookChatService) buildPrompt(history []models.ChatMessage, sources []NotebookChatSource, question string) string {
	var builder strings.Builder

	if len(sources) == 0 {
		// 非RAG模式：纯 AI 对话，不引用文档上下文
		builder.WriteString("You are a helpful AI assistant. ")
		builder.WriteString("Answer questions using your general knowledge.\n\n")
	} else {
		// RAG 模式：基于文档检索结果回答
		builder.WriteString("You are an enterprise AI assistant similar to Google NotebookLM. ")
		builder.WriteString("Answer questions strictly based on the provided context from documents.\n\n")

		builder.WriteString("## Instructions\n")
		builder.WriteString("1. Answer based ONLY on the provided context\n")
		builder.WriteString("2. When referencing information, cite the source using [Source: DocumentName, Page X]\n")
		builder.WriteString("3. If the context is insufficient, say: 'I cannot find relevant information in the provided documents'\n")
		builder.WriteString("4. Be concise but comprehensive\n\n")

		builder.WriteString("## Retrieved Context\n")
		for i, src := range sources {
			builder.WriteString(fmt.Sprintf("[%d] Source: %s (Page %d)\nContent: %s\n\n",
				i+1,
				src.DocumentName,
				src.PageNumber+1, // Convert 0-indexed to 1-indexed
				src.Content,
			))
		}
	}

	builder.WriteString("## Conversation History\n")
	if len(history) > 0 {
		for _, msg := range history {
			builder.WriteString(fmt.Sprintf("%s: %s\n", strings.Title(msg.Role), msg.Content))
		}
	} else {
		builder.WriteString("(No previous messages)\n")
	}

	builder.WriteString("\n## Question\n")
	builder.WriteString(question)
	builder.WriteString("\n\n## Answer\n")

	return builder.String()
}

// buildSourcesFromChunks 从向量检索的 chunks 中构建带文档名的 sources 列表（避免重复代码）
func buildSourcesFromChunks(svc *notebookChatService, ctx context.Context, chunks []repository.NotebookChunk, scores []float32, userID string) []NotebookChatSource {
	sources := make([]NotebookChatSource, 0, len(chunks))

	docIDSet := make(map[string]struct{}, len(chunks))
	for _, chunk := range chunks {
		docIDSet[chunk.DocumentID] = struct{}{}
	}
	uniqueDocIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		uniqueDocIDs = append(uniqueDocIDs, id)
	}
	docNameMap := svc.docRepo.GetNamesByIDs(ctx, userID, uniqueDocIDs)

	for i, chunk := range chunks {
		docName := docNameMap[chunk.DocumentID]
		if docName == "" {
			docName = "Unknown Document"
		}
		sources = append(sources, NotebookChatSource{
			NotebookID:   chunk.NotebookID,
			DocumentID:   chunk.DocumentID,
			DocumentName: docName,
			PageNumber:   chunk.PageNumber,
			ChunkIndex:   chunk.ChunkIndex,
			Content:      chunk.Content,
			Score:        scores[i],
		})
	}
	return sources
}

// getNotebookSourceDocumentIDs 从 NotebookChatSource 列表中提取文档 ID（用于日志）
func getNotebookSourceDocumentIDs(sources []NotebookChatSource) []string {
	ids := make([]string, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		if _, exists := seen[src.DocumentID]; !exists && src.DocumentID != "" {
			ids = append(ids, src.DocumentID)
			seen[src.DocumentID] = struct{}{}
		}
	}
	return ids
}
