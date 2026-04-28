package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"NotebookAI/internal/configs"
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
	NotebookID   string    `json:"notebook_id"`
	DocumentID   string    `json:"document_id"`
	DocumentName string    `json:"document_name"`
	CitationID   string    `json:"citation_id,omitempty"`
	PageNumber   int64     `json:"page_number"`
	ChunkIndex   int64     `json:"chunk_index"`
	Content      string    `json:"content"`
	Score        float32   `json:"score"`
	ChunkType    string    `json:"chunk_type,omitempty"`
	SectionPath  []string  `json:"section_path,omitempty"`
	BoundingBox  []float32 `json:"bounding_box,omitempty"`
	VisualPath   string    `json:"visual_path,omitempty"`
	VisualType   string    `json:"visual_type,omitempty"`
}

type SelectedDocumentContext struct {
	DocumentID   string
	DocumentName string
	Summary      string
	KeyPoints    []string
	FAQ          []DocumentGuideFAQ
	GuideStatus  string
}

type DocumentGuideFAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// NotebookChatReply represents a streaming chat response
type NotebookChatReply struct {
	SessionID    string               `json:"session_id"`
	MessageID    string               `json:"message_id"`
	Content      string               `json:"content"`
	Sources      []NotebookChatSource `json:"sources"`
	PromptTokens int                  `json:"prompt_tokens"`
}

