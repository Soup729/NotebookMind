package main

import (
	"context"
	"fmt"

	"NotebookAI/internal/app"
	"NotebookAI/internal/configs"
	"NotebookAI/internal/platform/cache"
	"NotebookAI/internal/platform/database"
	"NotebookAI/internal/platform/logger"
	"NotebookAI/internal/worker"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func main() {
	cfg, err := configs.LoadConfig("configs/config.yaml")
	if err != nil {
		panic(fmt.Sprintf("Fatal error loading config: %v", err))
	}

	if err := logger.InitLogger(&cfg.Log); err != nil {
		panic(fmt.Sprintf("Fatal error initializing logger: %v", err))
	}
	defer logger.Sync()

	if _, err := database.InitPostgres(&cfg.Database.Postgres); err != nil {
		zap.L().Fatal("Failed to initialize database", zap.Error(err))
	}
	defer database.ClosePostgres()

	if _, err := cache.InitRedis(&cfg.Cache.Redis); err != nil {
		zap.L().Fatal("Failed to initialize redis", zap.Error(err))
	}
	defer cache.CloseRedis()

	container, err := app.NewContainer(context.Background(), cfg)
	if err != nil {
		zap.L().Fatal("Failed to initialize dependencies", zap.Error(err))
	}
	defer container.TaskProducer.Close()

	processor := worker.NewDocumentProcessor(container.LLMService, container.DocumentRepository, container.NotebookService, container.NotebookExportService, container.KnowledgeGraphService, container.ParserService, container.BM25Index)
	mux := asynq.NewServeMux()
	processor.RegisterHandlers(mux)

	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.Cache.Redis.Addr,
			Password: cfg.Cache.Redis.Password,
			DB:       cfg.Cache.Redis.DB,
		},
		asynq.Config{
			Concurrency: cfg.Asynq.Concurrency,
			Queues: map[string]int{
				"documents": 10,
				"exports":   3,
			},
		},
	)

	zap.L().Info("Asynq worker starting", zap.Int("concurrency", cfg.Asynq.Concurrency))
	if err := srv.Run(mux); err != nil {
		zap.L().Fatal("Asynq worker failed", zap.Error(err))
	}
}
