package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/schema"
)

type ChatSource struct {
	DocumentID string  `json:"document_id"`
	FileName   string  `json:"file_name"`
	Content    string  `json:"content"`
	Score      float32 `json:"score"`
	ChunkIndex int     `json:"chunk_index"`
}

type ChatReply struct {
	Session *models.ChatSession
	Message *models.ChatMessage
	Sources []ChatSource
}

type ChatService interface {
	CreateSession(ctx context.Context, userID, title string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error)
	ListMessages(ctx context.Context, userID, sessionID string) ([]models.ChatMessage, error)
	SendMessage(ctx context.Context, userID, sessionID, question string) (*ChatReply, error)
	Search(ctx context.Context, userID, query string, topK int) ([]ChatSource, error)
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

func (s *chatService) SendMessage(ctx context.Context, userID, sessionID, question string) (*ChatReply, error) {
	session, err := s.chatRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, s.historyLimit)
	if err != nil {
		return nil, fmt.Errorf("load chat history: %w", err)
	}

	retrievedDocs, err := s.llmService.RetrieveContext(ctx, question, s.retrievalTopK, RetrievalOptions{
		UserID: userID,
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

func (s *chatService) Search(ctx context.Context, userID, query string, topK int) ([]ChatSource, error) {
	docs, err := s.llmService.RetrieveContext(ctx, query, topK, RetrievalOptions{
		UserID: userID,
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