func (s NotebookChatSource) VisualEvidence() (string, string) {
	path := strings.TrimSpace(s.VisualPath)
	visualType := strings.TrimSpace(s.VisualType)
	if visualType == "" {
		visualType = extractVisualTypeMarker(s.Content)
	}
	if path == "" {
		path = extractVisualPathMarker(s.Content)
	}
	return path, visualType
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
	GetSessionMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error)
	RefreshSessionMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error)
	ClearSessionMemory(ctx context.Context, userID, sessionID string) error
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
	// Phase 2: Hybrid RAG
	hybridSearch     HybridSearchService
	intentRewrite    IntentRewriteService
	bm25Index        *BM25Index
	trustWorkflow    TrustWorkflow
	trustConfig      *configs.TrustWorkflowConfig
	citationGuard    *configs.CitationGuardConfig
	visualAnswerer   LLMService
	multimodalConfig *configs.MultimodalConfig
	memoryService    SessionMemoryService
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
	hybridSearch HybridSearchService,
	intentRewrite IntentRewriteService,
	bm25Index *BM25Index,
	trustWorkflow TrustWorkflow,
	trustConfig *configs.TrustWorkflowConfig,
	citationGuard *configs.CitationGuardConfig,
	visualAnswerer LLMService,
	multimodalConfig *configs.MultimodalConfig,
	memoryService SessionMemoryService,
) NotebookChatService {
	return &notebookChatService{
		notebookRepo:     notebookRepo,
		docRepo:          docRepo,
		vectorStore:      vectorStore,
		chatRepo:         chatRepo,
		llm:              llm,
		embedder:         embedder,
		retrievalTopK:    retrievalTopK,
		hybridSearch:     hybridSearch,
		intentRewrite:    intentRewrite,
		bm25Index:        bm25Index,
		trustWorkflow:    trustWorkflow,
		trustConfig:      trustConfig,
		citationGuard:    citationGuard,
		visualAnswerer:   visualAnswerer,
		multimodalConfig: multimodalConfig,
		memoryService:    memoryService,
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
		NotebookID:    notebookID,
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

	// Step 4: Retrieve relevant chunks (仅 RAG 模式下执行)
	retrievalTimer := observability.NewStopwatch()
	var sources []NotebookChatSource
	var hybridResults []HybridResult

	if isRAGMode {
		if s.hybridSearch != nil {
			// Phase 2: 混合检索路径
			hybridResults, err = s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
				Query:       question,
				UserID:      userID,
				SessionID:   sessionID,
				NotebookID:  session.NotebookID,
				DocumentIDs: docIDs,
				TopK:        s.retrievalTopK,
			})
			metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
			if err != nil {
				metrics.ErrorType = "hybrid_search_failed"
				metrics.ErrorMsg = err.Error()
				metrics.TotalLatencyMs = totalTimer.ElapsedMs()
				observability.LogChatRequest(metrics)
				return fmt.Errorf("hybrid search: %w", err)
			}
			sources = hybridResultsToNotebookSources(hybridResults, s, ctx, userID)
		} else {
			// 降级：纯 Dense 检索
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

			sources = buildSourcesFromChunks(s, ctx, chunks, scores, userID)
		}
		sources = annotateSourcesWithCitationIDs(sources)
	}

	// Step 6: Build prompt with strict format
	memoryPrompt := s.memoryPrompt(ctx, userID, sessionID)
	docContexts := s.loadSelectedDocumentContexts(ctx, userID, docIDs)
	prompt := s.buildPrompt(history, docContexts, sources, question, memoryPrompt)
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
	response, err := s.generateNotebookAnswer(ctx, prompt, TrustWorkflowInput{
		Question:         question,
		UserID:           userID,
		SessionID:        sessionID,
		NotebookID:       session.NotebookID,
		DocumentIDs:      docIDs,
		DocumentContexts: docContexts,
		History:          history,
		SearchResults:    hybridResults,
		Sources:          sources,
	})
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
	s.refreshMemoryAsync(userID, sessionID)

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

	// Step 3: Retrieve relevant chunks（仅 RAG 模式）
	var sources []NotebookChatSource
	var hybridResults []HybridResult

	if isRAGMode {
		if s.hybridSearch != nil {
			// Phase 2: 混合检索路径
			hybridResults, err = s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
				Query:       question,
				UserID:      userID,
				SessionID:   sessionID,
				NotebookID:  session.NotebookID,
				DocumentIDs: docIDs,
				TopK:        s.retrievalTopK,
			})
			if err != nil {
				return fmt.Errorf("hybrid search: %w", err)
			}
			sources = hybridResultsToNotebookSources(hybridResults, s, ctx, userID)
		} else {
			// 降级：纯 Dense 检索
			queryVector, err := s.embedder.EmbedQuery(ctx, question)
			if err != nil {
				return fmt.Errorf("embed query: %w", err)
			}

			chunks, scores, err := s.vectorStore.Search(ctx, queryVector, s.retrievalTopK, session.NotebookID, docIDs)
			if err != nil {
				return fmt.Errorf("search chunks: %w", err)
			}

			sources = buildSourcesFromChunks(s, ctx, chunks, scores, userID)
		}
		sources = annotateSourcesWithCitationIDs(sources)
	}

	// Step 5: Build prompt
	memoryPrompt := s.memoryPrompt(ctx, userID, sessionID)
	docContexts := s.loadSelectedDocumentContexts(ctx, userID, docIDs)
	prompt := s.buildPrompt(history, docContexts, sources, question, memoryPrompt)

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

	response, err := s.generateNotebookAnswer(ctx, prompt, TrustWorkflowInput{
		Question:         question,
		UserID:           userID,
		SessionID:        sessionID,
		NotebookID:       session.NotebookID,
		DocumentIDs:      docIDs,
		DocumentContexts: docContexts,
		History:          history,
		SearchResults:    hybridResults,
		Sources:          sources,
	})
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
	s.refreshMemoryAsync(userID, sessionID)

	// Update session activity
	title := session.Title
	if title == "" || title == "New conversation" {
		title = buildSessionTitle(question)
	}
	_ = s.chatRepo.UpdateSessionActivity(ctx, userID, sessionID, title, now)

	return nil
}

func (s *notebookChatService) shouldUseTrustWorkflow(input TrustWorkflowInput) bool {
	if s.trustWorkflow == nil || s.trustConfig == nil || !s.trustConfig.Enabled {
		return false
	}
	if isGuideFirstOverviewInput(input) {
		return false
	}
	plan := BuildTrustPlan(input.Question, input.History)
	return !s.trustConfig.HighRiskOnly || plan.HasHighRisk()
}

