package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	NotebookID     string    `json:"notebook_id"`
	DocumentID     string    `json:"document_id"`
	DocumentName   string    `json:"document_name"`
	CitationID     string    `json:"citation_id,omitempty"`
	PageNumber     int64     `json:"page_number"`
	ChunkIndex     int64     `json:"chunk_index"`
	Content        string    `json:"content"`
	Score          float32   `json:"score"`
	ChunkType      string    `json:"chunk_type,omitempty"`
	SectionPath    []string  `json:"section_path,omitempty"`
	RetrievalRoute string    `json:"retrieval_route,omitempty"`
	BoundingBox    []float32 `json:"bounding_box,omitempty"`
	VisualPath     string    `json:"visual_path,omitempty"`
	VisualType     string    `json:"visual_type,omitempty"`
	EvidenceAnchor string    `json:"-"`
	EvidenceBucket string    `json:"-"`
	EvidenceStatus string    `json:"-"`
	EvidenceReason string    `json:"-"`
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

type NotebookAnswerMode string

const (
	NotebookAnswerModeExactFact       NotebookAnswerMode = "exact_fact"
	NotebookAnswerModeConstraintCheck NotebookAnswerMode = "constraint_check"
	NotebookAnswerModeComparison      NotebookAnswerMode = "comparison"
	NotebookAnswerModeCoverageListing NotebookAnswerMode = "coverage_listing"
	NotebookAnswerModeDesignSynthesis NotebookAnswerMode = "constrained_synthesis"
	NotebookAnswerModeOpenSynthesis   NotebookAnswerMode = "open_synthesis"
	NotebookAnswerModeOverview        NotebookAnswerMode = "overview"
)

type DocumentAnchor struct {
	Type    string
	Label   string
	Number  string
	Aliases []string
}

type NotebookRetrievalRoute struct {
	Name  string
	Query string
	TopK  int
}

type NotebookRetrievalPlan struct {
	Question    string
	Mode        NotebookAnswerMode
	Anchors     []DocumentAnchor
	Routes      []NotebookRetrievalRoute
	MaxEvidence int
}

type CoverageStatus string

const (
	CoverageExplicit CoverageStatus = "explicit"
	CoverageRelated  CoverageStatus = "related"
	CoverageExcluded CoverageStatus = "excluded"
	CoverageMissing  CoverageStatus = "missing"
)

type StructureEvidence struct {
	Source         NotebookChatSource
	Route          string
	Anchor         string
	Subject        string
	BucketKey      string
	CoverageStatus CoverageStatus
	Mandatory      bool
	Priority       int
	Reason         string
}

type CoverageItem struct {
	Anchor            string
	DocumentID        string
	DocumentName      string
	Evidence          []NotebookChatSource
	StructureEvidence []StructureEvidence
	IsExplicit        bool
	Status            CoverageStatus
	Signals           []string
	MissingFields     []string
	Reason            string
}

type CoverageMatrix struct {
	Items []CoverageItem
}

type CoverageCandidate struct {
	Anchor       string
	DocumentID   string
	DocumentName string
	SectionPath  []string
	PageNumber   int64
}

type CoverageScanResult struct {
	Items      []CoverageItem
	Partial    bool
	LimitHit   bool
	SourceKind string
}

type ComparisonSubject struct {
	Label               string
	DocumentID          string
	Aliases             []string
	DocumentNameDerived bool
}

type ComparisonDimension struct {
	Name     string
	Aliases  []string
	Required bool
}

type ComparisonCell struct {
	SubjectLabel string
	Dimension    string
	Evidence     []NotebookChatSource
	Status       string
}

