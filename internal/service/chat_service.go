package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"NotebookAI/internal/models"
	"NotebookAI/internal/observability"
	"NotebookAI/internal/repository"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

type ChatSource struct {
	DocumentID string  `json:"document_id"`
	FileName   string  `json:"file_name"`
	Content    string  `json:"content"`
	Score      float32 `json:"score"`
	ChunkIndex int     `json:"chunk_index"`
}

type ChatReply struct {
	Session              *models.ChatSession
	Message              *models.ChatMessage
	Sources              []ChatSource
	RecommendedQuestions []string          `json:"recommended_questions,omitempty"`
	Reflection           *ReflectionResult `json:"reflection,omitempty"`
}

type ChatService interface {
	CreateSession(ctx context.Context, userID, title string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error)
	ListMessages(ctx context.Context, userID, sessionID string) ([]models.ChatMessage, error)
	SendMessage(ctx context.Context, userID, sessionID, question string, documentIDs []string) (*ChatReply, error)
	Search(ctx context.Context, userID, query string, topK int, documentIDs []string) ([]ChatSource, error)
	StreamSendMessage(ctx context.Context, opts StreamChatOptions) error
	GenerateReflection(ctx context.Context, userID, sessionID, messageID string) (*ReflectionResult, error)
}

type chatService struct {
	llmService    LLMService
	chatRepo      repository.ChatRepository
	historyLimit  int
	retrievalTopK int
	// Phase 2: Hybrid RAG
	hybridSearch  HybridSearchService
	intentRewrite IntentRewriteService
	bm25Index     *BM25Index
}

func NewChatService(
	llmService LLMService,
	chatRepo repository.ChatRepository,
	historyLimit int,
	retrievalTopK int,
	hybridSearch HybridSearchService,
	intentRewrite IntentRewriteService,
	bm25Index *BM25Index,
) ChatService {
	return &chatService{
		llmService:    llmService,
		chatRepo:      chatRepo,
		historyLimit:  historyLimit,
		retrievalTopK: retrievalTopK,
		hybridSearch:  hybridSearch,
		intentRewrite: intentRewrite,
		bm25Index:     bm25Index,
	}
}

func (s *chatService) CreateSession(ctx context.Context, userID, title string) (*models.ChatSession, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "New conversation"
	}
	now := time.Now()
	session := &models.ChatSession{
		ID:            uuid.NewString(),
		UserID:        userID,
		Title:         title,
		LastMessageAt: now,
	}
	if err := s.chatRepo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func (s *chatService) ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error) {
	sessions, err := s.chatRepo.ListSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return sessions, nil
}

func (s *chatService) ListMessages(ctx context.Context, userID, sessionID string) ([]models.ChatMessage, error) {
	if _, err := s.chatRepo.GetSession(ctx, userID, sessionID); err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return messages, nil
}