func (s *notebookChatService) generateNotebookAnswer(ctx context.Context, prompt string, input TrustWorkflowInput) (string, error) {
	if answer, ok := s.generateVisualNotebookAnswer(ctx, input); ok {
		return answer, nil
	}
	if s.shouldUseTrustWorkflow(input) && len(input.Sources) > 0 {
		output, err := s.trustWorkflow.Run(ctx, input)
		if err == nil && strings.TrimSpace(output.Answer) != "" {
			zap.L().Info("trust workflow generated notebook answer",
				zap.Bool("repaired", output.Repaired),
				zap.Bool("verified", output.Verification.Passed),
				zap.Int("evidence_count", len(output.EvidencePack.Items)),
			)
			return strings.TrimSpace(output.Answer), nil
		}
		zap.L().Warn("trust workflow failed, falling back to standard notebook generation", zap.Error(err))
	}
	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s.applyCitationGuard(ctx, response, input)), nil
}

func (s *notebookChatService) generateVisualNotebookAnswer(ctx context.Context, input TrustWorkflowInput) (string, bool) {
	cfg := s.multimodalConfig
	if cfg == nil || !cfg.Enabled || !cfg.VisualGenerationEnabled || s.visualAnswerer == nil {
		return "", false
	}
	if !isVisualQuestion(input.Question) {
		return "", false
	}
	source, path, ok := selectVisualSource(input.Sources, cfg.MinVisualScore)
	if !ok {
		return "", false
	}
	imageData, err := os.ReadFile(path)
	if err != nil {
		zap.L().Warn("visual answer image read failed, falling back to text generation",
			zap.String("visual_path", path),
			zap.Error(err),
		)
		return "", false
	}
	visionPrompt := buildVisualAnswerPrompt(input.Question, input.Sources, source)
	mimeType := mimeTypeFromPath(path)
	generated, err := s.visualAnswerer.AnswerWithImage(ctx, visionPrompt, imageData, mimeType)
	if err != nil || generated == nil || strings.TrimSpace(generated.Text) == "" {
		zap.L().Warn("visual answer generation failed, falling back to text generation", zap.Error(err))
		return "", false
	}
	return strings.TrimSpace(s.applyCitationGuard(ctx, generated.Text, input)), true
}

func (s *notebookChatService) applyCitationGuard(ctx context.Context, answer string, input TrustWorkflowInput) string {
	cfg := s.citationGuard
	if cfg == nil || !cfg.Enabled || len(input.Sources) == 0 {
		return strings.TrimSpace(answer)
	}

	pack := BuildEvidencePackFromNotebookSources(input.Sources)
	if len(pack.Items) == 0 {
		return citedInsufficientAnswer(pack)
	}
	if isGuideFirstOverviewInput(input) {
		return RenderEvidenceCitations(answer, pack)
	}

	plan := BuildTrustPlan(input.Question, input.History)
	if cfg.HighRiskOnly && !plan.HasHighRisk() {
		return RenderEvidenceCitations(answer, pack)
	}

	options := CitationGuardOptions{
		RequireParagraphCitations: cfg.RequireParagraphCitations,
		ValidateNumbers:           cfg.ValidateNumbers,
		ValidateEntityPhrases:     cfg.ValidateEntityPhrases,
		MinCitationCoverage:       cfg.MinCitationCoverage,
	}
	result := ValidateCitationBoundAnswer(answer, pack, options)
	if result.Passed {
		return RenderEvidenceCitations(answer, pack)
	}

	if cfg.RepairEnabled && cfg.MaxRepairAttempts > 0 {
		repaired, err := s.repairCitationBoundAnswer(ctx, answer, input, pack, result)
		if err == nil {
			repairedResult := ValidateCitationBoundAnswer(repaired, pack, options)
			if repairedResult.Passed {
				return RenderEvidenceCitations(repaired, pack)
			}
			zap.L().Warn("citation guard repair did not pass validation",
				zap.Int("issue_count", len(repairedResult.Issues)),
				zap.Float64("coverage", repairedResult.CitationCoverage),
			)
		} else {
			zap.L().Warn("citation guard repair failed", zap.Error(err))
		}
	}

	if cfg.FailClosedForHighRisk || plan.HasHighRisk() {
		return citedInsufficientAnswer(pack)
	}
	return RenderEvidenceCitations(answer, pack)
}

