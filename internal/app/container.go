package app

import (
	"context"
	"fmt"

	"NotebookAI/internal/api/handlers"
	"NotebookAI/internal/configs"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/platform/database"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/service"
	"NotebookAI/internal/worker"
)

type Container struct {
	LLMService       service.LLMService
	AuthService      service.AuthService
	DocumentService  service.DocumentService
	ChatService      service.ChatService
	DashboardService service.DashboardService
	TaskProducer     worker.TaskProducer

	// NotebookLM
	NotebookService   service.NotebookService
	NotebookChatService service.NotebookChatService
	NotebookRepository repository.NotebookRepository

	// Notes
	NoteService   service.NoteService
	NoteRepository repository.NoteRepository

	// VQA
	VQAHandler *handlers.VQAHandler

	AuthHandler      *handlers.AuthHandler
	DocumentHandler  *handlers.DocumentHandler
	ChatHandler      *handlers.ChatHandler
	DashboardHandler *handlers.DashboardHandler
	SearchHandler    *handlers.SearchHandler
	UsageHandler     *handlers.UsageHandler
	NotebookHandler  *handlers.NotebookHandler
	NoteHandler     *handlers.NoteHandler

	DocumentRepository repository.DocumentRepository

	// Phase 1: 结构化文档解析
	ParserService parser.ParserService
}

func NewContainer(ctx context.Context, cfg *configs.Config) (*Container, error) {
	userRepo, err := repository.NewUserRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize user repository: %w", err)
	}
	documentRepo, err := repository.NewDocumentRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize document repository: %w", err)
	}
	chatRepo, err := repository.NewChatRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize chat repository: %w", err)
	}
	notebookRepo, err := repository.NewNotebookRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize notebook repository: %w", err)
	}

	llmService, err := service.NewLLMService(ctx, database.DB, &cfg.LLM, &cfg.Milvus)
	if err != nil {
		return nil, fmt.Errorf("initialize llm service: %w", err)
	}

	// Initialize NotebookLM services
	notebookVectorStore, err := repository.NewNotebookMilvusStore(ctx, &cfg.Milvus)
	if err != nil {
		// Notebook vector store is optional, log warning but continue
		fmt.Printf("Warning: Notebook vector store unavailable: %v\n", err)
	}

	embedder, chatLLM, err := service.NewNotebookLLMService(ctx, &cfg.LLM)
	if err != nil {
		return nil, fmt.Errorf("initialize notebook LLM service: %w", err)
	}

	notebookService := service.NewNotebookService(notebookRepo, notebookVectorStore, embedder, chatLLM, &cfg.LLM)
	notebookChatService := service.NewNotebookChatService(notebookRepo, documentRepo, notebookVectorStore, chatRepo, chatLLM, embedder, cfg.Chat.RetrievalTopK)

	// Initialize Note service
	noteRepo, err := repository.NewNoteRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize note repository: %w", err)
	}
	noteService := service.NewNoteService(noteRepo)

	authService := service.NewAuthService(userRepo, cfg.Auth.JWTSecret)
	documentService := service.NewDocumentService(documentRepo, llmService)
	chatService := service.NewChatService(llmService, chatRepo, cfg.Chat.HistoryLimit, cfg.Chat.RetrievalTopK)
	dashboardService := service.NewDashboardService(documentRepo, chatRepo)

	producer := worker.NewTaskProducer(&cfg.Cache.Redis)

	authHandler := handlers.NewAuthHandler(authService)
	documentHandler := handlers.NewDocumentHandler(producer, documentService, notebookService, cfg.Upload)
	chatHandler := handlers.NewChatHandler(chatService, &cfg.LLM)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)
	searchHandler := handlers.NewSearchHandler(chatService)
	usageHandler := handlers.NewUsageHandler(dashboardService)
	notebookHandler := handlers.NewNotebookHandler(notebookService, notebookChatService, embedder)
	noteHandler := handlers.NewNoteHandler(noteService)
	vqaHandler := handlers.NewVQAHandler(llmService)

	// ========== Phase 1: 初始化结构化文档解析服务 ==========
	parserSvc := initParserService(cfg)

	return &Container{
		LLMService:           llmService,
		AuthService:          authService,
		DocumentService:      documentService,
		ChatService:          chatService,
		DashboardService:    dashboardService,
		TaskProducer:         producer,
		NotebookService:     notebookService,
		NotebookChatService: notebookChatService,
		NotebookRepository:  notebookRepo,
		NoteService:         noteService,
		NoteRepository:      noteRepo,
		VQAHandler:          vqaHandler,
		AuthHandler:         authHandler,
		DocumentHandler:     documentHandler,
		ChatHandler:         chatHandler,
		DashboardHandler:    dashboardHandler,
		SearchHandler:       searchHandler,
		UsageHandler:        usageHandler,
		NotebookHandler:     notebookHandler,
		NoteHandler:         noteHandler,
		DocumentRepository:   documentRepo,
		ParserService:        parserSvc,
	}, nil
}

// initParserService 初始化结构化文档解析服务（含 OCR + VLM）
func initParserService(cfg *configs.Config) parser.ParserService {
	// 构建 ParserConfig
	parserCfg := &parser.ParserConfig{
		ChunkSize:       cfg.Parser.ChunkSize,
		ChunkOverlap:    cfg.Parser.ChunkOverlap,
		ChildChunkSize:  cfg.Parser.ChildChunkSize,
		ExtractTables:   cfg.Parser.ExtractTables,
		TableMaxRows:    cfg.Parser.TableMaxRows,
		TableMaxCols:    cfg.Parser.TableMaxCols,
		ExtractImages:   cfg.Parser.ExtractImages,
		ImageMinSize:    cfg.Parser.ImageMinSize,
		VLMEnabled:      cfg.VLM.Enabled || cfg.Parser.VLMEnabled,
		VLMBatchSize:    cfg.Parser.VLMBatchSize,
		OCRThreshold:    cfg.Parser.OCRThreshold,
		OCREnabled:      cfg.Parser.OCREnabled,
		DetectHeadings:  cfg.Parser.DetectHeadings,
	}

	// 初始化 OCR Provider
	var ocrProvider parser.OCRProvider
	switch cfg.OCR.Provider {
	case "rapidocr":
		ocrProvider = parser.NewRapidOCRProvider(cfg.OCR.BaseURL)
	default:
		ocrProvider = parser.NewFallbackOCRProvider()
	}

	// 初始化 VLM Provider
	var vlmProvider parser.VLMProvider
	if (cfg.VLM.Enabled || cfg.Parser.VLMEnabled) && cfg.VLM.APIKey != "" {
		vlmAPIKey := cfg.VLM.APIKey
		vlmBaseURL := cfg.VLM.BaseURL
		vlmModel := cfg.VLM.Model
		if vlmModel == "" { vlmModel = "gpt-4o-mini" }
		// 默认复用 OpenAI 配置
		if vlmAPIKey == "" { vlmAPIKey = cfg.LLM.Providers.OpenAI.APIKey }
		if vlmBaseURL == "" { vlmBaseURL = cfg.LLM.Providers.OpenAI.BaseURL }
		
		vlmProvider = parser.NewOpenAIVLMProvider(vlmAPIKey, vlmBaseURL, vlmModel)
	} else {
		vlmProvider = parser.NewFallbackVLMProvider()
	}

	return parser.NewDocumentParser(parserCfg, ocrProvider, vlmProvider)
}