func (s *chatService) SendMessage(ctx context.Context, userID, sessionID, question string, documentIDs []string) (*ChatReply, error) {
	totalTimer := observability.NewStopwatch()
	requestID := uuid.NewString()
	metrics := observability.NewChatMetrics(requestID, sessionID, userID, "chat")

	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		metrics.ErrorType = "session_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return nil, fmt.Errorf("load session: %w", err)
	}

	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, s.historyLimit)
	if err != nil {
		metrics.ErrorType = "history_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return nil, fmt.Errorf("load chat history: %w", err)
	}

	// 检索阶段计时
	retrievalTimer := observability.NewStopwatch()
	var sources []ChatSource

	if s.hybridSearch != nil {
		// Phase 2: 混合检索路径
		hybridResults, err := s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
			Query:       question,
			UserID:      userID,
			SessionID:   sessionID,
			DocumentIDs: documentIDs,
			TopK:        s.retrievalTopK,
		})
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "retrieval_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return nil, fmt.Errorf("hybrid search: %w", err)
		}
		sources = hybridResultsToChatSources(hybridResults)
	} else {
		// 降级：纯 Dense 检索
		retrievedDocs, err := s.llmService.RetrieveContext(ctx, question, s.retrievalTopK, RetrievalOptions{
			UserID:      userID,
			DocumentIDs: documentIDs,
		})
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "retrieval_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return nil, fmt.Errorf("retrieve source documents: %w", err)
		}
		sources = documentsToSources(retrievedDocs)
	}
	metrics.RetrievedChunks = len(sources)
	metrics.TopK = s.retrievalTopK

	prompt := buildPrompt(history, sources, question)

	// LLM 生成阶段计时
	llmTimer := observability.NewStopwatch()
	answer, err := s.llmService.GenerateAnswer(ctx, prompt)
	metrics.LLMLatencyMs = llmTimer.ElapsedMs()
	if err != nil {
		metrics.ErrorType = "llm_generation_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		return nil, fmt.Errorf("generate answer: %w", err)
	}

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
		return nil, fmt.Errorf("save user message: %w", err)
	}

	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return nil, fmt.Errorf("marshal sources: %w", err)
	}

	assistantMessage := &models.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		UserID:           userID,
		Role:             "assistant",
		Content:          answer.Text,
		SourcesJSON:      string(sourcesJSON),
		PromptTokens:     answer.PromptTokens,
		CompletionTokens: answer.CompletionTokens,
		TotalTokens:      answer.TotalTokens,
		CreatedAt:        now,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		return nil, fmt.Errorf("save assistant message: %w", err)
	}

	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(question)
	}
	if err := s.chatRepo.UpdateSessionActivity(ctx, userID, sessionID, title, now); err != nil {
		return nil, fmt.Errorf("update session activity: %w", err)
	}
	session.Title = title
	session.LastMessageAt = now

	// 记录完整的聊天指标
	metrics.TotalLatencyMs = totalTimer.ElapsedMs()
	metrics.PromptTokens = answer.PromptTokens
	metrics.CompletionTokens = answer.CompletionTokens
	metrics.TotalTokens = answer.TotalTokens
	metrics.SourceCount = len(sources)
	metrics.SourceDocumentIDs = getSourceDocumentIDs(sources)
	observability.LogChatRequest(metrics)

	return &ChatReply{
		Session: session,
		Message: assistantMessage,
		Sources: sources,
	}, nil
}

func (s *chatService) Search(ctx context.Context, userID, query string, topK int, documentIDs []string) ([]ChatSource, error) {
	if s.hybridSearch != nil {
		// Phase 2: 混合检索路径
		hybridResults, err := s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
			Query:       query,
			UserID:      userID,
			DocumentIDs: documentIDs,
			TopK:        topK,
		})
		if err != nil {
			return nil, fmt.Errorf("hybrid search: %w", err)
		}
		return hybridResultsToChatSources(hybridResults), nil
	}
	// 降级：纯 Dense 检索
	docs, err := s.llmService.RetrieveContext(ctx, query, topK, RetrievalOptions{
		UserID:      userID,
		DocumentIDs: documentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("search context: %w", err)
	}
	return documentsToSources(docs), nil
}

func documentsToSources(docs []schema.Document) []ChatSource {
	sources := make([]ChatSource, 0, len(docs))
	for _, doc := range docs {
		sources = append(sources, ChatSource{
			DocumentID: metadataString(doc.Metadata, "document_id"),
			FileName:   metadataString(doc.Metadata, "file_name"),
			Content:    doc.PageContent,
			Score:      doc.Score,
			ChunkIndex: metadataInt(doc.Metadata, "chunk_index"),
		})
	}
	return sources
}

