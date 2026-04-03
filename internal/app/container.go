package app

import (
	"context"
	"fmt"

	"enterprise-pdf-ai/internal/api/handlers"
	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/platform/database"
	"enterprise-pdf-ai/internal/repository"
	"enterprise-pdf-ai/internal/service"
	"enterprise-pdf-ai/internal/worker"
)

type Container struct {
	LLMService      service.LLMService
	ChatService     service.ChatService
	TaskProducer    worker.TaskProducer
	DocumentHandler *handlers.DocumentHandler
	ChatHandler     *handlers.ChatHandler
}

func NewContainer(ctx context.Context, cfg *configs.Config) (*Container, error) {
	llmService, err := service.NewLLMService(ctx, &cfg.LLM, &cfg.Milvus)
	if err != nil {
		return nil, fmt.Errorf("initialize llm service: %w", err)
	}

	chatRepo, err := repository.NewChatRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize chat repository: %w", err)
	}

	chatService := service.NewChatService(
		llmService,
		chatRepo,
		cfg.Chat.HistoryLimit,
		cfg.Chat.RetrievalTopK,
	)

	producer := worker.NewTaskProducer(&cfg.Cache.Redis)
	documentHandler := handlers.NewDocumentHandler(producer, cfg.Upload)
	chatHandler := handlers.NewChatHandler(chatService)

	return &Container{
		LLMService:      llmService,
		ChatService:     chatService,
		TaskProducer:    producer,
		DocumentHandler: documentHandler,
		ChatHandler:     chatHandler,
	}, nil
}
