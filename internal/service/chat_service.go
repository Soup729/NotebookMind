package service

import (
	"context"
	"fmt"
	"strings"

	"enterprise-pdf-ai/internal/repository"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

type ChatService interface {
	Ask(ctx context.Context, userID, sessionID, question string) (string, []schema.Document, error)
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

func (s *chatService) Ask(ctx context.Context, userID, sessionID, question string) (string, []schema.Document, error) {
	history, err := s.chatRepo.ListSessionMessages(ctx, userID, sessionID, s.historyLimit)
	if err != nil {
		return "", nil, fmt.Errorf("load chat history: %w", err)
	}

	sources, err := s.llmService.RetrieveContext(ctx, question, s.retrievalTopK)
	if err != nil {
		return "", nil, fmt.Errorf("retrieve source documents: %w", err)
	}

	prompt := buildPrompt(history, sources, question)
	answer, err := s.llmService.GenerateAnswer(ctx, prompt)
	if err != nil {
		return "", nil, fmt.Errorf("generate answer: %w", err)
	}

	userMessage := &repository.ChatMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   question,
	}
	if err := s.chatRepo.SaveMessage(ctx, userMessage); err != nil {
		return "", nil, fmt.Errorf("save user message: %w", err)
	}

	assistantMessage := &repository.ChatMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "assistant",
		Content:   answer,
	}
	if err := s.chatRepo.SaveMessage(ctx, assistantMessage); err != nil {
		return "", nil, fmt.Errorf("save assistant message: %w", err)
	}

	zap.L().Info("chat answered",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID),
		zap.Int("history_count", len(history)),
		zap.Int("source_count", len(sources)),
	)

	return answer, sources, nil
}

func buildPrompt(history []repository.ChatMessage, sources []schema.Document, question string) string {
	var builder strings.Builder

	builder.WriteString("你是企业知识库问答助手。你必须严格依据给定上下文回答。如果上下文不足，请明确说明无法从文档中确认答案。")
	builder.WriteString("\n\n对话历史:\n")
	for _, message := range history {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}

	builder.WriteString("\n检索到的文档片段:\n")
	for i, doc := range sources {
		builder.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.PageContent))
	}

	builder.WriteString("\n用户问题:\n")
	builder.WriteString(question)
	builder.WriteString("\n\n请给出最终回答，并确保结论可在文档片段中找到依据。")

	return builder.String()
}