// hybridResultsToChatSources 将 HybridResult 转换为 ChatSource
func hybridResultsToChatSources(results []HybridResult) []ChatSource {
	sources := make([]ChatSource, 0, len(results))
	for _, r := range results {
		chunkIndex := 0
		if v, ok := r.Metadata["chunk_index"]; ok {
			switch tv := v.(type) {
			case int:
				chunkIndex = tv
			case int64:
				chunkIndex = int(tv)
			case float64:
				chunkIndex = int(tv)
			}
		}
		sources = append(sources, ChatSource{
			DocumentID: r.DocumentID,
			Content:    r.Content,
			Score:      r.Score,
			ChunkIndex: chunkIndex,
		})
	}
	return sources
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func metadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func buildPrompt(history []models.ChatMessage, sources []ChatSource, question string) string {
	var builder strings.Builder

	builder.WriteString("You are an enterprise knowledge-base assistant. ")
	builder.WriteString("Answer strictly based on the provided context. ")
	builder.WriteString("If the context is insufficient, say so clearly and do not fabricate details.\n\n")
	builder.WriteString("Conversation history:\n")
	for _, message := range history {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}

	builder.WriteString("\nRetrieved context:\n")
	for i, source := range sources {
		builder.WriteString(fmt.Sprintf("[%d] (%s) %s\n", i+1, source.FileName, source.Content))
	}

	builder.WriteString("\nQuestion:\n")
	builder.WriteString(question)
	builder.WriteString("\n\nAnswer with concise reasoning and keep references grounded in the retrieved context.")

	return builder.String()
}

func buildSessionTitle(question string) string {
	title := strings.TrimSpace(question)
	if len([]rune(title)) > 53 {
		title = string([]rune(title)[:53]) + "..."
	}
	if title == "" {
		return "New conversation"
	}
	return title
}

// getSourceDocumentIDs 从来源列表中提取文档 ID 列表（用于日志）
func getSourceDocumentIDs(sources []ChatSource) []string {
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

// StreamSendMessage performs RAG-based chat with SSE streaming response
func (s *chatService) StreamSendMessage(ctx context.Context, opts StreamChatOptions) error {
	totalTimer := observability.NewStopwatch()
	requestID := uuid.NewString()
	metrics := observability.NewChatMetrics(requestID, opts.SessionID, opts.UserID, "chat")

	// Step 1: Load session
	session, err := s.chatRepo.GetSession(ctx, opts.UserID, opts.SessionID)
	if err != nil {
		metrics.ErrorType = "session_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("load session: %w", err))
		}
		return fmt.Errorf("load session: %w", err)
	}

	// Step 2: Get conversation history
	history, err := s.chatRepo.ListSessionMessages(ctx, opts.UserID, opts.SessionID, s.historyLimit)
	if err != nil {
		metrics.ErrorType = "history_load_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("load history: %w", err))
		}
		return fmt.Errorf("load history: %w", err)
	}

	// Step 3: Retrieve relevant chunks (with timing)
	retrievalTimer := observability.NewStopwatch()
	var sources []ChatSource

	if s.hybridSearch != nil {
		// Phase 2: 混合检索路径
		hybridResults, err := s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
			Query:       opts.Question,
			UserID:      opts.UserID,
			SessionID:   opts.SessionID,
			DocumentIDs: opts.DocumentIDs,
			TopK:        s.retrievalTopK,
		})
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "retrieval_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			if opts.OnError != nil {
				opts.OnError(fmt.Errorf("hybrid search: %w", err))
			}
			return fmt.Errorf("hybrid search: %w", err)
		}
		sources = hybridResultsToChatSources(hybridResults)
	} else {
		// 降级：纯 Dense 检索
		retrievedDocs, err := s.llmService.RetrieveContext(ctx, opts.Question, s.retrievalTopK, RetrievalOptions{
			UserID:      opts.UserID,
			DocumentIDs: opts.DocumentIDs,
		})
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "retrieval_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			if opts.OnError != nil {
				opts.OnError(fmt.Errorf("retrieve source documents: %w", err))
			}
			return fmt.Errorf("retrieve source documents: %w", err)
		}
		sources = documentsToSources(retrievedDocs)
	}
	metrics.RetrievedChunks = len(sources)
	metrics.TopK = s.retrievalTopK
	prompt := buildPrompt(history, sources, opts.Question)
	promptTokens := estimateTokens(prompt)

	// 记录检索事件
	sourceDocIDs := getSourceDocumentIDs(sources)
	observability.LogRetrievalEvent(opts.Question, s.retrievalTopK, len(sources), metrics.RetrievalLatency, sourceDocIDs)

	// Step 4: Send sources event first
	if opts.OnSource != nil {
		if !opts.OnSource(sources, promptTokens) {
			return fmt.Errorf("client disconnected")
		}
	}

	// Step 5: Generate response (with timing)
	llmTimer := observability.NewStopwatch()
	answer, err := s.llmService.GenerateAnswer(ctx, prompt)
	metrics.LLMLatencyMs = llmTimer.ElapsedMs()
	if err != nil {
		metrics.ErrorType = "llm_generation_failed"
		metrics.ErrorMsg = err.Error()
		metrics.TotalLatencyMs = totalTimer.ElapsedMs()
		observability.LogChatRequest(metrics)
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("generate answer: %w", err))
		}
		return fmt.Errorf("generate answer: %w", err)
	}

	// 记录 LLM 调用指标
	observability.LogLLMCall("generate", answer.PromptTokens, answer.CompletionTokens, answer.TotalTokens, metrics.LLMLatencyMs, "openai", nil)

	// Step 6: Stream response tokens (chunk size ~20 chars for natural feel)
	messageID := uuid.NewString()
	content := answer.Text
	chunkSize := 20
	for i := 0; i < len(content); i += chunkSize {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}
		token := content[i:end]

		if opts.OnToken != nil {
			if !opts.OnToken(token) {
				return fmt.Errorf("client disconnected")
			}
		}
	}

	// Step 7: Send done event
	if opts.OnDone != nil {
		opts.OnDone(content, promptTokens, answer.CompletionTokens, answer.TotalTokens)
	}

	// Step 8: Save messages to database
	now := time.Now()
	userMessage := &models.ChatMessage{
		ID:        uuid.NewString(),
		SessionID: opts.SessionID,
		UserID:    opts.UserID,
		Role:      "user",
		Content:   opts.Question,
		CreatedAt: now,
	}
	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		zap.L().Error("save user message failed", zap.Error(err))
	}

	sourcesJSON, _ := json.Marshal(sources)
	assistantMessage := &models.ChatMessage{
		ID:               messageID,
		SessionID:        opts.SessionID,
		UserID:           opts.UserID,
		Role:             "assistant",
		Content:          content,
		SourcesJSON:      string(sourcesJSON),
		PromptTokens:     answer.PromptTokens,
		CompletionTokens: answer.CompletionTokens,
		TotalTokens:      answer.TotalTokens,
		CreatedAt:        now,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		zap.L().Error("save assistant message failed", zap.Error(err))
	}

	// Update session activity
	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(opts.Question)
	}
	_ = s.chatRepo.UpdateSessionActivity(ctx, opts.UserID, opts.SessionID, title, now)

	// 记录完整的流式聊天指标
	metrics.TotalLatencyMs = totalTimer.ElapsedMs()
	metrics.PromptTokens = answer.PromptTokens
	metrics.CompletionTokens = answer.CompletionTokens
	metrics.TotalTokens = answer.TotalTokens
	metrics.SourceCount = len(sources)
	metrics.SourceDocumentIDs = sourceDocIDs
	observability.LogChatRequest(metrics)

	return nil
}