type ComparisonMatrix struct {
	Subjects   []ComparisonSubject
	Dimensions []ComparisonDimension
	Cells      []ComparisonCell
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
	hybridSearch      HybridSearchService
	intentRewrite     IntentRewriteService
	bm25Index         *BM25Index
	trustWorkflow     TrustWorkflow
	trustConfig       *configs.TrustWorkflowConfig
	citationGuard     *configs.CitationGuardConfig
	visualAnswerer    LLMService
	multimodalConfig  *configs.MultimodalConfig
	memoryService     SessionMemoryService
	structureEvidence *configs.StructureEvidenceConfig
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
	structureEvidence *configs.StructureEvidenceConfig,
) NotebookChatService {
	return &notebookChatService{
		notebookRepo:      notebookRepo,
		docRepo:           docRepo,
		vectorStore:       vectorStore,
		chatRepo:          chatRepo,
		llm:               llm,
		embedder:          embedder,
		retrievalTopK:     retrievalTopK,
		hybridSearch:      hybridSearch,
		intentRewrite:     intentRewrite,
		bm25Index:         bm25Index,
		trustWorkflow:     trustWorkflow,
		trustConfig:       trustConfig,
		citationGuard:     citationGuard,
		visualAnswerer:    visualAnswerer,
		multimodalConfig:  multimodalConfig,
		memoryService:     memoryService,
		structureEvidence: structureEvidence,
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

		filtered, ok := filterSelectedNotebookDocuments(docIDs, documentIDs)
		if !ok {
			metrics.ErrorType = "selected_documents_not_found"
			metrics.ErrorMsg = "selected documents are not in this notebook"
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return fmt.Errorf("selected documents are not in this notebook")
		}
		docIDs = filtered
	}

	// Step 4: Retrieve relevant chunks (仅 RAG 模式下执行)
	retrievalTimer := observability.NewStopwatch()
	var sources []NotebookChatSource
	var hybridResults []HybridResult

	if isRAGMode {
		sources, hybridResults, err = s.retrieveNotebookSources(ctx, userID, session.ID, session.NotebookID, docIDs, question)
		metrics.RetrievalLatency = retrievalTimer.ElapsedMs()
		if err != nil {
			metrics.ErrorType = "notebook_retrieval_failed"
			metrics.ErrorMsg = err.Error()
			metrics.TotalLatencyMs = totalTimer.ElapsedMs()
			observability.LogChatRequest(metrics)
			return fmt.Errorf("retrieve notebook sources: %w", err)
		}
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

		filtered, ok := filterSelectedNotebookDocuments(docIDs, documentIDs)
		if !ok {
			return fmt.Errorf("selected documents are not in this notebook")
		}
		docIDs = filtered
	}

	// Step 3: Retrieve relevant chunks（仅 RAG 模式）
	var sources []NotebookChatSource
	var hybridResults []HybridResult

	if isRAGMode {
		sources, hybridResults, err = s.retrieveNotebookSources(ctx, userID, session.ID, session.NotebookID, docIDs, question)
		if err != nil {
			return fmt.Errorf("retrieve notebook sources: %w", err)
		}
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
	if isGuideGroundedSynthesisInput(input) {
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
	if isGuideGroundedSynthesisInput(input) {
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
	hasRelevantEvidence := evidencePackHasQuestionRelevantEvidence(input.Question, pack)
	if shouldKeepAnswerDespiteCitationIssues(result, input) {
		return RenderEvidenceCitations(attachFallbackEvidenceIDs(answer, pack), pack)
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

	if !hasRelevantEvidence && shouldFailClosedForCitationIssues(cfg, plan, input) {
		return citedInsufficientAnswer(pack)
	}
	if hasRelevantEvidence {
		return RenderEvidenceCitations(attachFallbackEvidenceIDs(answer, pack), pack)
	}
	return RenderEvidenceCitations(answer, pack)
}

func shouldKeepAnswerDespiteCitationIssues(result CitationGuardResult, input TrustWorkflowInput) bool {
	mode := classifyNotebookAnswerMode(input.Question)
	switch mode {
	case NotebookAnswerModeConstraintCheck, NotebookAnswerModeCoverageListing, NotebookAnswerModeComparison, NotebookAnswerModeDesignSynthesis, NotebookAnswerModeOpenSynthesis, NotebookAnswerModeOverview:
	default:
		return false
	}
	if len(result.Issues) == 0 {
		return false
	}
	for _, issue := range result.Issues {
		if issue.Type != "missing_paragraph_citation" && issue.Type != "weak_citation_coverage" {
			return false
		}
	}
	return true
}

func shouldFailClosedForCitationIssues(cfg *configs.CitationGuardConfig, plan TrustPlan, input TrustWorkflowInput) bool {
	mode := classifyNotebookAnswerMode(input.Question)
	if mode == NotebookAnswerModeExactFact && (cfg.FailClosedForHighRisk || plan.HasHighRisk()) {
		return true
	}
	if isPrecisionQuestion(input.Question) && (cfg.FailClosedForHighRisk || plan.HasHighRisk()) {
		return true
	}
	return plan.HasHighRisk() && mode == NotebookAnswerModeConstraintCheck && len(input.Sources) == 0
}

func attachFallbackEvidenceIDs(answer string, pack EvidencePack) string {
	if strings.TrimSpace(answer) == "" || len(pack.Items) == 0 || len(evidenceIDsInText(answer)) > 0 {
		return answer
	}
	paragraphs := answerParagraphs(answer)
	if len(paragraphs) == 0 {
		return answer
	}
	for i, paragraph := range paragraphs {
		if looksFactualParagraph(paragraph) {
			paragraphs[i] = strings.TrimSpace(paragraph) + " [" + pack.Items[0].ID + "]"
			break
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

func isGuideGroundedSynthesisInput(input TrustWorkflowInput) bool {
	if len(input.DocumentContexts) == 0 {
		return false
	}
	return classifyNotebookAnswerMode(input.Question).allowsGuideGrounding() && !isPrecisionQuestion(input.Question)
}

func prioritizeNotebookSourcesForQuestion(sources []NotebookChatSource, question string) []NotebookChatSource {
	if len(sources) < 2 {
		return sources
	}
	anchors := resolveDocumentAnchors(question)
	mode := classifyNotebookAnswerMode(question)
	ranked := make([]NotebookChatSource, len(sources))
	copy(ranked, sources)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := notebookSourcePriorityScore(ranked[i], anchors, mode)
		right := notebookSourcePriorityScore(ranked[j], anchors, mode)
		if left == right {
			return ranked[i].Score > ranked[j].Score
		}
		return left > right
	})
	return ranked
}

func notebookSourcePriorityScore(source NotebookChatSource, anchors []DocumentAnchor, mode NotebookAnswerMode) int {
	text := strings.ToLower(strings.Join([]string{
		source.DocumentName,
		strings.Join(source.SectionPath, " "),
		source.ChunkType,
		source.Content,
	}, " "))
	score := 0
	for _, anchor := range anchors {
		if anchorMatchesText(anchor, text) {
			score += 120
		} else if sameAnchorTypeDifferentNumber(anchor, text) {
			score -= 80
		}
	}
	scope := inferEvidenceScope(source.Content)
	switch mode {
	case NotebookAnswerModeConstraintCheck, NotebookAnswerModeExactFact:
		score += scoreScope(scope, "requirement", "instruction", "constraint", "code", "table")
	case NotebookAnswerModeCoverageListing:
		score += scoreScope(scope, "requirement", "instruction", "output", "table", "code")
	case NotebookAnswerModeDesignSynthesis:
		score += scoreScope(scope, "component_list", "requirement", "instruction", "constraint")
	case NotebookAnswerModeComparison:
		score += scoreScope(scope, "requirement", "instruction", "table", "result")
	}
	return score
}

func anchorMatchesText(anchor DocumentAnchor, lowerText string) bool {
	if anchorPhraseMatchesText(anchor.Label, lowerText) {
		return true
	}
	for _, alias := range anchor.Aliases {
		if anchorPhraseMatchesText(alias, lowerText) {
			return true
		}
	}
	return false
}

func anchorPhraseMatchesText(phrase, lowerText string) bool {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" || lowerText == "" {
		return false
	}
	pattern := regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(phrase) + `($|[^\p{L}\p{N}])`)
	return pattern.MatchString(lowerText)
}

func sameAnchorTypeDifferentNumber(anchor DocumentAnchor, lowerText string) bool {
	if anchor.Type == "" || anchor.Number == "" {
		return false
	}
	numberPattern := `([A-Za-z]?\d+(?:\.\d+)*[A-Za-z]?)`
	if strings.EqualFold(anchor.Type, "appendix") {
		numberPattern = `([A-Za-z]|\d+(?:\.\d+)*[A-Za-z]?)`
	}
	pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(anchor.Type) + `\s+` + numberPattern + `\b`)
	for _, match := range pattern.FindAllStringSubmatch(lowerText, -1) {
		if len(match) > 1 && !strings.EqualFold(match[1], anchor.Number) {
			return true
		}
	}
	return false
}

func inferEvidenceScope(content string) string {
	lower := strings.ToLower(content)
	switch {
	case hasAny(lower, "required", "requirement", "must", "you are required", "要求", "必须", "需要"):
		return "requirement"
	case hasAny(lower, "instruction", "step", "procedure", "connect", "output", "display", "print", "serial", "monitor", "显示", "输出"):
		return "instruction"
	case hasAny(lower, "component", "sensor", "actuator", "device", "module", "feature", "role", "field", "传感器", "执行器", "组件", "功能"):
		return "component_list"
	case hasAny(lower, "#include", "void setup", "void loop", "program prompt", "代码"):
		return "code"
	case hasAny(lower, "background", "theory", "overview", "理论", "背景"):
		return "background"
	default:
		return "unknown"
	}
}

func scoreScope(actual string, preferred ...string) int {
	for i, scope := range preferred {
		if actual == scope {
			return 50 - i*8
		}
	}
	if actual == "background" {
		return -30
	}
	return 0
}

func (s *notebookChatService) repairCitationBoundAnswer(ctx context.Context, answer string, input TrustWorkflowInput, pack EvidencePack, result CitationGuardResult) (string, error) {
	prompt := buildCitationRepairPrompt(input.Question, answer, pack, result)
	return llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
}

func buildCitationRepairPrompt(question, answer string, pack EvidencePack, result CitationGuardResult) string {
	var issueLines []string
	for _, issue := range result.Issues {
		issueLines = append(issueLines, fmt.Sprintf("- %s: %s", issue.Type, issue.Detail))
	}
	relevanceRule := `- Say exactly "The provided documents do not contain sufficient information to answer this question." ONLY if none of the evidence blocks is relevant to the question.`
	if evidencePackHasQuestionRelevantEvidence(question, pack) {
		relevanceRule = `- The evidence contains information relevant to the question. Do NOT replace a supported answer with "insufficient information" merely because citations are missing or weak.`
	}
	return fmt.Sprintf(`Repair the answer so every factual paragraph is supported by evidence IDs.

Rules:
- Use only evidence IDs like [E1].
- Do not output [Source: ...] directly.
- Remove or soften unsupported claims.
- If the evidence contains relevant information, you MUST answer using that evidence.
%s

Question:
%s

Evidence:
%s

Validation issues:
%s

Previous answer:
%s
`, relevanceRule, question, pack.FormatForPrompt(), strings.Join(issueLines, "\n"), answer)
}

func evidencePackHasQuestionRelevantEvidence(question string, pack EvidencePack) bool {
	if strings.TrimSpace(question) == "" || len(pack.Items) == 0 {
		return false
	}
	questionTerms := relevantQuestionTerms(question)
	anchors := resolveDocumentAnchors(question)
	for _, item := range pack.Items {
		text := strings.ToLower(strings.Join([]string{
			item.DocumentName,
			strings.Join(item.SectionPath, " "),
			item.ChunkType,
			item.Content,
		}, " "))
		if isOutputFormatQuestion(question) {
			if evidenceHasOutputFormatSignal(text) && evidenceMatchesSpecificQuestionConcept(question, text) {
				return true
			}
			continue
		}
		for _, anchor := range anchors {
			if anchorMatchesText(anchor, text) {
				return true
			}
		}
		matchCount := 0
		for _, term := range questionTerms {
			if isGenericRelevanceTerm(term) {
				continue
			}
			if evidenceMatchesQuestionTerm(term, text) {
				matchCount++
			}
			if matchCount >= 2 {
				return true
			}
		}
	}
	return false
}

func evidenceHasOutputFormatSignal(lowerText string) bool {
	return hasAny(lowerText,
		"output", "display", "show", "shown", "print", "printed", "prints", "serial", "monitor",
		"format", "formatted", "pattern", "frequency", "trigger", "loop", "cycle",
		"输出", "显示", "串口", "监视器", "格式", "频率", "触发")
}

func evidenceMatchesSpecificQuestionConcept(question, lowerText string) bool {
	for _, term := range relevantQuestionTerms(question) {
		if isGenericRelevanceTerm(term) || isQuestionAnchorRelevanceTerm(question, term) {
			continue
		}
		if evidenceMatchesQuestionTerm(term, lowerText) {
			return true
		}
	}
	return false
}

func isQuestionAnchorRelevanceTerm(question, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	for _, anchor := range resolveDocumentAnchors(question) {
		if term == strings.ToLower(anchor.Type) ||
			term == strings.ToLower(anchor.Label) ||
			term == strings.ToLower(anchor.Number) {
			return true
		}
		for _, alias := range anchor.Aliases {
			if term == strings.ToLower(alias) {
				return true
			}
		}
	}
	return false
}

func evidenceMatchesQuestionTerm(term, lowerText string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	if strings.Contains(lowerText, term) {
		return true
	}
	for _, alias := range bilingualQuestionTermAliases(term) {
		if strings.Contains(lowerText, alias) {
			return true
		}
	}
	return false
}

func bilingualQuestionTermAliases(term string) []string {
	switch term {
	case "距离":
		return []string{"distance", "range"}
	case "温度":
		return []string{"temperature", "temp"}
	case "湿度":
		return []string{"humidity"}
	case "串口", "监视器":
		return []string{"serial", "serial monitor"}
	case "输出":
		return []string{"output", "print", "printed", "prints", "log"}
	case "显示":
		return []string{"display", "show", "shown", "shows"}
	case "格式":
		return []string{"format", "formatted", "pattern"}
	case "频率":
		return []string{"frequency", "interval", "cadence", "cycle"}
	case "触发":
		return []string{"trigger", "condition"}
	case "实时":
		return []string{"real-time", "realtime", "live"}
	case "监控":
		return []string{"monitoring", "monitor"}
	case "可视化":
		return []string{"visualization", "visualisation", "visualize", "visualise"}
	default:
		return nil
	}
}

func isGenericRelevanceTerm(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "要求", "约束", "功能", "模块", "组件", "传感器", "执行器",
		"output", "display", "输出", "显示", "format", "格式":
		return true
	default:
		return false
	}
}

func relevantQuestionTerms(question string) []string {
	stopwords := map[string]struct{}{
		"the": {}, "and": {}, "or": {}, "for": {}, "with": {}, "that": {}, "this": {}, "what": {}, "which": {}, "does": {}, "into": {}, "from": {},
		"is": {}, "are": {}, "was": {}, "were": {}, "must": {}, "should": {}, "can": {}, "could": {}, "would": {},
	}
	seen := map[string]struct{}{}
	var terms []string
	addTerm := func(term string) bool {
		if len(terms) >= 20 {
			return true
		}
		term = strings.ToLower(strings.TrimSpace(term))
		if len([]rune(term)) < 2 {
			return false
		}
		if _, ok := stopwords[term]; ok {
			return false
		}
		if _, ok := seen[term]; ok {
			return false
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		return len(terms) >= 20
	}

	latinTokenPattern := regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*(?:[-_][A-Za-z0-9]+)*`)
	for _, token := range latinTokenPattern.FindAllString(question, -1) {
		if addTerm(token) {
			return terms
		}
	}

	chineseDomainTerms := []string{
		"实时", "显示", "输出", "串口", "监视器", "监控", "可视化", "格式", "频率", "触发",
		"距离", "温度", "湿度", "传感器", "执行器", "组件", "功能", "模块", "要求", "约束",
	}
	for _, term := range chineseDomainTerms {
		if strings.Contains(question, term) && addTerm(term) {
			return terms
		}
	}

	for _, anchor := range resolveDocumentAnchors(question) {
		if addTerm(anchor.Label) {
			return terms
		}
		for _, alias := range anchor.Aliases {
			if addTerm(alias) {
				return terms
			}
		}
		if addTerm(anchor.Number) {
			return terms
		}
	}

	normalized := strings.ToLower(question)
	normalized = regexp.MustCompile(`[^\p{L}\p{N}_]+`).ReplaceAllString(normalized, " ")
	for _, field := range strings.Fields(normalized) {
		if addTerm(field) {
			break
		}
	}
	return terms
}

func isOutputFormatQuestion(question string) bool {
	lower := strings.ToLower(question)
	return hasAny(lower, "输出方式", "格式", "频率", "触发条件", "output method", "output format", "frequency", "trigger", "display", "serial")
}

func (s *notebookChatService) retrieveNotebookSources(ctx context.Context, userID, sessionID, notebookID string, docIDs []string, question string) ([]NotebookChatSource, []HybridResult, error) {
	return s.retrieveNotebookSourcesWithTopK(ctx, userID, sessionID, notebookID, docIDs, question, s.retrievalTopK)
}

func (s *notebookChatService) retrieveNotebookSourcesWithTopK(ctx context.Context, userID, sessionID, notebookID string, docIDs []string, question string, topK int) ([]NotebookChatSource, []HybridResult, error) {
	if topK <= 0 {
		topK = s.retrievalTopK
	}
	plan := buildNotebookRetrievalPlan(question, topK)
	var allSources []NotebookChatSource
	var allHybridResults []HybridResult
	var lastErr error

	for _, route := range plan.Routes {
		if strings.TrimSpace(route.Query) == "" {
			continue
		}
		topK := route.TopK
		if topK <= 0 {
			topK = plan.MaxEvidence
		}
		if topK <= 0 {
			topK = s.retrievalTopK
		}
		if s.hybridSearch != nil {
			hybridResults, err := s.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
				Query:       route.Query,
				UserID:      userID,
				SessionID:   sessionID,
				NotebookID:  notebookID,
				DocumentIDs: docIDs,
				TopK:        topK,
			})
			if err != nil {
				lastErr = fmt.Errorf("%s: %w", route.Name, err)
				zap.L().Warn("notebook retrieval route failed", zap.String("route", route.Name), zap.Error(err))
				continue
			}
			allHybridResults = append(allHybridResults, hybridResults...)
			sources := hybridResultsToNotebookSources(hybridResults, s, ctx, userID)
			for i := range sources {
				sources[i].RetrievalRoute = route.Name
			}
			allSources = append(allSources, sources...)
			continue
		}

		if s.vectorStore == nil || s.embedder == nil {
			lastErr = fmt.Errorf("vector retrieval not available")
			continue
		}
		queryVector, err := s.embedder.EmbedQuery(ctx, route.Query)
		if err != nil {
			lastErr = fmt.Errorf("%s embed query: %w", route.Name, err)
			zap.L().Warn("notebook retrieval route embedding failed", zap.String("route", route.Name), zap.Error(err))
			continue
		}
		chunks, scores, err := s.vectorStore.Search(ctx, queryVector, topK, notebookID, docIDs)
		if err != nil {
			lastErr = fmt.Errorf("%s vector search: %w", route.Name, err)
			zap.L().Warn("notebook retrieval route vector search failed", zap.String("route", route.Name), zap.Error(err))
			continue
		}
		sources := buildSourcesFromChunks(s, ctx, chunks, scores, userID)
		for i := range sources {
			sources[i].RetrievalRoute = route.Name
		}
		allSources = append(allSources, sources...)
	}

	if len(allSources) == 0 && lastErr != nil {
		return nil, nil, lastErr
	}
	structureCfg := defaultStructureEvidenceConfig()
	if s.structureEvidence != nil {
		structureCfg = *s.structureEvidence
	}
	if structureCfg.MaxEvidence <= 0 {
		structureCfg.MaxEvidence = plan.MaxEvidence
	}
	broadSources := s.loadBroadNotebookSources(ctx, userID, notebookID, docIDs, structureCfg)
	if structureCfg.Enabled {
		mergedSources := applyStructureFirstEvidence(allSources, broadSources, question, structureCfg)
		s.logStructureEvidenceSources("final", mergedSources)
		return annotateSourcesWithCitationIDs(mergedSources), allHybridResults, nil
	}
	mergedSources := mergeNotebookSources(allSources, plan.MaxEvidence)
	mergedSources = prioritizeNotebookSourcesForQuestion(mergedSources, question)
	if plan.MaxEvidence > 0 && len(mergedSources) > plan.MaxEvidence {
		mergedSources = mergedSources[:plan.MaxEvidence]
	}
	return annotateSourcesWithCitationIDs(mergedSources), allHybridResults, nil
}

func mergeNotebookSources(sources []NotebookChatSource, maxEvidence int) []NotebookChatSource {
	if len(sources) == 0 {
		return nil
	}
	bestByKey := make(map[string]NotebookChatSource, len(sources))
	order := make([]string, 0, len(sources))
	for _, source := range sources {
		key := notebookSourceEvidenceKey(source)
		if key == "" {
			key = fmt.Sprintf("%s:%d:%d:%s", source.DocumentID, source.PageNumber, source.ChunkIndex, strings.Join(strings.Fields(source.Content), " "))
		}
		existing, exists := bestByKey[key]
		if !exists {
			order = append(order, key)
			bestByKey[key] = source
			continue
		}
		if source.Score > existing.Score {
			bestByKey[key] = source
		}
	}
	merged := make([]NotebookChatSource, 0, len(bestByKey))
	for _, key := range order {
		merged = append(merged, bestByKey[key])
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	if maxEvidence > 0 && len(merged) > maxEvidence {
		merged = merged[:maxEvidence]
	}
	return merged
}

func defaultStructureEvidenceConfig() configs.StructureEvidenceConfig {
	return configs.StructureEvidenceConfig{
		Enabled:              true,
		MaxChunksPerDocument: 500,
		AnchorContextWindow:  3,
		MaxEvidence:          24,
	}
}

func effectiveStructureEvidenceConfig(cfg configs.StructureEvidenceConfig) configs.StructureEvidenceConfig {
	defaults := defaultStructureEvidenceConfig()
	if cfg.MaxChunksPerDocument <= 0 {
		cfg.MaxChunksPerDocument = defaults.MaxChunksPerDocument
	}
	if cfg.AnchorContextWindow <= 0 {
		cfg.AnchorContextWindow = defaults.AnchorContextWindow
	}
	if cfg.MaxEvidence <= 0 {
		cfg.MaxEvidence = defaults.MaxEvidence
	}
	return cfg
}

func structureEvidenceBucketKey(ev StructureEvidence) string {
	if strings.TrimSpace(ev.BucketKey) != "" {
		return strings.TrimSpace(ev.BucketKey)
	}
	if strings.TrimSpace(ev.Anchor) != "" && (ev.Route == "anchor_first" || ev.Route == "exact_phrase") {
		return "anchor:" + strings.TrimSpace(ev.Anchor)
	}
	if strings.TrimSpace(ev.Anchor) != "" {
		return "coverage:" + strings.TrimSpace(ev.Anchor)
	}
	if strings.TrimSpace(ev.Subject) != "" {
		return "subject:" + strings.TrimSpace(ev.Subject)
	}
	return "route:" + strings.TrimSpace(ev.Route)
}

func structureEvidencePriority(ev StructureEvidence) int {
	priority := int(ev.Source.Score * 100)
	if ev.Mandatory {
		priority += 1000
	}
	switch ev.Route {
	case "anchor_first":
		priority += 500
	case "exact_phrase":
		priority += 450
	case "section_scan":
		priority += 400
	case "comparison_subject":
		priority += 350
	case "structure_discovery":
		priority += 250
	}
	switch ev.CoverageStatus {
	case CoverageExplicit:
		priority += 200
	case CoverageRelated:
		priority += 80
	case CoverageExcluded:
		priority += 30
	}
	if hasCoverageRequirementLanguage(ev.Source.Content) {
		priority += 50
	}
	if len(explicitCoverageSignals(ev.Source.Content, "")) > 0 {
		priority += 30
	}
	return priority
}

func structureEvidenceKey(ev StructureEvidence) string {
	key := notebookSourceEvidenceKey(ev.Source)
	if key == "" {
		key = fmt.Sprintf("%s:%s:%s", ev.Route, ev.BucketKey, strings.Join(strings.Fields(ev.Source.Content), " "))
	}
	return key
}

func dedupeStructureEvidence(evidence []StructureEvidence) []StructureEvidence {
	bestByKey := make(map[string]StructureEvidence, len(evidence))
	order := make([]string, 0, len(evidence))
	for _, ev := range evidence {
		ev.BucketKey = structureEvidenceBucketKey(ev)
		ev.Priority = structureEvidencePriority(ev)
		key := structureEvidenceKey(ev)
		existing, exists := bestByKey[key]
		if !exists {
			bestByKey[key] = ev
			order = append(order, key)
			continue
		}
		if ev.Priority > existing.Priority || ev.Priority == existing.Priority && ev.Source.Score > existing.Source.Score {
			bestByKey[key] = ev
		}
	}
	out := make([]StructureEvidence, 0, len(bestByKey))
	for _, key := range order {
		out = append(out, bestByKey[key])
	}
	return out
}

func mergeStructureEvidence(evidence []StructureEvidence, maxEvidence int) []StructureEvidence {
	if len(evidence) == 0 {
		return nil
	}
	if maxEvidence <= 0 {
		maxEvidence = len(evidence)
	}
	evidence = dedupeStructureEvidence(evidence)
	for i := range evidence {
		evidence[i].BucketKey = structureEvidenceBucketKey(evidence[i])
		evidence[i].Priority = structureEvidencePriority(evidence[i])
	}
	mandatoryBuckets := make(map[string][]StructureEvidence)
	for _, ev := range evidence {
		if ev.Mandatory {
			mandatoryBuckets[ev.BucketKey] = append(mandatoryBuckets[ev.BucketKey], ev)
		}
	}
	bucketKeys := make([]string, 0, len(mandatoryBuckets))
	for key := range mandatoryBuckets {
		bucketKeys = append(bucketKeys, key)
	}
	sort.SliceStable(bucketKeys, func(i, j int) bool {
		return bucketPriority(mandatoryBuckets[bucketKeys[i]]) > bucketPriority(mandatoryBuckets[bucketKeys[j]])
	})

	selected := make([]StructureEvidence, 0, minInt(maxEvidence, len(evidence)))
	used := map[string]struct{}{}
	for _, key := range bucketKeys {
		if len(selected) >= maxEvidence {
			break
		}
		bucket := mandatoryBuckets[key]
		sort.SliceStable(bucket, func(i, j int) bool {
			if bucket[i].Priority != bucket[j].Priority {
				return bucket[i].Priority > bucket[j].Priority
			}
			return bucket[i].Source.Score > bucket[j].Source.Score
		})
		selected = append(selected, bucket[0])
		used[structureEvidenceKey(bucket[0])] = struct{}{}
	}

	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].Mandatory != evidence[j].Mandatory {
			return evidence[i].Mandatory
		}
		if evidence[i].Priority != evidence[j].Priority {
			return evidence[i].Priority > evidence[j].Priority
		}
		return evidence[i].Source.Score > evidence[j].Source.Score
	})
	for _, ev := range evidence {
		if len(selected) >= maxEvidence {
			break
		}
		key := structureEvidenceKey(ev)
		if _, ok := used[key]; ok {
			continue
		}
		selected = append(selected, ev)
		used[key] = struct{}{}
	}
	return selected
}

func bucketPriority(bucket []StructureEvidence) int {
	best := 0
	for _, ev := range bucket {
		if ev.Priority > best {
			best = ev.Priority
		}
	}
	return best
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func discoverCoverageCandidates(sources []NotebookChatSource, question string) []CoverageCandidate {
	_ = question
	if len(sources) == 0 {
		return nil
	}
	candidatesByKey := make(map[string]CoverageCandidate, len(sources))
	order := make([]string, 0, len(sources))
	for _, source := range sources {
		for _, candidate := range coverageCandidatesFromSource(source) {
			if candidate.Anchor == "" {
				continue
			}
			key := coverageCandidateKey(candidate)
			if _, exists := candidatesByKey[key]; exists {
				continue
			}
			candidatesByKey[key] = candidate
			order = append(order, key)
		}
	}

	candidates := make([]CoverageCandidate, 0, len(candidatesByKey))
	for _, key := range order {
		candidates = append(candidates, candidatesByKey[key])
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if cmp := strings.Compare(strings.ToLower(candidates[i].DocumentName), strings.ToLower(candidates[j].DocumentName)); cmp != 0 {
			return cmp < 0
		}
		return compareCoverageAnchors(candidates[i].Anchor, candidates[j].Anchor) < 0
	})
	return candidates
}

func coverageCandidateKey(candidate CoverageCandidate) string {
	documentKey := strings.TrimSpace(candidate.DocumentID)
	if documentKey == "" {
		documentKey = strings.TrimSpace(candidate.DocumentName)
	}
	return documentKey + "\x00" + strings.TrimSpace(candidate.Anchor)
}

func coverageCandidateFromSource(source NotebookChatSource) CoverageCandidate {
	candidates := coverageCandidatesFromSource(source)
	if len(candidates) > 0 {
		return candidates[0]
	}
	return CoverageCandidate{}
}

func coverageCandidatesFromSource(source NotebookChatSource) []CoverageCandidate {
	if anchor := strings.TrimSpace(source.EvidenceAnchor); anchor != "" {
		return []CoverageCandidate{{
			Anchor:       anchor,
			DocumentID:   source.DocumentID,
			DocumentName: source.DocumentName,
			SectionPath:  source.SectionPath,
			PageNumber:   source.PageNumber,
		}}
	}
	anchors := coverageAnchorsInText(source.Content)
	if len(anchors) > 0 {
		candidates := make([]CoverageCandidate, 0, len(anchors))
		seen := map[string]struct{}{}
		for _, anchor := range anchors {
			if _, ok := seen[anchor]; ok {
				continue
			}
			seen[anchor] = struct{}{}
			candidates = append(candidates, CoverageCandidate{
				Anchor:       anchor,
				DocumentID:   source.DocumentID,
				DocumentName: source.DocumentName,
				SectionPath:  source.SectionPath,
				PageNumber:   source.PageNumber,
			})
		}
		return candidates
	}
	return []CoverageCandidate{{
		Anchor:       coverageAnchorForSource(source),
		DocumentID:   source.DocumentID,
		DocumentName: source.DocumentName,
		SectionPath:  source.SectionPath,
		PageNumber:   source.PageNumber,
	}}
}

func coverageAnchorsInText(text string) []string {
	matches := coverageAnchorPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	anchors := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		anchors = append(anchors, canonicalAnchorLabel(canonicalAnchorType(match[1]), strings.TrimSpace(match[2])))
	}
	return anchors
}

func buildCoverageItems(sources []NotebookChatSource, question string) []CoverageItem {
	return buildCoverageRows(sources, question, discoverCoverageCandidates(sources, question), false)
}

func buildCoverageMatrix(sources []NotebookChatSource, question string) CoverageMatrix {
	items := buildCoverageRows(sources, question, discoverCoverageCandidates(sources, question), true)
	return CoverageMatrix{Items: items}
}

func buildCoverageRows(sources []NotebookChatSource, question string, candidates []CoverageCandidate, includeMissingFields bool) []CoverageItem {
	if len(candidates) == 0 {
		return nil
	}
	groups := make(map[string]*CoverageItem, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := coverageCandidateKey(candidate)
		order = append(order, key)
		groups[key] = &CoverageItem{
			Anchor:       candidate.Anchor,
			DocumentID:   candidate.DocumentID,
			DocumentName: candidate.DocumentName,
			Status:       CoverageMissing,
		}
	}

	explicitQuestion := isExplicitCoverageQuestion(question)
	for _, source := range sources {
		for _, segment := range coverageSegmentSourcesForSource(source) {
			candidate := coverageCandidateFromSource(segment)
			key := coverageCandidateKey(candidate)
			item, ok := groups[key]
			if !ok {
				continue
			}
			item.Evidence = append(item.Evidence, segment)
			item.Signals = uniqueStrings(append(item.Signals, explicitCoverageSignals(segment.Content, question)...))
			status := CoverageRelated
			if strings.TrimSpace(segment.EvidenceStatus) != "" {
				status = CoverageStatus(segment.EvidenceStatus)
				item.IsExplicit = item.IsExplicit || status == CoverageExplicit
				if strings.TrimSpace(segment.EvidenceReason) != "" {
					item.Reason = segment.EvidenceReason
				}
			} else if len(item.Signals) > 0 {
				if explicitQuestion {
					item.IsExplicit = item.IsExplicit || hasCoverageRequirementLanguage(segment.Content)
				} else {
					item.IsExplicit = true
				}
				if !explicitQuestion || hasCoverageRequirementLanguage(segment.Content) {
					status = CoverageExplicit
				}
			} else if coverageSourceLooksControlOnly(segment.Content) {
				status = CoverageExcluded
			}
			if coverageStatusRank(status) > coverageStatusRank(item.Status) {
				item.Status = status
			}
			if anchor := strings.TrimSpace(segment.EvidenceAnchor); anchor != "" {
				item.Anchor = anchor
				item.DocumentID = segment.DocumentID
				item.DocumentName = segment.DocumentName
			}
		}
	}
	items := make([]CoverageItem, 0, len(groups))
	for _, key := range order {
		item := *groups[key]
		if item.Status == "" {
			item.Status = CoverageMissing
		}
		if item.IsExplicit {
			item.Status = CoverageExplicit
		}
		if item.Status == CoverageExcluded {
			item.Reason = "control/action evidence found, but no explicit data output/display requirement"
		}
		sort.SliceStable(item.Evidence, func(i, j int) bool {
			left := coverageEvidenceScore(item.Evidence[i], question)
			right := coverageEvidenceScore(item.Evidence[j], question)
			if left != right {
				return left > right
			}
			return item.Evidence[i].Score > item.Evidence[j].Score
		})
		if len(item.Evidence) > 3 {
			item.Evidence = item.Evidence[:3]
		}
		item.StructureEvidence = coverageItemStructureEvidence(item, question)
		if includeMissingFields {
			item.MissingFields = coverageMissingFields(item, question)
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsExplicit != items[j].IsExplicit {
			return items[i].IsExplicit
		}
		if cmp := strings.Compare(strings.ToLower(items[i].DocumentName), strings.ToLower(items[j].DocumentName)); cmp != 0 {
			return cmp < 0
		}
		if cmp := compareCoverageAnchors(items[i].Anchor, items[j].Anchor); cmp != 0 {
			return cmp < 0
		}
		return coverageItemScore(items[i], question) > coverageItemScore(items[j], question)
	})
	return items
}

func coverageSegmentSourcesForSource(source NotebookChatSource) []NotebookChatSource {
	if strings.TrimSpace(source.EvidenceAnchor) != "" {
		return []NotebookChatSource{source}
	}
	matches := coverageAnchorPattern.FindAllStringSubmatchIndex(source.Content, -1)
	if len(matches) == 0 {
		return []NotebookChatSource{source}
	}
	segments := make([]NotebookChatSource, 0, len(matches))
	for i, match := range matches {
		label := coverageAnchorLabelFromMatch(source.Content, match)
		if label == "" {
			continue
		}
		start := match[0]
		end := len(source.Content)
		if i+1 < len(matches) && matches[i+1][0] > start {
			end = matches[i+1][0]
		}
		segment := source
		segment.Content = strings.TrimSpace(source.Content[start:end])
		segment.EvidenceAnchor = label
		segment.EvidenceBucket = "coverage:" + label
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return []NotebookChatSource{source}
	}
	return segments
}

func coverageAnchorLabelFromMatch(text string, match []int) string {
	if len(match) < 6 || match[2] < 0 || match[3] < 0 || match[4] < 0 || match[5] < 0 {
		return ""
	}
	typ := text[match[2]:match[3]]
	number := text[match[4]:match[5]]
	return canonicalAnchorLabel(canonicalAnchorType(typ), strings.TrimSpace(number))
}

func coverageStatusRank(status CoverageStatus) int {
	switch status {
	case CoverageExplicit:
		return 4
	case CoverageRelated:
		return 3
	case CoverageExcluded:
		return 2
	case CoverageMissing:
		return 1
	default:
		return 0
	}
}

func coverageItemStructureEvidence(item CoverageItem, question string) []StructureEvidence {
	out := make([]StructureEvidence, 0, len(item.Evidence))
	for _, source := range item.Evidence {
		if source.EvidenceAnchor == "" {
			source.EvidenceAnchor = item.Anchor
		}
		if source.EvidenceBucket == "" {
			source.EvidenceBucket = "coverage:" + strings.TrimSpace(item.Anchor)
		}
		source.EvidenceStatus = string(item.Status)
		source.EvidenceReason = item.Reason
		out = append(out, StructureEvidence{
			Source:         source,
			Route:          "section_scan",
			Anchor:         item.Anchor,
			BucketKey:      "coverage:" + strings.TrimSpace(item.Anchor),
			CoverageStatus: item.Status,
			Mandatory:      item.Status == CoverageExplicit,
			Reason:         item.Reason,
		})
	}
	_ = question
	return out
}

func coverageSourceLooksControlOnly(text string) bool {
	lower := strings.ToLower(text)
	if lower == "" {
		return false
	}
	if hasAny(lower, "serial", "print", "display", "lcd", "monitor", "output", "log", "report", "显示", "输出", "串口", "监视器", "日志") {
		return false
	}
	return hasAny(lower, "turn on", "turns on", "switch", "relay", "led", "servo", "motor", "control", "detect", "检测", "控制", "打开", "关闭")
}

func buildAnchorFirstEvidenceFromSources(sources []NotebookChatSource, anchors []DocumentAnchor, cfg configs.StructureEvidenceConfig) []StructureEvidence {
	cfg = effectiveStructureEvidenceConfig(cfg)
	if len(sources) == 0 || len(anchors) == 0 || !cfg.Enabled {
		return nil
	}
	sorted := make([]NotebookChatSource, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].DocumentID != sorted[j].DocumentID {
			return sorted[i].DocumentID < sorted[j].DocumentID
		}
		return sorted[i].ChunkIndex < sorted[j].ChunkIndex
	})

	var evidence []StructureEvidence
	seen := map[string]struct{}{}
	for _, anchor := range anchors {
		for idx, source := range sorted {
			if !sourceMatchesAnchor(source, anchor) {
				continue
			}
			candidates := nearbyAnchorContext(sorted, idx, cfg.AnchorContextWindow)
			for _, candidate := range candidates {
				key := notebookSourceEvidenceKey(candidate)
				if _, ok := seen[anchor.Label+"\x00"+key]; ok {
					continue
				}
				seen[anchor.Label+"\x00"+key] = struct{}{}
				evidence = append(evidence, StructureEvidence{
					Source:    candidate,
					Route:     "anchor_first",
					Anchor:    anchor.Label,
					BucketKey: "anchor:" + anchor.Label,
					Mandatory: true,
				})
			}
		}
	}
	sort.SliceStable(evidence, func(i, j int) bool {
		left := anchorContextPriority(evidence[i].Source)
		right := anchorContextPriority(evidence[j].Source)
		if left != right {
			return left > right
		}
		return evidence[i].Source.ChunkIndex < evidence[j].Source.ChunkIndex
	})
	return evidence
}

func sourceMatchesAnchor(source NotebookChatSource, anchor DocumentAnchor) bool {
	text := strings.ToLower(strings.Join([]string{
		source.DocumentName,
		strings.Join(source.SectionPath, " "),
		source.Content,
	}, " "))
	return anchorMatchesText(anchor, text)
}

func nearbyAnchorContext(sources []NotebookChatSource, anchorIndex int, window int) []NotebookChatSource {
	if anchorIndex < 0 || anchorIndex >= len(sources) {
		return nil
	}
	if window <= 0 {
		window = 1
	}
	anchor := sources[anchorIndex]
	out := []NotebookChatSource{anchor}
	for i := 0; i < len(sources); i++ {
		if i == anchorIndex {
			continue
		}
		candidate := sources[i]
		if candidate.DocumentID != anchor.DocumentID {
			continue
		}
		if sameSectionPath(candidate.SectionPath, anchor.SectionPath) {
			if candidate.ChunkIndex >= anchor.ChunkIndex && candidate.ChunkIndex <= anchor.ChunkIndex+int64(window) {
				out = append(out, candidate)
			}
			continue
		}
		if len(anchor.SectionPath) == 0 && candidate.ChunkIndex > anchor.ChunkIndex && candidate.ChunkIndex <= anchor.ChunkIndex+int64(window) {
			out = append(out, candidate)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := anchorContextPriority(out[i])
		right := anchorContextPriority(out[j])
		if left != right {
			return left > right
		}
		return out[i].ChunkIndex < out[j].ChunkIndex
	})
	return out
}

func sameSectionPath(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return false
	}
	for i := range left {
		if !strings.EqualFold(strings.TrimSpace(left[i]), strings.TrimSpace(right[i])) {
			return false
		}
	}
	return true
}

func anchorContextPriority(source NotebookChatSource) int {
	score := 0
	if hasCoverageRequirementLanguage(source.Content) {
		score += 80
	}
	if len(explicitCoverageSignals(source.Content, "output display")) > 0 {
		score += 60
	}
	if firstCoverageAnchor(source.Content) != "" {
		score += 10
	}
	return score
}

type ExactPhraseCandidate struct {
	Phrase     string
	Variants   []string
	Confidence string
}

func extractExactPhraseCandidates(question string) []ExactPhraseCandidate {
	seen := map[string]struct{}{}
	add := func(out *[]ExactPhraseCandidate, phrase, confidence string) {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" {
			return
		}
		key := strings.ToLower(phrase) + "\x00" + confidence
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		*out = append(*out, ExactPhraseCandidate{
			Phrase:     phrase,
			Variants:   normalizeExactPhraseVariants(phrase),
			Confidence: confidence,
		})
	}

	var out []ExactPhraseCandidate
	for _, match := range regexp.MustCompile(`"([^"]+)"|'([^']+)'`).FindAllStringSubmatch(question, -1) {
		add(&out, firstNonEmpty(match[1], match[2]), "high")
	}
	for _, match := range regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(?:ms|s|sec|seconds|minutes|hours|days|%|cm|mm|m|hz|rpm|v|ma|a)\b`).FindAllString(question, -1) {
		add(&out, match, "high")
	}
	for _, anchor := range resolveDocumentAnchors(question) {
		add(&out, anchor.Label, "high")
	}
	for _, match := range regexp.MustCompile(`\b[A-Z][A-Za-z]+(?:\s+\d{2,}|\s+[A-Z0-9][A-Za-z0-9-]+)\b|[A-Za-z]+-\d+[A-Za-z0-9-]*\b`).FindAllString(question, -1) {
		add(&out, match, "medium")
	}
	return out
}

func normalizeExactPhraseVariants(phrase string) []string {
	phrase = strings.TrimSpace(strings.Join(strings.Fields(phrase), " "))
	if phrase == "" {
		return nil
	}
	variants := []string{phrase}
	noSpaceDecimal := regexp.MustCompile(`(\w+)\.\s+(\w+)`).ReplaceAllString(phrase, `$1.$2`)
	noSpaceUnits := regexp.MustCompile(`(?i)(\d)\s+(ms|s|sec|seconds|minutes|hours|days|%|cm|mm|m|hz|rpm|v|ma|a)\b`).ReplaceAllString(noSpaceDecimal, `$1$2`)
	withSpaceUnits := regexp.MustCompile(`(?i)(\d)(ms|sec|seconds|minutes|hours|days|cm|mm|hz|rpm|ma)\b`).ReplaceAllString(noSpaceDecimal, `$1 $2`)
	variants = append(variants, noSpaceDecimal, noSpaceUnits, withSpaceUnits)
	return uniqueStrings(variants)
}

func buildCoverageScanResult(retrieved []NotebookChatSource, broad []NotebookChatSource, question string, cfg configs.StructureEvidenceConfig) CoverageScanResult {
	cfg = effectiveStructureEvidenceConfig(cfg)
	var candidates []NotebookChatSource
	sourceKind := "fallback_topk"
	partial := true
	if len(broad) > 0 {
		candidates = broad
		sourceKind = "repository_chunks"
		partial = false
	} else {
		candidates = retrieved
	}
	candidates, limitHit := capCoverageScanCandidates(candidates, cfg.MaxChunksPerDocument)
	items := buildCoverageRows(candidates, question, discoverCoverageCandidates(candidates, question), true)
	for i := range items {
		if items[i].Status == "" {
			if items[i].IsExplicit {
				items[i].Status = CoverageExplicit
			} else if len(items[i].Evidence) > 0 {
				items[i].Status = CoverageRelated
			} else {
				items[i].Status = CoverageMissing
			}
		}
		if len(items[i].StructureEvidence) == 0 {
			items[i].StructureEvidence = coverageItemStructureEvidence(items[i], question)
		}
	}
	return CoverageScanResult{
		Items:      items,
		Partial:    partial || limitHit,
		LimitHit:   limitHit,
		SourceKind: sourceKind,
	}
}

func capCoverageScanCandidates(sources []NotebookChatSource, perDocLimit int) ([]NotebookChatSource, bool) {
	if perDocLimit <= 0 {
		return sources, false
	}
	counts := map[string]int{}
	out := make([]NotebookChatSource, 0, len(sources))
	limitHit := false
	for _, source := range sources {
		docKey := firstNonEmpty(source.DocumentID, source.DocumentName)
		if docKey == "" {
			docKey = "_unknown"
		}
		if counts[docKey] >= perDocLimit {
			limitHit = true
			continue
		}
		counts[docKey]++
		out = append(out, source)
	}
	return out, limitHit
}

func coverageScanAnchorsByStatus(result CoverageScanResult, status CoverageStatus) []string {
	anchors := make([]string, 0)
	for _, item := range result.Items {
		if item.Status == status {
			anchors = append(anchors, item.Anchor)
		}
	}
	return anchors
}

func applyStructureFirstEvidenceForTest(retrieved, broad []NotebookChatSource, question string, cfg configs.StructureEvidenceConfig) []NotebookChatSource {
	return applyStructureFirstEvidence(retrieved, broad, question, cfg)
}

func applyStructureFirstEvidence(retrieved, broad []NotebookChatSource, question string, cfg configs.StructureEvidenceConfig) []NotebookChatSource {
	cfg = effectiveStructureEvidenceConfig(cfg)
	if !cfg.Enabled {
		return retrieved
	}
	anchors := resolveDocumentAnchors(question)
	var evidence []StructureEvidence
	evidence = append(evidence, sourcesToStructureEvidence(retrieved, "original_question", false)...)
	evidence = append(evidence, buildAnchorFirstEvidenceFromSources(broad, anchors, cfg)...)
	evidence = append(evidence, exactPhraseEvidenceFromSources(broad, extractExactPhraseCandidates(question))...)
	if classifyNotebookAnswerMode(question) == NotebookAnswerModeCoverageListing {
		result := buildCoverageScanResult(retrieved, broad, question, cfg)
		for _, item := range result.Items {
			evidence = append(evidence, item.StructureEvidence...)
		}
	}
	for _, subject := range extractComparisonSubjects(question) {
		for _, source := range broad {
			if evidenceMatchesComparisonSubject(source, subject) {
				evidence = append(evidence, StructureEvidence{
					Source:    source,
					Route:     "comparison_subject",
					Subject:   subject.Label,
					BucketKey: "subject:" + subject.Label,
					Mandatory: true,
				})
			}
		}
	}
	merged := mergeStructureEvidence(evidence, cfg.MaxEvidence)
	return structureEvidenceToSources(merged)
}

func sourcesToStructureEvidence(sources []NotebookChatSource, route string, mandatory bool) []StructureEvidence {
	out := make([]StructureEvidence, 0, len(sources))
	for _, source := range sources {
		out = append(out, StructureEvidence{Source: source, Route: route, Mandatory: mandatory})
	}
	return out
}

func exactPhraseEvidenceFromSources(sources []NotebookChatSource, candidates []ExactPhraseCandidate) []StructureEvidence {
	var out []StructureEvidence
	for _, candidate := range candidates {
		if candidate.Confidence != "high" {
			continue
		}
		for _, source := range sources {
			if !sourceMatchesExactPhraseCandidate(source, candidate) {
				continue
			}
			out = append(out, StructureEvidence{
				Source:    source,
				Route:     "exact_phrase",
				Anchor:    candidate.Phrase,
				BucketKey: "anchor:" + candidate.Phrase,
				Mandatory: true,
			})
		}
	}
	return out
}

func sourceMatchesExactPhraseCandidate(source NotebookChatSource, candidate ExactPhraseCandidate) bool {
	text := strings.ToLower(strings.Join(strings.Fields(source.Content), " "))
	for _, variant := range candidate.Variants {
		variant = strings.ToLower(strings.Join(strings.Fields(variant), " "))
		if variant != "" && strings.Contains(text, variant) {
			return true
		}
	}
	return false
}

func structureEvidenceToSources(evidence []StructureEvidence) []NotebookChatSource {
	out := make([]NotebookChatSource, 0, len(evidence))
	for _, ev := range evidence {
		source := ev.Source
		if source.RetrievalRoute == "" {
			source.RetrievalRoute = ev.Route
		}
		if source.EvidenceAnchor == "" {
			source.EvidenceAnchor = ev.Anchor
		}
		if source.EvidenceBucket == "" {
			source.EvidenceBucket = structureEvidenceBucketKey(ev)
		}
		if source.EvidenceStatus == "" && ev.CoverageStatus != "" {
			source.EvidenceStatus = string(ev.CoverageStatus)
		}
		if source.EvidenceReason == "" {
			source.EvidenceReason = ev.Reason
		}
		out = append(out, source)
	}
	return out
}

func coverageMissingFields(item CoverageItem, question string) []string {
	lowerQuestion := strings.ToLower(question)
	checks := []struct {
		key           string
		questionTerms []string
		evidenceTerms []string
		signalTerms   []string
	}{
		{
			key:           "format",
			questionTerms: []string{"format", "格式"},
			evidenceTerms: []string{"format", "格式"},
			signalTerms:   []string{"format"},
		},
		{
			key:           "frequency",
			questionTerms: []string{"frequency", "cadence", "interval", "refresh", "频率", "刷新", "间隔", "周期"},
			evidenceTerms: []string{"frequency", "cadence", "interval", "refresh", "every", "per ", "each ", "每", "频率", "刷新", "间隔", "周期"},
			signalTerms:   []string{"frequency", "every"},
		},
		{
			key:           "trigger",
			questionTerms: []string{"trigger", "condition", "when", "触发", "条件"},
			evidenceTerms: []string{"trigger", "condition", "when ", "after ", "触发", "条件"},
			signalTerms:   []string{"trigger"},
		},
		{
			key:           "output_method",
			questionTerms: []string{"output method", "display method", "输出方式", "显示方式", "输出方法", "显示方法"},
			evidenceTerms: []string{"serial monitor", "serial", "monitor", "display", "lcd", "screen", "print", "output", "log", "report", "串口", "监视器", "显示", "屏幕", "打印", "输出", "日志"},
			signalTerms:   []string{"serial monitor", "serial", "monitor", "display", "print", "output", "log", "report"},
		},
	}

	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if !hasAny(lowerQuestion, check.questionTerms...) {
			continue
		}
		if coverageItemHasAnySignal(item, check.signalTerms...) || coverageItemEvidenceHasAny(item, check.evidenceTerms...) || check.key == "format" && coverageItemHasOutputShapeEvidence(item) {
			continue
		}
		missing = append(missing, check.key)
	}
	return missing
}

func coverageItemHasAnySignal(item CoverageItem, terms ...string) bool {
	for _, signal := range item.Signals {
		if hasAny(strings.ToLower(signal), terms...) {
			return true
		}
	}
	return false
}

func coverageItemEvidenceHasAny(item CoverageItem, terms ...string) bool {
	for _, source := range item.Evidence {
		if hasAny(strings.ToLower(source.Content), terms...) {
			return true
		}
	}
	return false
}

func coverageItemHasOutputShapeEvidence(item CoverageItem) bool {
	for _, source := range item.Evidence {
		if hasOutputShapeEvidence(source.Content) {
			return true
		}
	}
	return false
}

func hasOutputShapeEvidence(text string) bool {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return false
	}
	if outputShapeHasTemplateValue(normalized, outputShapePattern) {
		return true
	}
	if outputShapeHasTemplateValue(normalized, outputTemplatePattern) {
		return true
	}
	if unitFormatPattern.MatchString(normalized) {
		return true
	}
	return quotedOutputPattern.MatchString(normalized)
}

func outputShapeHasTemplateValue(text string, pattern *regexp.Regexp) bool {
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && outputShapeValueLooksTemplate(match[1]) {
			return true
		}
	}
	return false
}

func outputShapeValueLooksTemplate(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "xx") || strings.Contains(lower, "%") {
		return true
	}
	if quotedTemplateValuePattern.MatchString(lower) {
		return true
	}
	if numericValuePattern.MatchString(lower) {
		return true
	}
	return unitValuePattern.MatchString(lower)
}

var (
	outputShapePattern         = regexp.MustCompile(`(?i)\b[A-Za-z][A-Za-z0-9 _/-]{0,40}:\s*([^.;\n]+)`)
	outputTemplatePattern      = regexp.MustCompile(`(?i)\b[A-Za-z][A-Za-z0-9 _/-]{0,40}\s*=\s*([^.;\n]+)`)
	unitFormatPattern          = regexp.MustCompile(`(?i)\b(?:distance|temperature|humidity|speed|rpm|voltage|current|duration|time|frequency|angle|pressure|距离|温度|湿度|速度|电压|电流|时间|频率|角度|压力)\s+in\s+(?:cm|mm|m|ms|s|sec|seconds|hz|rpm|v|ma|a|c|f|%)\b`)
	quotedOutputPattern        = regexp.MustCompile(`(?i)(?:print|display|output|show|log|report|输出|显示|打印)\s+(?:"[^"]+"|'[^']+')`)
	quotedTemplateValuePattern = regexp.MustCompile(`(?i)^["'][^"']+["']$`)
	numericValuePattern        = regexp.MustCompile(`(?i)\b\d+(?:\.\d+)?\s*(?:cm|mm|m|ms|s|sec|seconds|hz|rpm|v|ma|a|c|f|%)\b`)
	unitValuePattern           = regexp.MustCompile(`(?i)\b(?:cm|mm|m|ms|sec|seconds|hz|rpm|ma)\b|%`)
)

