package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"NotebookAI/internal/api/router"
	"NotebookAI/internal/app"
	"NotebookAI/internal/configs"
	"NotebookAI/internal/platform/cache"
	"NotebookAI/internal/platform/database"
	"NotebookAI/internal/platform/logger"

	"go.uber.org/zap"
)

func main() {
	if wantsHelp(os.Args[1:]) {
		fmt.Println("Usage: go run ./cmd/api")
		fmt.Println("Starts the NotebookMind API server using configs/config.yaml.")
		return
	}

	cfg, err := configs.LoadConfig("configs/config.yaml")
	if err != nil {
		panic(fmt.Sprintf("Fatal error loading config: %v", err))
	}

	if err := logger.InitLogger(&cfg.Log); err != nil {
		panic(fmt.Sprintf("Fatal error initializing logger: %v", err))
	}
	defer logger.Sync()

	zap.L().Info("Starting application", zap.String("name", cfg.App.Name), zap.String("env", cfg.App.Env))

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

	engine := router.New(
		cfg,
		container.AuthHandler,
		container.DocumentHandler,
		container.ChatHandler,
		container.DashboardHandler,
		container.SearchHandler,
		container.UsageHandler,
		container.NotebookHandler,
		container.NoteHandler,
		container.VQAHandler,
	)
	engine.MaxMultipartMemory = cfg.Upload.MaxFileSizeMB << 20
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.App.Port),
		Handler: engine,
	}

	go func() {
		zap.L().Info("Server is running", zap.Int("port", cfg.App.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("Server failed to listen", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("Server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Server exiting")
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}