// GenerateReflection analyzes a specific message and provides feedback
func (s *chatService) GenerateReflection(ctx context.Context, userID, sessionID, messageID string) (*ReflectionResult, error) {
	// Load messages
	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 20)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	// Find the target message
	var targetMsg *models.ChatMessage
	var question string
	for i, msg := range messages {
		if msg.ID == messageID {
			targetMsg = &messages[i]
			break
		}
		// The question before this answer
		if msg.Role == "user" && i+1 < len(messages) && messages[i+1].Role == "assistant" && messages[i+1].ID == messageID {
			question = msg.Content
			targetMsg = &messages[i+1]
			break
		}
	}

	if targetMsg == nil {
		return nil, fmt.Errorf("message not found")
	}

	if targetMsg.Role != "assistant" {
		return nil, fmt.Errorf("target message must be an assistant response")
	}

	// Parse sources
	var sources []ChatSource
	if targetMsg.SourcesJSON != "" {
		json.Unmarshal([]byte(targetMsg.SourcesJSON), &sources)
	}

	sourceStrings := make([]string, 0, len(sources))
	for _, src := range sources {
		sourceStrings = append(sourceStrings, fmt.Sprintf("[%s] %s", src.FileName, src.Content))
	}

	// Generate reflection
	reflection, err := s.llmService.GenerateReflection(ctx, question, targetMsg.Content, sourceStrings)
	if err != nil {
		return nil, fmt.Errorf("generate reflection: %w", err)
	}

	return reflection, nil
}