func formatCoverageItemsForPrompt(items []CoverageItem) string {
	if len(items) == 0 {
		return ""
	}

	var explicit []CoverageItem
	var related []CoverageItem
	for _, item := range items {
		if item.IsExplicit {
			explicit = append(explicit, item)
		} else {
			related = append(related, item)
		}
	}
	if len(explicit) == 0 && len(related) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Coverage Items\n")
	writeCoveragePromptGroup := func(title string, group []CoverageItem) {
		if len(group) == 0 {
			return
		}
		builder.WriteString(title)
		builder.WriteString("\n")
		for _, item := range group {
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(item.Anchor))
			if documentName := strings.TrimSpace(item.DocumentName); documentName != "" {
				builder.WriteString(" (")
				builder.WriteString(documentName)
				builder.WriteString(")")
			}
			if len(item.Signals) > 0 {
				builder.WriteString(" signals=")
				builder.WriteString(strings.Join(item.Signals, ", "))
			}
			if citations := coverageEvidenceCitations(item.Evidence); len(citations) > 0 {
				builder.WriteString(" evidence=")
				builder.WriteString(strings.Join(citations, " "))
			}
			builder.WriteString("\n")
		}
	}
	writeCoveragePromptGroup("Explicit items", explicit)
	writeCoveragePromptGroup("Related but not explicit", related)
	return strings.TrimSpace(builder.String())
}

