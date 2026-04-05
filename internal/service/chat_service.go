package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"NotebookAI/internal/models"
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
	Session                *models.ChatSession
	Message                *models.ChatMessage
	Sources                []ChatSource
	RecommendedQuestions   []string        `json:"recommended_questions,omitempty"`
	Reflection             *ReflectionResult `json:"reflection,omitempty"`
}

type ChatService interface {
	CreateSession(ctx context.Context, userID, title string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error)
	ListMessages(ctx context.Context, userID, sessionID string) ([]models.ChatMessage, error)
	SendMessage(ctx context.Context, userID, sessionID, question string, documentIDs []string) (*ChatReply, error)
	Search(ctx context.Context, userID, query string, topK int, documentIDs []string) ([]ChatSource, error)
	StreamSendMessage(ctx context.Context, opts StreamChatOptions) error
	GenerateRecommendedQuestions(ctx context.Context, userID, sessionID string) ([]string, error)
	GenerateReflection(ctx context.Context, userID, sessionID, messageID string) (*ReflectionResult, error)
}

type chatService struct {
	llmService    LLMService
	chatRepo      repository.ChatRepository
	historyLimit  int
	retrievalTopK int
}

func NewChatService(llmService LLMService, chatRepo repository.ChatRepository, historyLimit int, retrievalTopK int) ChatService {
	return &chatService{
		llmService:    llmService,
		chatRepo:      chatRepo,
		historyLimit:  historyLimit,
		retrievalTopK: retrievalTopK,
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
	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, s.historyLimit)
	if err != nil {
		return nil, fmt.Errorf("load chat history: %w", err)
	}

	retrievedDocs, err := s.llmService.RetrieveContext(ctx, question, s.retrievalTopK, RetrievalOptions{
		UserID:      userID,
		DocumentIDs: documentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("retrieve source documents: %w", err)
	}

	sources := documentsToSources(retrievedDocs)
	prompt := buildPrompt(history, sources, question)
	answer, err := s.llmService.GenerateAnswer(ctx, prompt)
	if err != nil {
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

	return &ChatReply{
		Session: session,
		Message: assistantMessage,
		Sources: sources,
	}, nil
}

func (s *chatService) Search(ctx context.Context, userID, query string, topK int, documentIDs []string) ([]ChatSource, error) {
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
	if len([]rune(title)) > 48 {
		title = string([]rune(title)[:48]) + "..."
	}
	if title == "" {
		return "New conversation"
	}
	return title
}

// StreamSendMessage performs RAG-based chat with SSE streaming response
func (s *chatService) StreamSendMessage(ctx context.Context, opts StreamChatOptions) error {
	// Step 1: Load session
	session, err := s.chatRepo.GetSession(ctx, opts.UserID, opts.SessionID)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("load session: %w", err))
		}
		return fmt.Errorf("load session: %w", err)
	}

	// Step 2: Get conversation history
	history, err := s.chatRepo.ListSessionMessages(ctx, opts.UserID, opts.SessionID, s.historyLimit)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("load history: %w", err))
		}
		return fmt.Errorf("load history: %w", err)
	}

	// Step 3: Retrieve relevant chunks
	retrievedDocs, err := s.llmService.RetrieveContext(ctx, opts.Question, s.retrievalTopK, RetrievalOptions{
		UserID:      opts.UserID,
		DocumentIDs: opts.DocumentIDs,
	})
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("retrieve source documents: %w", err))
		}
		return fmt.Errorf("retrieve source documents: %w", err)
	}

	sources := documentsToSources(retrievedDocs)
	prompt := buildPrompt(history, sources, opts.Question)
	promptTokens := estimateTokens(prompt)

	// Step 4: Send sources event first
	if opts.OnSource != nil {
		if !opts.OnSource(sources, promptTokens) {
			return fmt.Errorf("client disconnected")
		}
	}

	// Step 5: Generate response
	answer, err := s.llmService.GenerateAnswer(ctx, prompt)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(fmt.Errorf("generate answer: %w", err))
		}
		return fmt.Errorf("generate answer: %w", err)
	}

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

	return nil
}

// GenerateRecommendedQuestions generates suggested follow-up questions based on the conversation context
func (s *chatService) GenerateRecommendedQuestions(ctx context.Context, userID, sessionID string) ([]string, error) {
	// Load session and messages
	_, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	messages, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, 10)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	// Build context from recent messages
	var contextBuilder strings.Builder
	for _, msg := range messages {
		if msg.Role == "user" {
			contextBuilder.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		} else if msg.Role == "assistant" {
			contextBuilder.WriteString(fmt.Sprintf("Assistant: %s\n", msg.Content))
		}
	}

	// Retrieve relevant context
	latestQuestion := ""
	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				latestQuestion = messages[i].Content
				break
			}
		}
	}

	docs, err := s.llmService.RetrieveContext(ctx, latestQuestion, s.retrievalTopK, RetrievalOptions{
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("retrieve context: %w", err)
	}

	sources := documentsToSources(docs)

	// Build context string for question generation
	contextBuilder.WriteString("\nRetrieved context:\n")
	for _, src := range sources {
		contextBuilder.WriteString(fmt.Sprintf("- [%s] %s\n", src.FileName, src.Content))
	}

	// Generate recommended questions
	questions, err := s.llmService.GenerateRecommendedQuestions(ctx, contextBuilder.String(), 3)
	if err != nil {
		zap.L().Warn("failed to generate recommended questions", zap.Error(err))
		// Return default questions as fallback
		return []string{
			"Can you summarize the key points?",
			"What are the main conclusions?",
			"Can you explain this in more detail?",
		}, nil
	}

	return questions, nil
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