func isGuideFirstOverviewInput(input TrustWorkflowInput) bool {
	return len(input.DocumentContexts) > 0 && isMultiDocumentOverviewQuestion(input.Question)
}

func (s *notebookChatService) repairCitationBoundAnswer(ctx context.Context, answer string, input TrustWorkflowInput, pack EvidencePack, result CitationGuardResult) (string, error) {
	var issueLines []string
	for _, issue := range result.Issues {
		issueLines = append(issueLines, fmt.Sprintf("- %s: %s", issue.Type, issue.Detail))
	}
	prompt := fmt.Sprintf(`Repair the answer so every factual paragraph is supported by evidence IDs.

Rules:
- Use only evidence IDs like [E1].
- Do not output [Source: ...] directly.
- Remove or soften unsupported claims.
- If evidence is insufficient, say exactly: The provided documents do not contain sufficient information to answer this question.

Question:
%s

Evidence:
%s

Validation issues:
%s

Previous answer:
%s
`, input.Question, pack.FormatForPrompt(), strings.Join(issueLines, "\n"), answer)

	return llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
}

func (s *notebookChatService) SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error) {
	if topK <= 0 {
		topK = s.retrievalTopK
	}

	if s.hybridSearch != nil {
		// Phase 2: 混合检索路径
		docIDs, err := s.notebookRepo.GetDocumentIDs(ctx, notebookID)
		if err != nil {
			return nil, fmt.Errorf("get notebook documents: %w", err)
		}
		hybridResults, err := s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
			Query:       query,
			UserID:      userID,
			NotebookID:  notebookID,
			DocumentIDs: docIDs,
			TopK:        topK,
		})
		if err != nil {
			return nil, fmt.Errorf("hybrid search: %w", err)
		}
		return annotateSourcesWithCitationIDs(hybridResultsToNotebookSources(hybridResults, s, ctx, userID)), nil
	}

	// 降级：纯 Dense 检索
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

	return annotateSourcesWithCitationIDs(sources), nil
}

// buildPrompt constructs the prompt with strict format for source citations
func (s *notebookChatService) memoryPrompt(ctx context.Context, userID, sessionID string) string {
	if s.memoryService == nil {
		return ""
	}
	return s.memoryService.MemoryPrompt(ctx, userID, sessionID)
}

func (s *notebookChatService) refreshMemoryAsync(userID, sessionID string) {
	if s.memoryService != nil {
		s.memoryService.MaybeRefreshAsync(userID, sessionID)
	}
}

func (s *notebookChatService) GetSessionMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error) {
	if s.memoryService == nil {
		return &SessionMemory{}, nil
	}
	return s.memoryService.GetMemory(ctx, userID, sessionID)
}

func (s *notebookChatService) RefreshSessionMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error) {
	if s.memoryService == nil {
		return &SessionMemory{}, nil
	}
	return s.memoryService.RefreshMemory(ctx, userID, sessionID)
}

func (s *notebookChatService) ClearSessionMemory(ctx context.Context, userID, sessionID string) error {
	if s.memoryService == nil {
		return nil
	}
	return s.memoryService.ClearMemory(ctx, userID, sessionID)
}