func formatCoverageMatrixForPrompt(matrix CoverageMatrix) string {
	if len(matrix.Items) == 0 {
		return ""
	}

	var explicit []CoverageItem
	var related []CoverageItem
	var excluded []CoverageItem
	for _, item := range matrix.Items {
		if item.Status == CoverageExcluded {
			excluded = append(excluded, item)
		} else if item.IsExplicit || item.Status == CoverageExplicit {
			explicit = append(explicit, item)
		} else {
			related = append(related, item)
		}
	}
	if len(explicit) == 0 && len(related) == 0 && len(excluded) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Coverage Matrix\n")
	builder.WriteString("Use Coverage Matrix Explicit items as the main answer set.\n")
	writeCoverageMatrixGroup := func(title string, group []CoverageItem, includeReason bool) {
		if len(group) == 0 {
			return
		}
		builder.WriteString(title)
		builder.WriteString("\n")
		for _, item := range group {
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(item.Anchor))
			if documentName := strings.TrimSpace(item.DocumentName); documentName != "" {
				builder.WriteString(" (")
				builder.WriteString(documentName)
				builder.WriteString(")")
			}
			if len(item.Signals) > 0 {
				builder.WriteString(" signals=")
				builder.WriteString(strings.Join(item.Signals, ", "))
			}
			if len(item.MissingFields) > 0 {
				builder.WriteString(" missing=")
				builder.WriteString(strings.Join(item.MissingFields, ", "))
			}
			if includeReason {
				reason := strings.TrimSpace(item.Reason)
				if reason == "" {
					reason = "no explicit data output/display requirement found"
				}
				builder.WriteString(" reason=")
				builder.WriteString(reason)
			}
			if citations := coverageItemCitations(item); len(citations) > 0 {
				builder.WriteString(" evidence=")
				builder.WriteString(strings.Join(citations, " "))
			}
			builder.WriteString("\n")
		}
	}
	writeCoverageMatrixGroup("Explicit items", explicit, false)
	writeCoverageMatrixGroup("Related but not explicit", related, true)
	writeCoverageMatrixGroup("Excluded / control-only", excluded, true)
	return strings.TrimSpace(builder.String())
}