func (s *notebookChatService) buildPrompt(history []models.ChatMessage, docContexts []SelectedDocumentContext, sources []NotebookChatSource, question string, memoryPrompt string) string {
	var builder strings.Builder

	if len(sources) == 0 && len(docContexts) == 0 {
		// 非RAG模式：纯 AI 对话，不引用文档上下文
		builder.WriteString("You are a helpful AI assistant. ")
		builder.WriteString("Answer questions using your general knowledge.\n\n")
	} else {
		// RAG/Guide 模式：基于选中文档指南和检索结果回答
		pack := BuildEvidencePackFromNotebookSources(sources)
		guideFirstOverview := len(docContexts) > 0 && isMultiDocumentOverviewQuestion(question)
		builder.WriteString("You are an enterprise AI assistant similar to Google NotebookLM. ")
		builder.WriteString("Answer questions strictly based on the provided context from documents.\n\n")

		builder.WriteString("## Instructions\n")
		builder.WriteString("1. Answer based ONLY on the selected document context and evidence blocks below. Do NOT use external knowledge.\n")
		builder.WriteString("2. Every factual paragraph must end with one or more evidence IDs, for example [E1] or [E2][E5].\n")
		builder.WriteString("3. Do not output [Source: ...] citations directly; use evidence IDs only.\n")
		builder.WriteString("4. If a paragraph contains numbers, dates, names, rankings, risk ratings, or comparisons, cite the evidence block that directly contains those facts.\n")
		builder.WriteString("5. If evidence is insufficient, say: 'The provided documents do not contain sufficient information to answer this question.'\n")
		builder.WriteString("6. Keep paragraphs substantial and useful: prefer 2-4 concise paragraphs or bullets over many tiny citation-only lines.\n")
		builder.WriteString("7. Do not repeat the same citation after consecutive sentences that use the same source; place the citation at the end of the combined paragraph.\n\n")
		if guideFirstOverview {
			builder.WriteString("8. For high-level document overviews, the selected document guide context is sufficient grounding. Use evidence IDs only when citing exact page-level details from the evidence blocks; do not refuse simply because retrieved evidence is sparse.\n\n")
		}

		if strings.TrimSpace(memoryPrompt) != "" {
			builder.WriteString(formatMemoryPromptForNotebookChat(memoryPrompt))
			builder.WriteString("\n\n")
		}

		if len(docContexts) > 0 {
			builder.WriteString(formatSelectedDocumentContext(docContexts, question))
			builder.WriteString("\n\n")
		}

		builder.WriteString("## Evidence Blocks\n")
		if len(pack.Items) == 0 {
			builder.WriteString("No exact retrieved evidence blocks were found. Use the selected document guide context for high-level answers, and avoid page-specific claims.\n\n")
		} else {
			for _, item := range pack.Items {
				builder.WriteString(fmt.Sprintf("[%s] [Source: %s, Page %d]\nContent: %s\n\n",
					item.ID,
					item.DocumentName,
					item.PageNumber+1,
					item.Content,
				))
			}
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

func (s *notebookChatService) loadSelectedDocumentContexts(ctx context.Context, userID string, docIDs []string) []SelectedDocumentContext {
	if len(docIDs) == 0 || s.notebookRepo == nil {
		return nil
	}
	names := map[string]string{}
	if s.docRepo != nil {
		names = s.docRepo.GetNamesByIDs(ctx, userID, docIDs)
	}
	contexts := make([]SelectedDocumentContext, 0, len(docIDs))
	for _, docID := range docIDs {
		docID = strings.TrimSpace(docID)
		if docID == "" {
			continue
		}
		ctxItem := SelectedDocumentContext{
			DocumentID:   docID,
			DocumentName: firstNonEmptyString(names[docID], docID),
			GuideStatus:  models.GuideStatusPending,
		}
		guide, err := s.notebookRepo.GetGuide(ctx, docID)
		if err == nil && guide != nil {
			ctxItem.Summary = strings.TrimSpace(guide.Summary)
			ctxItem.KeyPoints = parseGuideStringList(guide.KeyPoints)
			ctxItem.FAQ = parseGuideFAQ(guide.FaqJSON)
			ctxItem.GuideStatus = guide.Status
		}
		contexts = append(contexts, ctxItem)
	}
	return contexts
}

func formatSelectedDocumentContext(contexts []SelectedDocumentContext, question string) string {
	var builder strings.Builder
	builder.WriteString("## Selected Document Context\n")
	builder.WriteString(fmt.Sprintf("The user selected %d document(s). Treat these documents as the active reading context for this answer.\n", len(contexts)))
	if len(contexts) > 1 && isMultiDocumentOverviewQuestion(question) {
		builder.WriteString("This is a multi-document overview/comparison request: you must address each selected document explicitly. If exact retrieved evidence is thin for a document, use its guide summary/key points for the high-level overview and avoid inventing page-specific facts.\n")
	}
	for i, doc := range contexts {
		builder.WriteString(fmt.Sprintf("\n[D%d] %s\n", i+1, firstNonEmptyString(doc.DocumentName, doc.DocumentID)))
		if doc.GuideStatus != "" {
			builder.WriteString("Guide status: " + doc.GuideStatus + "\n")
		}
		if strings.TrimSpace(doc.Summary) != "" {
			builder.WriteString("Summary: " + trimForPrompt(doc.Summary, 900) + "\n")
		}
		if len(doc.KeyPoints) > 0 {
			builder.WriteString("Key points:\n")
			for _, point := range firstNStrings(doc.KeyPoints, 6) {
				builder.WriteString("- " + trimForPrompt(point, 240) + "\n")
			}
		}
		if len(doc.FAQ) > 0 {
			builder.WriteString("Representative FAQ:\n")
			for _, item := range firstNFAQ(doc.FAQ, 3) {
				builder.WriteString("- Q: " + trimForPrompt(item.Question, 160) + "\n")
				if strings.TrimSpace(item.Answer) != "" {
					builder.WriteString("  A: " + trimForPrompt(item.Answer, 240) + "\n")
				}
			}
		}
	}
	return builder.String()
}

func parseGuideStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return cleanStringList(values)
	}
	return cleanStringList(strings.Split(raw, "\n"))
}

func parseGuideFAQ(raw string) []DocumentGuideFAQ {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []DocumentGuideFAQ
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	return nil
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(value, "-"))
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func firstNStrings(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func firstNFAQ(values []DocumentGuideFAQ, n int) []DocumentGuideFAQ {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func trimForPrompt(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func isMultiDocumentOverviewQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"分别", "各自", "两个文档", "多份文档", "这些文档", "对比", "比较", "区别", "相同", "不同", "讲了什么", "总结", "概括", "overview", "compare", "comparison", "summarize"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func formatMemoryPromptForNotebookChat(memoryPrompt string) string {
	memoryPrompt = strings.TrimSpace(memoryPrompt)
	if memoryPrompt == "" {
		return ""
	}
	if strings.Contains(memoryPrompt, "## Conversation Memory") {
		return memoryPrompt
	}
	return "## Conversation Memory\nThis memory summarizes prior turns in this session and never overrides document evidence.\n- Summary: " + memoryPrompt
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
			ChunkType:    chunk.ChunkType,
			SectionPath:  parseStringArray(chunk.SectionPath),
			BoundingBox:  parseFloat32Array(chunk.BBox),
			VisualPath:   extractVisualPathMarker(chunk.Content),
			VisualType:   extractVisualTypeMarker(chunk.Content),
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

// hybridResultsToNotebookSources 将 HybridResult 转换为 NotebookChatSource
func hybridResultsToNotebookSources(results []HybridResult, svc *notebookChatService, ctx context.Context, userID string) []NotebookChatSource {
	sources := make([]NotebookChatSource, 0, len(results))

	// 批量获取文档名
	docIDSet := make(map[string]struct{}, len(results))
	for _, r := range results {
		if r.DocumentID != "" {
			docIDSet[r.DocumentID] = struct{}{}
		}
	}
	uniqueDocIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		uniqueDocIDs = append(uniqueDocIDs, id)
	}
	docNameMap := svc.docRepo.GetNamesByIDs(ctx, userID, uniqueDocIDs)

	for _, r := range results {
		docName := docNameMap[r.DocumentID]
		if docName == "" {
			docName = "Unknown Document"
		}
		pageNumber := int64(0)
		if v, ok := r.Metadata["page_number"]; ok {
			switch tv := v.(type) {
			case int:
				pageNumber = int64(tv)
			case int64:
				pageNumber = tv
			case float64:
				pageNumber = int64(tv)
			}
		}
		chunkIndex := int64(0)
		if v, ok := r.Metadata["chunk_index"]; ok {
			switch tv := v.(type) {
			case int:
				chunkIndex = int64(tv)
			case int64:
				chunkIndex = tv
			case float64:
				chunkIndex = int64(tv)
			}
		}
		sources = append(sources, NotebookChatSource{
			DocumentID:   r.DocumentID,
			DocumentName: docName,
			PageNumber:   pageNumber,
			ChunkIndex:   chunkIndex,
			Content:      r.Content,
			Score:        r.Score,
			ChunkType:    metadataStringAny(r.Metadata, "chunk_type"),
			SectionPath:  metadataStringSlice(r.Metadata, "section_path"),
			BoundingBox:  metadataFloat32Slice(r.Metadata, "bbox"),
			VisualPath:   firstNonEmpty(metadataStringAny(r.Metadata, "visual_path"), extractVisualPathMarker(r.Content)),
			VisualType:   firstNonEmpty(metadataStringAny(r.Metadata, "visual_type"), extractVisualTypeMarker(r.Content)),
		})
	}
	return sources
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func extractVisualPathMarker(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "visualpath:") {
			return strings.TrimSpace(line[len("VisualPath:"):])
		}
	}
	return ""
}

func extractVisualTypeMarker(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "[visual:") && strings.HasSuffix(line, "]") {
			return strings.TrimSpace(line[len("[Visual:") : len(line)-1])
		}
	}
	return ""
}

func isVisualQuestion(question string) bool {
	q := strings.ToLower(question)
	return hasAny(q, "chart", "graph", "figure", "image", "diagram", "shown", "visual", "plot", "trend", "图", "图表", "图片", "曲线", "趋势", "柱状", "饼图", "组织结构")
}

func selectVisualSource(sources []NotebookChatSource, minScore float32) (NotebookChatSource, string, bool) {
	for _, source := range sources {
		path, visualType := source.VisualEvidence()
		if path == "" {
			continue
		}
		if minScore > 0 && source.Score > 0 && source.Score < minScore {
			continue
		}
		if visualType == "" && source.ChunkType != "image" && source.ChunkType != "caption" {
			continue
		}
		return source, path, true
	}
	return NotebookChatSource{}, "", false
}

func buildVisualAnswerPrompt(question string, sources []NotebookChatSource, visualSource NotebookChatSource) string {
	pack := BuildEvidencePackFromNotebookSources(sources)
	var builder strings.Builder
	builder.WriteString("Answer this document question using the attached cropped visual region and the evidence text below.\n")
	builder.WriteString("Use only visible information from the image and evidence. If the visual evidence is insufficient, say exactly: The provided documents do not contain sufficient information to answer this question.\n")
	builder.WriteString("Every factual paragraph must end with evidence IDs such as [E1]. Do not output raw [Source: ...] citations.\n\n")
	builder.WriteString("Question:\n")
	builder.WriteString(question)
	builder.WriteString("\n\nSelected visual evidence:\n")
	builder.WriteString(fmt.Sprintf("Document: %s, Page %d, Type: %s\n", visualSource.DocumentName, visualSource.PageNumber+1, firstNonEmpty(visualSource.VisualType, visualSource.ChunkType)))
	builder.WriteString(visualSource.Content)
	builder.WriteString("\n\nEvidence blocks:\n")
	builder.WriteString(pack.FormatForPrompt())
	builder.WriteString("\n\nAnswer:\n")
	return builder.String()
}

func mimeTypeFromPath(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func metadataStringAny(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata[key]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func metadataStringSlice(metadata map[string]interface{}, key string) []string {
	if metadata == nil {
		return nil
	}
	switch v := metadata[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return parseStringArray(v)
	default:
		return nil
	}
}

func metadataFloat32Slice(metadata map[string]interface{}, key string) []float32 {
	if metadata == nil {
		return nil
	}
	switch v := metadata[key].(type) {
	case []float32:
		return v
	case []float64:
		out := make([]float32, 0, len(v))
		for _, n := range v {
			out = append(out, float32(n))
		}
		return out
	case []interface{}:
		out := make([]float32, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case float32:
				out = append(out, n)
			case float64:
				out = append(out, float32(n))
			case int:
				out = append(out, float32(n))
			}
		}
		return out
	case string:
		return parseFloat32Array(v)
	default:
		return nil
	}
}

func parseStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	return []string{raw}
}

func parseFloat32Array(raw string) []float32 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []float32
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	var values64 []float64
	if err := json.Unmarshal([]byte(raw), &values64); err == nil {
		out := make([]float32, 0, len(values64))
		for _, n := range values64 {
			out = append(out, float32(n))
		}
		return out
	}
	return nil
}