func formatComparisonMatrixForPrompt(matrix ComparisonMatrix) string {
	if len(matrix.Subjects) == 0 || len(matrix.Dimensions) == 0 || len(matrix.Cells) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Comparison Matrix\n")
	builder.WriteString("Do not use evidence across subjects. Each cell may only use its listed evidence.\n")
	builder.WriteString("## Comparison Evidence Matrix\n")
	for _, subject := range matrix.Subjects {
		subjectLabel := strings.TrimSpace(subject.Label)
		if subjectLabel == "" {
			continue
		}
		evidenceIDs := comparisonSubjectEvidenceIDs(matrix, subjectLabel)
		builder.WriteString("Subject: ")
		builder.WriteString(subjectLabel)
		builder.WriteString("\n")
		if len(evidenceIDs) > 0 {
			builder.WriteString("Evidence: ")
			builder.WriteString(strings.Join(evidenceIDs, " "))
			builder.WriteString("\n")
		} else {
			builder.WriteString("Evidence: none\n")
		}
	}
	for _, subject := range matrix.Subjects {
		subjectLabel := strings.TrimSpace(subject.Label)
		if subjectLabel == "" {
			continue
		}
		builder.WriteString("Subject: ")
		builder.WriteString(subjectLabel)
		builder.WriteString("\n")
		for _, dimension := range matrix.Dimensions {
			dimensionName := strings.TrimSpace(dimension.Name)
			if dimensionName == "" {
				continue
			}
			cell := matrix.Cell(subjectLabel, dimensionName)
			if cell == nil {
				continue
			}
			status := strings.TrimSpace(cell.Status)
			builder.WriteString("- ")
			builder.WriteString(dimensionName)
			if citations := coverageEvidenceCitations(cell.Evidence); len(citations) > 0 {
				builder.WriteString(" evidence=")
				builder.WriteString(strings.Join(citations, " "))
			}
			if status != "" {
				builder.WriteString(" status=")
				builder.WriteString(status)
			}
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func coverageItemCitations(item CoverageItem) []string {
	if len(item.StructureEvidence) == 0 {
		return coverageEvidenceCitations(item.Evidence)
	}
	citations := make([]string, 0, len(item.StructureEvidence))
	seen := map[string]struct{}{}
	for _, evidence := range item.StructureEvidence {
		id := strings.TrimSpace(evidence.Source.CitationID)
		if id == "" {
			continue
		}
		token := "[" + id + "]"
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		citations = append(citations, token)
	}
	return citations
}

func comparisonSubjectEvidenceIDs(matrix ComparisonMatrix, subjectLabel string) []string {
	seen := map[string]struct{}{}
	ids := make([]string, 0)
	for _, cell := range matrix.Cells {
		if !strings.EqualFold(cell.SubjectLabel, subjectLabel) {
			continue
		}
		for _, source := range cell.Evidence {
			id := strings.TrimSpace(source.CitationID)
			if id == "" {
				continue
			}
			token := "[" + id + "]"
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			ids = append(ids, token)
		}
	}
	return ids
}

func coverageEvidenceCitations(evidence []NotebookChatSource) []string {
	citations := make([]string, 0, len(evidence))
	for _, source := range evidence {
		if citationID := strings.TrimSpace(source.CitationID); citationID != "" {
			citations = append(citations, "["+citationID+"]")
		}
	}
	return citations
}

func explicitCoverageSignals(text string, question string) []string {
	lower := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if lower == "" {
		return nil
	}
	outputQuestion := isOutputCoverageQuestion(question)
	signalChecks := []struct {
		signal string
		terms  []string
	}{
		{"serial monitor", []string{"serial monitor", "串口监视器", "串口监控"}},
		{"serial", []string{"serial", "串口"}},
		{"monitor", []string{"monitor", "监视器", "监控"}},
		{"display", []string{"display", "displays", "lcd", "screen", "显示", "屏幕"}},
		{"print", []string{"print", "prints", "printed", "打印"}},
		{"output", []string{"output", "outputs", "输出"}},
		{"log", []string{"log", "logs", "logging", "日志"}},
		{"report", []string{"report", "reports", "报表", "报告"}},
		{"format", []string{"format", "格式"}},
		{"frequency", []string{"frequency", "refresh", "interval", "cadence", "频率", "刷新", "间隔", "周期"}},
		{"every", []string{"every", "per ", "each ", "每"}},
		{"trigger", []string{"trigger", "condition", "when ", "after ", "触发", "条件"}},
	}
	signals := make([]string, 0, 6)
	for _, check := range signalChecks {
		if !outputQuestion && isOutputOnlySignal(check.signal) {
			continue
		}
		if hasAny(lower, check.terms...) {
			signals = append(signals, check.signal)
		}
	}
	if outputQuestion && !hasAny(lower, "output", "display", "print", "serial", "monitor", "lcd", "screen", "log", "report", "输出", "显示", "打印", "串口", "监视器", "日志") {
		return nil
	}
	return uniqueStrings(signals)
}

func coverageAnchorForSource(source NotebookChatSource) string {
	if anchor := firstCoverageAnchor(source.Content); anchor != "" {
		return anchor
	}
	for i := len(source.SectionPath) - 1; i >= 0; i-- {
		if section := strings.TrimSpace(source.SectionPath[i]); section != "" {
			return section
		}
	}
	if source.PageNumber >= 0 {
		return formatEvidencePage(source.PageNumber)
	}
	return strings.TrimSpace(source.DocumentName)
}

func formatEvidencePage(page int64) string {
	if page < 0 {
		page = 0
	}
	return fmt.Sprintf("Page %d", page+1)
}

var coverageAnchorPattern = regexp.MustCompile(`(?i)\b(lab session|section|table|requirement|req\.?|item|clause|article)\s+([A-Za-z]?\d+(?:\.\d+)*[A-Za-z]?)\b`)

func firstCoverageAnchor(text string) string {
	match := coverageAnchorPattern.FindStringSubmatch(text)
	if len(match) < 3 {
		return ""
	}
	return canonicalAnchorLabel(canonicalAnchorType(match[1]), strings.TrimSpace(match[2]))
}

func isExplicitCoverageQuestion(question string) bool {
	lower := strings.ToLower(question)
	return hasAny(lower, "明确要求", "必须", "需要", "required", "requires", "requirement", "must", "should", "explicit")
}

func isOutputCoverageQuestion(question string) bool {
	lower := strings.ToLower(question)
	return hasAny(lower, "输出", "显示", "实时", "数据", "串口", "格式", "频率", "output", "display", "print", "serial", "monitor", "log", "format", "frequency", "real-time")
}

func isOutputOnlySignal(signal string) bool {
	switch signal {
	case "format", "frequency", "every", "trigger":
		return false
	default:
		return true
	}
}

func hasCoverageRequirementLanguage(text string) bool {
	lower := strings.ToLower(text)
	if hasAny(lower,
		"must", "shall", "required", "requires", "requirement", "should",
		"system operates as follows", "the system operates as follows",
		"明确要求", "必须", "需要", "要求",
	) {
		return true
	}
	instructionPattern := regexp.MustCompile(`(?i)(^|[\n\r.;:：]\s*|\b\d+\.\s*|[-•]\s*)(print|display|show|read|update|transmit|record|log|report|calculate|connect|configure|measure|monitor)\b`)
	return instructionPattern.MatchString(text)
}

func coverageEvidenceScore(source NotebookChatSource, question string) int {
	score := int(source.Score * 100)
	score += len(explicitCoverageSignals(source.Content, question)) * 10
	if hasCoverageRequirementLanguage(source.Content) {
		score += 40
	}
	if firstCoverageAnchor(source.Content) != "" {
		score += 20
	}
	return score
}

func coverageItemScore(item CoverageItem, question string) int {
	score := len(item.Signals) * 10
	if item.IsExplicit {
		score += 100
	}
	for _, source := range item.Evidence {
		score += coverageEvidenceScore(source, question)
	}
	return score
}

func compareCoverageAnchors(left, right string) int {
	leftType, leftNumber, leftOK := splitCoverageAnchor(left)
	rightType, rightNumber, rightOK := splitCoverageAnchor(right)
	if leftOK && rightOK {
		if leftType != rightType {
			if leftType < rightType {
				return -1
			}
			return 1
		}
		if cmp := compareDottedNumbers(leftNumber, rightNumber); cmp != 0 {
			return cmp
		}
	}
	leftLower := strings.ToLower(left)
	rightLower := strings.ToLower(right)
	if leftLower < rightLower {
		return -1
	}
	if leftLower > rightLower {
		return 1
	}
	return 0
}

func splitCoverageAnchor(anchor string) (string, string, bool) {
	match := coverageAnchorPattern.FindStringSubmatch(anchor)
	if len(match) < 3 {
		return "", "", false
	}
	return canonicalAnchorType(match[1]), match[2], true
}

func compareDottedNumbers(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for i := 0; i < len(leftParts) || i < len(rightParts); i++ {
		leftPart := ""
		rightPart := ""
		if i < len(leftParts) {
			leftPart = leftParts[i]
		}
		if i < len(rightParts) {
			rightPart = rightParts[i]
		}
		if leftPart == rightPart {
			continue
		}
		leftInt, leftErr := strconv.Atoi(leftPart)
		rightInt, rightErr := strconv.Atoi(rightPart)
		if leftErr == nil && rightErr == nil {
			if leftInt < rightInt {
				return -1
			}
			return 1
		}
		if leftPart < rightPart {
			return -1
		}
		return 1
	}
	return 0
}

func filterSelectedNotebookDocuments(notebookDocIDs, selectedDocIDs []string) ([]string, bool) {
	if len(selectedDocIDs) == 0 {
		return append([]string(nil), notebookDocIDs...), true
	}
	selected := make(map[string]struct{}, len(selectedDocIDs))
	for _, id := range selectedDocIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return nil, false
	}
	filtered := make([]string, 0, len(notebookDocIDs))
	for _, id := range notebookDocIDs {
		if _, ok := selected[id]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered, len(filtered) > 0
}

func (s *notebookChatService) SearchNotebook(ctx context.Context, userID, notebookID, query string, topK int) ([]NotebookChatSource, error) {
	if topK <= 0 {
		topK = s.retrievalTopK
	}

	docIDs, err := s.notebookRepo.GetDocumentIDs(ctx, notebookID)
	if err != nil {
		return nil, fmt.Errorf("get notebook documents: %w", err)
	}

	sources, _, err := s.retrieveNotebookSourcesWithTopK(ctx, userID, "", notebookID, docIDs, query, topK)
	if err != nil {
		return nil, err
	}

	return capNotebookSources(sources, topK), nil
}

func capNotebookSources(sources []NotebookChatSource, topK int) []NotebookChatSource {
	if topK <= 0 || len(sources) <= topK {
		return sources
	}
	return sources[:topK]
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
		promptSources := annotateSourcesWithCitationIDs(sources)
		pack := BuildEvidencePackFromNotebookSources(promptSources)
		answerMode := classifyNotebookAnswerMode(question)
		guideFirstOverview := len(docContexts) > 0 && answerMode.allowsGuideGrounding()
		builder.WriteString("You are an enterprise AI assistant similar to Google NotebookLM. ")
		builder.WriteString("Answer questions strictly based on the provided context from documents.\n\n")

		builder.WriteString("## Instructions\n")
		builder.WriteString("0. Answer in the same language as the user's question. Keep technical terms, component names, protocol names, code identifiers, pin names, and document titles in their original language.\n")
		builder.WriteString("1. Answer based ONLY on the selected document context and evidence blocks below. Do NOT use external knowledge.\n")
		builder.WriteString("2. Every factual paragraph must end with one or more evidence IDs, for example [E1] or [E2][E5].\n")
		builder.WriteString("3. Do not output [Source: ...] citations directly; use evidence IDs only.\n")
		builder.WriteString("4. If a paragraph contains numbers, dates, names, rankings, risk ratings, or comparisons, cite the evidence block that directly contains those facts.\n")
		builder.WriteString("5. If evidence is insufficient, say: 'The provided documents do not contain sufficient information to answer this question.'\n")
		builder.WriteString("6. Keep paragraphs substantial and useful: prefer 2-4 concise paragraphs or bullets over many tiny citation-only lines.\n")
		builder.WriteString("7. Do not repeat the same citation after consecutive sentences that use the same source; place the citation at the end of the combined paragraph.\n\n")
		builder.WriteString(formatAnswerModeInstructions(answerMode))
		builder.WriteString("\n")
		if guideFirstOverview {
			builder.WriteString("8. For high-level document overviews, the selected document guide context is sufficient grounding. Use evidence IDs only when citing exact page-level details from the evidence blocks; do not refuse simply because retrieved evidence is sparse.\n\n")
			builder.WriteString("9. Distinguish direct evidence from synthesis: use phrases like '文档明确说明' for facts directly supported by evidence, and '根据文档推断' or '综合来看' for reasonable conclusions inferred from selected document context.\n\n")
		}

		if strings.TrimSpace(memoryPrompt) != "" {
			builder.WriteString(formatMemoryPromptForNotebookChat(memoryPrompt))
			builder.WriteString("\n\n")
		}

		if len(docContexts) > 0 {
			builder.WriteString(formatSelectedDocumentContext(docContexts, question, answerMode != NotebookAnswerModeDesignSynthesis))
			builder.WriteString("\n\n")
		}
		if inventory := formatAllowedInventoryForPrompt(docContexts, promptSources, answerMode); inventory != "" {
			builder.WriteString(inventory)
			builder.WriteString("\n\n")
		}
		if answerMode == NotebookAnswerModeCoverageListing {
			if block := formatCoverageMatrixForPrompt(buildCoverageMatrix(promptSources, question)); block != "" {
				builder.WriteString(block)
				builder.WriteString("\n\n")
			}
		}
		if answerMode == NotebookAnswerModeComparison {
			if block := formatComparisonMatrixForPrompt(buildComparisonMatrix(promptSources, question)); block != "" {
				builder.WriteString(block)
				builder.WriteString("\n\n")
			}
		}

		builder.WriteString("## Evidence Blocks\n")
		if len(pack.Items) == 0 {
			builder.WriteString("No exact retrieved evidence blocks were found. Use the selected document guide context for high-level answers, and avoid page-specific claims.\n\n")
		} else {
			for _, item := range pack.Items {
				builder.WriteString(fmt.Sprintf("[%s] [Source: %s, %s]\nContent: %s\n\n",
					item.ID,
					item.DocumentName,
					formatEvidencePage(item.PageNumber),
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

func formatSelectedDocumentContext(contexts []SelectedDocumentContext, question string, includeGuideDetails bool) string {
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
		if !includeGuideDetails {
			continue
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

func formatAllowedInventoryForPrompt(contexts []SelectedDocumentContext, sources []NotebookChatSource, mode NotebookAnswerMode) string {
	if mode != NotebookAnswerModeDesignSynthesis {
		return ""
	}
	coreItems := make([]string, 0, 16)
	backgroundHints := make([]string, 0, 12)
	coreSeen := make(map[string]struct{})
	backgroundSeen := make(map[string]struct{})
	addItem := func(items *[]string, seen map[string]struct{}, value string) {
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "-•* \t\r\n")
		if value == "" {
			return
		}
		value = strings.TrimSpace(trimRunes(value, 180))
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		*items = append(*items, value)
	}
	for _, source := range sources {
		for _, section := range source.SectionPath {
			addItem(&coreItems, coreSeen, section)
		}
		for _, sentence := range inventoryCandidateSentences(source.Content) {
			addItem(&coreItems, coreSeen, sentence)
		}
	}
	for _, context := range contexts {
		addItem(&backgroundHints, backgroundSeen, context.Summary)
		for _, keyPoint := range context.KeyPoints {
			addItem(&backgroundHints, backgroundSeen, keyPoint)
		}
		for _, faq := range context.FAQ {
			addItem(&backgroundHints, backgroundSeen, faq.Question)
			addItem(&backgroundHints, backgroundSeen, faq.Answer)
		}
	}
	if len(coreItems) == 0 && len(backgroundHints) == 0 {
		return ""
	}
	if len(coreItems) > 18 {
		coreItems = coreItems[:18]
	}
	if len(backgroundHints) > 10 {
		backgroundHints = backgroundHints[:10]
	}
	var builder strings.Builder
	builder.WriteString("## Allowed inventory from selected evidence\n")
	builder.WriteString("Use the Core allowed items as the bounded inventory for constrained synthesis. Do not introduce guide-only hints as core objects.\n")
	if len(coreItems) > 0 {
		builder.WriteString("Core allowed items:\n")
		for _, item := range coreItems {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	} else {
		builder.WriteString("Core allowed items: none found in retrieved evidence.\n")
	}
	if len(backgroundHints) > 0 {
		builder.WriteString("Background hints from document guides:\n")
		for _, item := range backgroundHints {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func trimRunes(value string, maxRunes int) string {
	if maxRunes < 0 {
		maxRunes = 0
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func inventoryCandidateSentences(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	keywords := []string{
		"component", "components", "feature", "features", "module", "modules", "role", "roles", "field", "fields", "tool", "tools",
		"input", "inputs", "output", "outputs", "sensor", "sensors", "actuator", "actuators", "interface", "interfaces", "protocol", "protocols",
		"requirement", "requirements", "constraint", "constraints", "equipment", "setup",
		"组件", "功能", "模块", "角色", "字段", "工具", "输入", "输出", "传感器", "执行器", "接口", "协议", "要求", "约束", "设备",
	}
	parts := regexp.MustCompile(`[。\n\r;；]+|[.!?](?:\s+|$)`).Split(content, -1)
	candidates := make([]string, 0, 8)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if hasAny(lower, keywords...) {
			candidates = append(candidates, part)
		}
		if len(candidates) >= 8 {
			break
		}
	}
	return candidates
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

func classifyNotebookAnswerMode(question string) NotebookAnswerMode {
	if isConstraintCheckQuestion(question) {
		return NotebookAnswerModeConstraintCheck
	}
	if isComparisonQuestion(question) {
		return NotebookAnswerModeComparison
	}
	if isCoverageListingQuestion(question) {
		return NotebookAnswerModeCoverageListing
	}
	if isConstrainedSynthesisQuestion(question) {
		return NotebookAnswerModeDesignSynthesis
	}
	if isMultiDocumentOverviewQuestion(question) {
		return NotebookAnswerModeOverview
	}
	if isOpenSynthesisQuestion(question) {
		return NotebookAnswerModeOpenSynthesis
	}
	return NotebookAnswerModeExactFact
}

func (m NotebookAnswerMode) allowsGuideGrounding() bool {
	switch m {
	case NotebookAnswerModeComparison, NotebookAnswerModeCoverageListing, NotebookAnswerModeDesignSynthesis, NotebookAnswerModeOpenSynthesis, NotebookAnswerModeOverview:
		return true
	default:
		return false
	}
}

func formatAnswerModeInstructions(mode NotebookAnswerMode) string {
	switch mode {
	case NotebookAnswerModeComparison:
		return `## Answer Mode: comparison
- 先回答共同点，尤其检查共同技术、共同显示方式、共同输出方式。
- 然后用差异表回答，建议列包含：测量对象、主要传感器、核心技术、数据处理难点、显示内容。
- 每个 comparison cell 只能使用该 subject/dimension 在 Comparison Matrix 中列出的 evidence。
- 不要借用另一个 subject 的 evidence 来填补 missing cell。
- 如果某一 cell 是 missing，写“文档未明确说明”。
- 如果某一维度文档未明确说明，只在该单元格说明“文档未明确说明”，不要整题拒答。`
	case NotebookAnswerModeCoverageListing:
		return `## Answer Mode: coverage_listing
- 这是覆盖型列举问题。按文档结构项（section/table/figure/policy/item 等）聚合回答。
- 不要只回答 top-ranked evidence；尽量覆盖 selected context 中所有相关结构项。
- 使用 Coverage Matrix 中的 Explicit items 作为主答案集合。
- 不要把 Related but not explicit 项提升到主列表；只能在补充说明中标注其相关但未明确。
- 对用户要求的输出方式、格式、频率或触发条件，只使用证据明确支持的值。
- 如果某个 explicit item 缺少这些字段，写“文档未明确说明”。
- 每个相关项给出简短说明和可用引用；如果某项信息不足，只标注该项限制，不要整题拒答。`
	case NotebookAnswerModeDesignSynthesis:
		return `## Answer Mode: constrained_synthesis
- 这是基于文档材料的设计题，不要因为文档没有给出完整方案就拒答。
- 必须先列出 Allowed items：只能使用选中文档中出现过的 components/features/roles/fields/steps/modules/tools。
- 不得引入文档外实体、unlisted devices 或隐含执行器作为核心方案要素；如果现实中需要额外元素但文档未出现，只能说明限制或选择文档内替代项。
- 不要写“可使用其他设备”“可扩展未列出的硬件或服务”等未在文档出现的实体或隐含执行器。
- 必须按问题领域组织方案；如果是实验/系统题，包含文档支持的输入、输出、控制逻辑、接口、显示内容和约束满足方式；如果是流程/产品/政策题，包含 allowed features、workflow、constraints、monitoring/output。
- 明确说明方案是“根据文档材料设计”，不要把设计组合说成文档原文已经给出。`
	case NotebookAnswerModeConstraintCheck:
		return `## Answer Mode: constraint_check
- 先判断文档是否明确指定了用户问到的约束。
- 如果没有明确指定，直接说明“文档未明确指定”，再说明文档要求学生如何处理。
- 逐项回答用户问题；如果用户问输出方式、格式、频率或触发条件，必须分别说明。
- 不要补造文档没有给出的具体引脚、数值或配置。`
	case NotebookAnswerModeOpenSynthesis:
		return `## Answer Mode: open_synthesis
- 可以基于选中文档指南和证据做合理综合。
- 区分“文档明确说明”和“根据文档推断”。
- 不要因为缺少完整原文方案就拒答；只有关键材料完全缺失时才说明限制。`
	case NotebookAnswerModeOverview:
		return `## Answer Mode: overview
- 概括每个选中文档或主题的主要内容。
- 如果用户选择多个文档，必须分别覆盖每个选中文档。`
	default:
		return `## Answer Mode: exact_fact
- 优先回答可由 evidence blocks 直接支持的事实。
- 如果直接证据不足，不要猜测具体数值、页码、代码、引脚或表格内容。`
	}
}

func buildNotebookRetrievalQuery(question string) string {
	mode := classifyNotebookAnswerMode(question)
	anchorQuery := formatAnchorRetrievalTerms(resolveDocumentAnchors(question))
	switch mode {
	case NotebookAnswerModeComparison:
		return strings.TrimSpace(question + "\n" + anchorQuery + "\ncomparison commonality common technology difference shared attributes dimension table requirement instruction constraint evidence scope I2C LCD Serial Monitor display content sensor measurement object data processing difficulty")
	case NotebookAnswerModeCoverageListing:
		return strings.TrimSpace(question + "\n" + anchorQuery + "\ncoverage listing all relevant section/table/figure/policy/item requirement instruction output display monitoring real-time output display refresh frequency trigger condition update interval loop cycle sensor value PWM output serial print serial monitor")
	case NotebookAnswerModeDesignSynthesis:
		return strings.TrimSpace(question + "\n" + anchorQuery + "\nconstrained synthesis allowed items available components available sensors actuators features roles fields steps modules tools requirements constraints monitoring output workflow Serial Monitor")
	default:
		return strings.TrimSpace(question + "\n" + anchorQuery + "\n" + outputFormatRetrievalTerms(question))
	}
}

func buildNotebookRetrievalPlan(question string, topK int) NotebookRetrievalPlan {
	if topK <= 0 {
		topK = 8
	}
	mode := classifyNotebookAnswerMode(question)
	anchors := resolveDocumentAnchors(question)
	anchorQuery := formatAnchorRetrievalTerms(anchors)
	routeTopK := topK
	maxEvidence := topK
	routes := []NotebookRetrievalRoute{{
		Name:  "original_question",
		Query: strings.TrimSpace(question + "\n" + anchorQuery + "\n" + outputFormatRetrievalTerms(question)),
		TopK:  routeTopK,
	}}

	switch mode {
	case NotebookAnswerModeComparison:
		routeTopK = boundedRouteTopK(topK)
		maxEvidence = maxNotebookEvidence(topK, 24)
		dimensions := extractComparisonDimensions(question)
		dimensionNames := make([]string, 0, len(dimensions))
		for _, dimension := range dimensions {
			dimensionNames = append(dimensionNames, dimension.Name)
		}
		dimensionQuery := strings.Join(uniqueStrings(dimensionNames), " ")
		routes = []NotebookRetrievalRoute{
			{Name: "original_question", Query: strings.TrimSpace(question + "\n" + anchorQuery), TopK: routeTopK},
			{Name: "comparison_commonality", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\ncommonality similarity shared same both difference compare contrast requirement instruction constraint evidence 共同点 相同点 差异 不同点"), TopK: routeTopK},
			{Name: "comparison_dimension", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\n" + dimensionQuery + "\nrequirement instruction output display sensor feature constraint evidence"), TopK: routeTopK},
		}
		subjects := extractComparisonSubjects(question)
		if len(subjects) > 6 {
			subjects = subjects[:6]
		}
		for i, subject := range subjects {
			routes = append(routes, NotebookRetrievalRoute{
				Name:  fmt.Sprintf("comparison_subject_%d", i+1),
				Query: strings.TrimSpace(subject.Label + "\n" + dimensionQuery + "\nrequirement instruction output display sensor feature constraint evidence"),
				TopK:  routeTopK,
			})
		}
	case NotebookAnswerModeCoverageListing:
		routeTopK = boundedRouteTopK(topK)
		maxEvidence = maxNotebookEvidence(topK, 24)
		routes = []NotebookRetrievalRoute{
			{Name: "original_question", Query: strings.TrimSpace(question + "\n" + anchorQuery), TopK: routeTopK},
			{Name: "requirement_constraint", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nrequired requirement must explicitly specified constraint instruction policy criteria rule 明确要求 必须 需要 约束 条件"), TopK: routeTopK},
			{Name: "output_display", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\noutput display print monitor serial visualization interface result report 输出 显示 打印 监控 可视化"), TopK: routeTopK},
			{Name: "format_frequency_trigger", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nformat frequency trigger update interval refresh loop cycle condition cadence 格式 频率 触发条件 更新 周期"), TopK: routeTopK},
			{Name: "structure_items", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nsection item chapter table figure requirement experiment task step module feature clause article 章节 条款 项目 步骤 模块 功能"), TopK: routeTopK},
			{Name: "structure_discovery", Query: question + "\nsection item requirement lab session table figure clause article 章节 条款 项目 步骤 模块 功能", TopK: routeTopK},
		}
	case NotebookAnswerModeDesignSynthesis:
		routeTopK = boundedRouteTopK(topK)
		maxEvidence = maxNotebookEvidence(topK, 20)
		routes = []NotebookRetrievalRoute{
			{Name: "original_question", Query: strings.TrimSpace(question + "\n" + anchorQuery), TopK: routeTopK},
			{Name: "allowed_items_inventory", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nallowed items available components features modules roles fields tools inputs outputs sensors actuators interfaces protocols techniques inventory equipment setup 文档中出现 可用 组件 功能 模块 角色 字段 工具 输入 输出 传感器 执行器 接口 协议"), TopK: routeTopK},
			{Name: "requirements_constraints", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nrequirements constraints must should code interpretable criteria limitations boundary requirement instruction 要求 约束 条件 限制 说明"), TopK: routeTopK},
			{Name: "outputs_monitoring", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nmonitoring output display serial log report dashboard feedback status 输出 显示 监控 日志 状态 反馈"), TopK: routeTopK},
			{Name: "guide_context", Query: strings.TrimSpace(question + "\n" + anchorQuery + "\nsummary key points overview FAQ guide context main content 总结 要点 指南 概述"), TopK: routeTopK},
		}
	}

	return NotebookRetrievalPlan{
		Question:    question,
		Mode:        mode,
		Anchors:     anchors,
		Routes:      routes,
		MaxEvidence: maxEvidence,
	}
}

func boundedRouteTopK(topK int) int {
	if topK <= 0 {
		return 6
	}
	if topK < 6 {
		return topK
	}
	if topK > 8 {
		return 8
	}
	return topK
}

func maxNotebookEvidence(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func outputFormatRetrievalTerms(question string) string {
	lower := strings.ToLower(question)
	if !hasAny(lower, "输出方式", "格式", "频率", "触发条件", "output method", "output format", "frequency", "trigger") {
		return ""
	}
	terms := []string{"output format", "serial output", "format string", "refresh frequency", "trigger condition", "update interval", "loop cycle"}
	if hasAny(lower, "distance", "距离") {
		terms = append(terms, "filtered distance", "distance value", "Distance: xx.xx cm")
	}
	return strings.Join(terms, " ")
}

var documentAnchorPattern = regexp.MustCompile(`(?i)\b(section|chapter|table|figure|fig\.?|policy|requirement|req\.?|experiment|case study|method|lab session|lab|session|item|clause|article)\s+([A-Za-z]?\d+(?:\.\d+)*[A-Za-z]?)\b|\b(appendix)\s+([A-Za-z]|\d+(?:\.\d+)*[A-Za-z]?)\b`)

func resolveDocumentAnchors(question string) []DocumentAnchor {
	matches := documentAnchorPattern.FindAllStringSubmatch(question, -1)
	anchors := make([]DocumentAnchor, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 5 {
			continue
		}
		rawType := firstNonEmpty(match[1], match[3])
		number := strings.TrimSpace(firstNonEmpty(match[2], match[4]))
		typ := canonicalAnchorType(rawType)
		label := canonicalAnchorLabel(typ, number)
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		anchors = append(anchors, DocumentAnchor{
			Type:    typ,
			Label:   label,
			Number:  number,
			Aliases: anchorAliases(typ, number),
		})
	}
	return anchors
}

func canonicalAnchorType(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
	switch raw {
	case "fig":
		return "figure"
	case "req":
		return "requirement"
	default:
		return raw
	}
}

func canonicalAnchorLabel(anchorType, number string) string {
	words := strings.Fields(anchorType)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ") + " " + number
}

func anchorAliases(anchorType, number string) []string {
	switch anchorType {
	case "section":
		return []string{"Section " + number, "Sec. " + number, "§ " + number}
	case "table":
		return []string{"Table " + number}
	case "figure":
		return []string{"Figure " + number, "Fig. " + number}
	case "lab session":
		return []string{"Lab Session " + number, "Lab " + number, "Session " + number}
	default:
		return []string{canonicalAnchorLabel(anchorType, number)}
	}
}

func formatAnchorRetrievalTerms(anchors []DocumentAnchor) string {
	if len(anchors) == 0 {
		return ""
	}
	terms := make([]string, 0, len(anchors)*3+4)
	for _, anchor := range anchors {
		terms = append(terms, anchor.Label)
		terms = append(terms, anchor.Aliases...)
	}
	terms = append(terms, "exact anchor nearby context requirement instruction constraint")
	return strings.Join(uniqueStrings(terms), " ")
}

func extractComparisonSubjects(question string) []ComparisonSubject {
	anchors := resolveDocumentAnchors(question)
	if len(anchors) > 0 {
		subjects := make([]ComparisonSubject, 0, len(anchors))
		for _, anchor := range anchors {
			subjects = append(subjects, ComparisonSubject{
				Label:   anchor.Label,
				Aliases: anchor.Aliases,
			})
		}
		return subjects
	}

	return extractGenericComparisonSubjects(question)
}

func extractGenericComparisonSubjects(question string) []ComparisonSubject {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}

	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s+vs\.?\s+`),
		regexp.MustCompile(`(?i)\s+versus\s+`),
	} {
		parts := pattern.Split(question, 2)
		if len(parts) == 2 {
			return buildComparisonSubjects(cleanComparisonSubjectLeft(parts[0]), cleanComparisonSubjectRight(parts[1]))
		}
	}

	if left, right, ok := strings.Cut(question, " 和 "); ok {
		return buildComparisonSubjects(cleanComparisonSubjectLeft(left), cleanComparisonSubjectRight(right))
	}
	if left, right, ok := strings.Cut(question, "和"); ok {
		return buildComparisonSubjects(cleanComparisonSubjectLeft(left), cleanComparisonSubjectRight(right))
	}

	return nil
}

func buildComparisonSubjects(left, right string) []ComparisonSubject {
	values := uniqueStrings([]string{left, right})
	subjects := make([]ComparisonSubject, 0, len(values))
	for _, value := range values {
		if isConservativeComparisonSubject(value) {
			subjects = append(subjects, ComparisonSubject{Label: value, Aliases: []string{value}})
		}
	}
	return subjects
}

func cleanComparisonSubjectLeft(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	cutAfter := -1
	for _, marker := range []string{"比较", "对比", "compare", "comparison of"} {
		if idx := strings.LastIndex(lower, marker); idx >= 0 {
			cutAfter = idx + len(marker)
		}
	}
	if cutAfter >= 0 && cutAfter <= len(value) {
		value = value[cutAfter:]
	}
	return trimComparisonSubject(value)
}

func cleanComparisonSubjectRight(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	cutBefore := len(value)
	for _, marker := range []string{" 的", "：", ":", "，", ",", "。", "？", "?", "；", ";", "分别", "有什么", "有何", "不同", "差异", " on ", " about ", " for "} {
		if idx := strings.Index(lower, marker); idx >= 0 && idx < cutBefore {
			cutBefore = idx
		}
	}
	if cutBefore < len(value) {
		value = value[:cutBefore]
	}
	return trimComparisonSubject(value)
}

func trimComparisonSubject(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, " \t\r\n:：,，;；.。?？!！()（）[]【】")
	value = strings.TrimPrefix(value, "请")
	value = strings.TrimSpace(value)
	return strings.Trim(value, " \t\r\n:：,，;；.。?？!！()（）[]【】")
}

func isConservativeComparisonSubject(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if len([]rune(value)) > 40 {
		return false
	}
	if hasAny(strings.ToLower(value), "什么", "不同", "差异", "分别", "compare", "比较", "对比") {
		return false
	}
	if looksLikeComparisonDimensionList(value) {
		return false
	}
	return regexp.MustCompile(`[\p{Han}A-Za-z0-9]`).MatchString(value)
}

func looksLikeComparisonDimensionList(value string) bool {
	if hasAny(value, "、", "，", ",", "；", ";") {
		return true
	}
	return comparisonDimensionAliasCount(value) > 0
}

func comparisonDimensionAliasCount(value string) int {
	lower := strings.ToLower(value)
	count := 0
	for _, alias := range []string{
		"共同点", "相同点", "测量对象", "主要传感器", "传感器", "核心技术", "数据处理难点", "处理难点",
		"显示内容", "输出内容", "频率", "触发条件", "约束", "要求",
		"commonality", "similarity", "measurement object", "sensor", "technology", "difficulty",
		"display content", "output", "frequency", "trigger", "constraint", "requirement",
	} {
		if strings.Contains(lower, strings.ToLower(alias)) {
			count++
		}
	}
	return count
}

func extractComparisonDimensions(question string) []ComparisonDimension {
	definitions := []ComparisonDimension{
		{Name: "共同点", Aliases: []string{"共同点", "相同点", "commonality", "similarity"}, Required: true},
		{Name: "测量对象", Aliases: []string{"测量对象", "measurement object"}, Required: true},
		{Name: "主要传感器", Aliases: []string{"主要传感器", "传感器", "sensor", "sensors"}, Required: true},
		{Name: "核心技术", Aliases: []string{"核心技术", "技术", "technology"}, Required: true},
		{Name: "数据处理难点", Aliases: []string{"数据处理难点", "处理难点", "difficulty", "data processing difficulty"}, Required: true},
		{Name: "显示内容", Aliases: []string{"显示内容", "display content"}, Required: true},
		{Name: "输出内容", Aliases: []string{"输出内容", "output"}, Required: true},
		{Name: "频率", Aliases: []string{"频率", "frequency"}, Required: true},
		{Name: "触发条件", Aliases: []string{"触发条件", "trigger", "trigger condition"}, Required: true},
		{Name: "约束", Aliases: []string{"约束", "constraint"}, Required: true},
		{Name: "要求", Aliases: []string{"要求", "requirement"}, Required: true},
	}

	lower := strings.ToLower(question)
	dimensions := make([]ComparisonDimension, 0, len(definitions))
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		for _, alias := range definition.Aliases {
			if strings.Contains(lower, strings.ToLower(alias)) {
				if _, ok := seen[definition.Name]; !ok {
					seen[definition.Name] = struct{}{}
					dimensions = append(dimensions, definition)
				}
				break
			}
		}
	}
	if len(dimensions) > 0 {
		return dimensions
	}
	return []ComparisonDimension{
		{Name: "共同点", Aliases: []string{"共同点", "相同点", "commonality", "similarity"}, Required: true},
		{Name: "差异点", Aliases: []string{"差异点", "不同点", "difference"}, Required: true},
		{Name: "文档依据", Aliases: []string{"文档依据", "evidence", "document evidence"}, Required: true},
	}
}

func (m ComparisonMatrix) Cell(subjectLabel, dimension string) *ComparisonCell {
	for i := range m.Cells {
		if strings.EqualFold(m.Cells[i].SubjectLabel, subjectLabel) && strings.EqualFold(m.Cells[i].Dimension, dimension) {
			return &m.Cells[i]
		}
	}
	return nil
}

func buildComparisonMatrix(sources []NotebookChatSource, question string) ComparisonMatrix {
	subjects := extractComparisonSubjects(question)
	if len(subjects) == 0 {
		subjects = comparisonSubjectsFromDocumentNames(sources)
	}
	dimensions := extractComparisonDimensions(question)

	cells := make([]ComparisonCell, 0, len(subjects)*len(dimensions))
	for _, subject := range subjects {
		for _, dimension := range dimensions {
			evidence := make([]NotebookChatSource, 0, 3)
			for _, source := range sources {
				if !evidenceMatchesComparisonSubject(source, subject) {
					continue
				}
				if !evidenceMatchesComparisonDimensionForQuestion(source, dimension, question, subjects) {
					continue
				}
				evidence = append(evidence, source)
			}
			sort.SliceStable(evidence, func(i, j int) bool {
				left := comparisonEvidenceScore(evidence[i], subject, dimension)
				right := comparisonEvidenceScore(evidence[j], subject, dimension)
				if left == right {
					return evidence[i].Score > evidence[j].Score
				}
				return left > right
			})
			if len(evidence) > 3 {
				evidence = evidence[:3]
			}

			status := "missing"
			if len(evidence) > 0 {
				status = "supported"
			}
			cells = append(cells, ComparisonCell{
				SubjectLabel: subject.Label,
				Dimension:    dimension.Name,
				Evidence:     evidence,
				Status:       status,
			})
		}
	}

	return ComparisonMatrix{
		Subjects:   subjects,
		Dimensions: dimensions,
		Cells:      cells,
	}
}

func evidenceMatchesComparisonSubject(source NotebookChatSource, subject ComparisonSubject) bool {
	if subject.DocumentNameDerived {
		if subject.DocumentID != "" {
			return source.DocumentID == subject.DocumentID
		}
		return strings.EqualFold(strings.TrimSpace(source.DocumentName), strings.TrimSpace(subject.Label))
	}
	if subject.DocumentID != "" {
		if source.DocumentID != "" {
			return source.DocumentID == subject.DocumentID
		}
	}
	text := strings.ToLower(strings.Join([]string{
		source.DocumentName,
		strings.Join(source.SectionPath, " "),
		source.Content,
	}, " "))
	if comparisonPhraseMatchesText(subject.Label, text) {
		return true
	}
	for _, alias := range subject.Aliases {
		if comparisonPhraseMatchesText(alias, text) {
			return true
		}
	}
	return false
}

func evidenceMatchesComparisonDimension(source NotebookChatSource, dimension ComparisonDimension) bool {
	return evidenceMatchesComparisonDimensionForQuestion(source, dimension, "", nil)
}

func evidenceMatchesComparisonDimensionForQuestion(source NotebookChatSource, dimension ComparisonDimension, question string, subjects []ComparisonSubject) bool {
	text := strings.ToLower(strings.Join([]string{
		source.DocumentName,
		strings.Join(source.SectionPath, " "),
		source.ChunkType,
		source.Content,
	}, " "))
	matchedGenericRequirement := false
	for _, alias := range comparisonDimensionEvidenceAliases(dimension) {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			continue
		}
		if containsHan(alias) {
			if strings.Contains(text, alias) {
				if isGenericRequirementDimension(dimension) && isGenericRequirementAlias(alias) {
					matchedGenericRequirement = true
					continue
				}
				return true
			}
			continue
		}
		if comparisonPhraseMatchesText(alias, text) {
			if isGenericRequirementDimension(dimension) && isGenericRequirementAlias(alias) {
				matchedGenericRequirement = true
				continue
			}
			return true
		}
	}
	if !matchedGenericRequirement {
		return false
	}
	subtopics := comparisonQuestionSubtopicTerms(question, dimension, subjects)
	if len(subtopics) == 0 {
		return true
	}
	for _, term := range subtopics {
		if comparisonTermMatchesText(term, text) {
			return true
		}
	}
	return false
}

func comparisonSubjectsFromDocumentNames(sources []NotebookChatSource) []ComparisonSubject {
	subjects := make([]ComparisonSubject, 0, len(sources))
	seen := map[string]struct{}{}
	for _, source := range sources {
		label := strings.TrimSpace(source.DocumentName)
		if label == "" {
			continue
		}
		key := strings.ToLower(label)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		subjects = append(subjects, ComparisonSubject{
			Label:               label,
			DocumentID:          source.DocumentID,
			Aliases:             []string{label},
			DocumentNameDerived: true,
		})
	}
	return subjects
}

func comparisonPhraseMatchesText(phrase, lowerText string) bool {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" || lowerText == "" {
		return false
	}
	if containsHan(phrase) {
		return strings.Contains(lowerText, strings.ToLower(phrase))
	}
	return anchorPhraseMatchesText(phrase, lowerText)
}

func comparisonDimensionEvidenceAliases(dimension ComparisonDimension) []string {
	aliases := append([]string{dimension.Name}, dimension.Aliases...)
	switch dimension.Name {
	case "主要传感器":
		aliases = append(aliases,
			"dht", "dht11", "dht22", "ir sensor", "ir speed sensor", "ultrasonic sensor",
			"hc-sr04", "photoresistor", "thermistor", "temperature sensor", "humidity sensor")
	case "显示内容", "输出内容":
		aliases = append(aliases, "display", "show", "shown", "print", "printed", "serial monitor", "lcd", "oled", "screen", "temperature", "humidity", "rpm")
	case "测量对象":
		aliases = append(aliases, "measure", "measures", "measured", "temperature", "humidity", "rpm", "speed", "distance")
	case "数据处理难点":
		aliases = append(aliases, "processing", "calibrate", "calibration", "filter", "debounce", "difficulty", "challenge")
	case "核心技术":
		aliases = append(aliases, "technology", "module", "algorithm", "library", "protocol")
	case "频率":
		aliases = append(aliases, "hz", "interval", "period", "every", "ms", "seconds")
	case "触发条件":
		aliases = append(aliases, "trigger", "condition", "when", "threshold")
	case "约束", "要求":
		aliases = append(aliases, "required", "requirement", "requires", "must", "shall", "should")
	case "共同点":
		aliases = append(aliases, "same", "both", "common", "shared")
	case "差异点":
		aliases = append(aliases, "different", "difference", "differs")
	}
	return uniqueStrings(aliases)
}

func isGenericRequirementDimension(dimension ComparisonDimension) bool {
	return dimension.Name == "要求" || dimension.Name == "约束"
}

func isGenericRequirementAlias(alias string) bool {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "要求", "约束", "required", "requirement", "requires", "must", "shall", "should", "constraint":
		return true
	default:
		return false
	}
}

func comparisonQuestionSubtopicTerms(question string, dimension ComparisonDimension, subjects []ComparisonSubject) []string {
	question = strings.TrimSpace(question)
	if question == "" || !isGenericRequirementDimension(dimension) {
		return nil
	}
	excluded := map[string]struct{}{
		"比较": {}, "对比": {}, "compare": {}, "comparison": {}, "这些": {}, "文档": {}, "的": {},
	}
	for _, subject := range subjects {
		addComparisonExclusion(excluded, subject.Label)
		addComparisonExclusion(excluded, subject.DocumentID)
		for _, alias := range subject.Aliases {
			addComparisonExclusion(excluded, alias)
		}
	}
	for _, alias := range comparisonDimensionEvidenceAliases(dimension) {
		addComparisonExclusion(excluded, alias)
	}

	var terms []string
	addTerm := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if len([]rune(term)) < 2 {
			return
		}
		if len([]rune(term)) > 20 || hasAny(term, "比较", "对比", "这些", "文档", "compare", "comparison") {
			return
		}
		if _, ok := excluded[term]; ok {
			return
		}
		if isGenericRelevanceTerm(term) {
			return
		}
		terms = append(terms, term)
	}

	lowerQuestion := strings.ToLower(question)
	for _, term := range []string{
		"审计日志", "审计", "日志", "audit log", "audit logging", "audit", "logging", "log",
		"密码轮换", "密码", "password rotation", "password", "rotation",
		"保留期限", "保留", "retention period", "retention",
	} {
		if strings.Contains(lowerQuestion, term) {
			addTerm(term)
		}
	}
	for _, term := range relevantQuestionTerms(question) {
		addTerm(term)
	}
	return uniqueStrings(terms)
}

func addComparisonExclusion(excluded map[string]struct{}, value string) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return
	}
	excluded[value] = struct{}{}
	for _, field := range strings.Fields(value) {
		excluded[field] = struct{}{}
	}
}

func comparisonTermMatchesText(term, lowerText string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" || lowerText == "" {
		return false
	}
	if containsHan(term) {
		return strings.Contains(lowerText, term)
	}
	switch term {
	case "审计日志", "审计", "日志", "audit log", "audit logging", "audit", "logging", "log":
		return hasAny(lowerText, "audit log", "audit logging", "audit", "logging", "log", "logs", "日志", "审计")
	case "密码轮换", "密码", "password rotation", "password", "rotation":
		return hasAny(lowerText, "password rotation", "password", "rotation", "密码", "轮换")
	case "保留期限", "保留", "retention period", "retention":
		return hasAny(lowerText, "retention period", "retention", "保留", "期限")
	default:
		return comparisonPhraseMatchesText(term, lowerText)
	}
}

func comparisonEvidenceScore(source NotebookChatSource, subject ComparisonSubject, dimension ComparisonDimension) int {
	score := int(source.Score * 100)
	text := strings.ToLower(strings.Join([]string{
		source.DocumentName,
		strings.Join(source.SectionPath, " "),
		source.Content,
	}, " "))
	if comparisonPhraseMatchesText(subject.Label, text) {
		score += 120
	}
	for _, alias := range subject.Aliases {
		if comparisonPhraseMatchesText(alias, text) {
			score += 80
			break
		}
	}
	for _, alias := range comparisonDimensionEvidenceAliases(dimension) {
		if comparisonPhraseMatchesText(alias, text) {
			score += 20
		}
	}
	return score
}

func containsHan(value string) bool {
	return regexp.MustCompile(`\p{Han}`).MatchString(value)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isMultiDocumentOverviewQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"分别", "各自", "两个文档", "多份文档", "这些文档", "讲了什么", "总结", "概括", "overview", "summarize"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func isComparisonQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"比较", "对比", "区别", "共同点", "相同点", "不同点", "差异", "compare", "comparison", "difference", "similarity", "different"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func isCoverageListingQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"哪些", "哪几", "列出", "所有", "涉及", "包含", "使用了", "提到", "where are", "which sections", "which items", "list all", "all sections", "all items", "coverage"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func isConstraintCheckQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"是否明确", "有没有明确", "是否指定", "有没有指定", "是否只需要", "是否必须", "说法对吗", "这个说法", "对吗", "未明确", "如果没有", "does not explicitly", "explicitly specify", "specified", "if not", "is it true", "is this correct"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func isConstrainedSynthesisQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	if hasAny(question, "只使用", "仅使用", "using only", "use only", "only use") &&
		(hasAny(question, "设计", "提出", "规划", "构思") || hasEnglishWord(question, "design") || hasEnglishWord(question, "propose")) {
		return true
	}
	if hasAny(question, "设计一个", "设计一种", "设计一套", "设计出", "请设计", "方案设计", "提出一个", "提出一种", "提出一套", "提出方案", "规划一个", "规划一种", "规划一套", "构思一个", "构思一种", "构思一套") {
		return true
	}
	if hasEnglishWord(question, "design") && hasAny(question, "workflow", "system", "solution", "architecture", "plan", "module", "component") {
		return true
	}
	if (hasEnglishWord(question, "propose") || hasEnglishWord(question, "proposal")) && hasAny(question, "workflow", "system", "solution", "architecture", "plan") {
		return true
	}
	return false
}

func hasEnglishWord(text, word string) bool {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(strings.ToLower(word)) + `\b`)
	return pattern.MatchString(strings.ToLower(text))
}

func isOpenSynthesisQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"根据", "综合", "推断", "共同", "作用", "关系", "适合", "意义", "启示", "评价", "判断", "建议", "why", "reason", "infer", "synthesize", "overall"}
	for _, keyword := range keywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}
	return false
}

func isPrecisionQuestion(question string) bool {
	question = strings.ToLower(strings.TrimSpace(question))
	keywords := []string{"第几页", "页码", "多少", "几个", "数值", "参数", "公式", "代码", "表格", "哪一页", "where exactly", "which page", "exact value", "number", "formula"}
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

func (s *notebookChatService) loadBroadNotebookSources(ctx context.Context, userID, notebookID string, docIDs []string, cfg configs.StructureEvidenceConfig) []NotebookChatSource {
	cfg = effectiveStructureEvidenceConfig(cfg)
	if !cfg.Enabled || s.vectorStore == nil || strings.TrimSpace(notebookID) == "" {
		return nil
	}
	chunks, err := s.vectorStore.GetAllChunks(ctx)
	if err != nil {
		zap.L().Warn("structure evidence broad chunk scan failed", zap.Error(err))
		return nil
	}
	docFilter := map[string]struct{}{}
	for _, docID := range docIDs {
		docID = strings.TrimSpace(docID)
		if docID != "" {
			docFilter[docID] = struct{}{}
		}
	}
	filtered := make([]repository.NotebookChunk, 0, minInt(len(chunks), cfg.MaxChunksPerDocument*maxPositiveInt(1, len(docIDs))))
	counts := map[string]int{}
	for _, chunk := range chunks {
		if chunk.NotebookID != notebookID {
			continue
		}
		if len(docFilter) > 0 {
			if _, ok := docFilter[chunk.DocumentID]; !ok {
				continue
			}
		}
		if cfg.MaxChunksPerDocument > 0 && counts[chunk.DocumentID] >= cfg.MaxChunksPerDocument {
			continue
		}
		counts[chunk.DocumentID]++
		filtered = append(filtered, chunk)
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].DocumentID != filtered[j].DocumentID {
			return filtered[i].DocumentID < filtered[j].DocumentID
		}
		return filtered[i].ChunkIndex < filtered[j].ChunkIndex
	})
	scores := make([]float32, len(filtered))
	for i := range scores {
		scores[i] = 0.01
	}
	sources := buildSourcesFromChunks(s, ctx, filtered, scores, userID)
	zap.L().Info("structure evidence broad sources loaded",
		zap.Int("count", len(sources)),
		zap.Strings("doc_ids", getNotebookSourceDocumentIDs(sources)),
		zap.String("source_kind", "repository_chunks"),
	)
	return sources
}

func (s *notebookChatService) logStructureEvidenceSources(stage string, sources []NotebookChatSource) {
	for _, source := range sources {
		zap.L().Debug("structure evidence source",
			zap.String("stage", stage),
			zap.String("doc", source.DocumentName),
			zap.String("doc_id", source.DocumentID),
			zap.Int64("page", source.PageNumber),
			zap.Int64("chunk", source.ChunkIndex),
			zap.String("route", source.RetrievalRoute),
			zap.String("bucket", source.EvidenceBucket),
			zap.String("status", source.EvidenceStatus),
			zap.String("preview", trimForPrompt(source.Content, 160)),
		)
	}
}

func maxPositiveInt(left, right int) int {
	if left > right {
		return left
	}
	return right
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
	docNameMap := map[string]string{}
	if svc != nil && svc.docRepo != nil {
		docNameMap = svc.docRepo.GetNamesByIDs(ctx, userID, uniqueDocIDs)
	}

	for i, chunk := range chunks {
		docName := docNameMap[chunk.DocumentID]
		if docName == "" {
			docName = "Unknown Document"
		}
		score := float32(0)
		if i < len(scores) {
			score = scores[i]
		}
		sources = append(sources, NotebookChatSource{
			NotebookID:   chunk.NotebookID,
			DocumentID:   chunk.DocumentID,
			DocumentName: docName,
			PageNumber:   chunk.PageNumber,
			ChunkIndex:   chunk.ChunkIndex,
			Content:      chunk.Content,
			Score:        score,
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
	builder.WriteString(fmt.Sprintf("Document: %s, %s, Type: %s\n", visualSource.DocumentName, formatEvidencePage(visualSource.PageNumber), firstNonEmpty(visualSource.VisualType, visualSource.ChunkType)))
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
