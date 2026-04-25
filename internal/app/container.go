package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"NotebookAI/internal/api/handlers"
	"NotebookAI/internal/configs"
	"NotebookAI/internal/observability"
	"NotebookAI/internal/parser"
	"NotebookAI/internal/platform/database"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/service"
	"NotebookAI/internal/worker"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"go.uber.org/zap"
)

type Container struct {
	LLMService       service.LLMService
	AuthService      service.AuthService
	DocumentService  service.DocumentService
	ChatService      service.ChatService
	DashboardService service.DashboardService
	TaskProducer     worker.TaskProducer

	// NotebookLM
	NotebookService         service.NotebookService
	NotebookChatService     service.NotebookChatService
	NotebookArtifactService service.NotebookArtifactService
	NotebookExportService   service.NotebookExportService
	NotebookRepository      repository.NotebookRepository

	// Notes
	NoteService    service.NoteService
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
	NoteHandler      *handlers.NoteHandler

	DocumentRepository repository.DocumentRepository

	// Phase 1: 结构化文档解析
	ParserService parser.ParserService

	// Phase 2: Hybrid RAG
	Tokenizer       *service.Tokenizer
	BM25Index       *service.BM25Index
	HybridSearch    service.HybridSearchService
	IntentRewrite   service.IntentRewriteService
	RerankerService service.RerankerService
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

	// ========== Phase 2: 初始化 Hybrid RAG 服务 ==========
	tokenizer, bm25Index, rerankerSvc, hybridSearchSvc, intentRewriteSvc := initPhase2Services(cfg, notebookVectorStore, embedder, chatLLM, chatRepo)

	// ========== Phase 2: BM25 索引预热（从Milvus加载已有chunks） ==========
	if bm25Index != nil && notebookVectorStore != nil && cfg.HybridSearch.Enabled {
		warmupTimer := observability.NewStopwatch()
		chunks, err := notebookVectorStore.GetAllChunks(ctx)
		if err != nil {
			zap.L().Warn("failed to warmup BM25 index from Milvus", zap.Error(err))
		} else {
			for _, chunk := range chunks {
				chunkID := fmt.Sprintf("nb_%s_%d", chunk.DocumentID, chunk.ChunkIndex)
				bm25Index.IndexDocumentWithMetadata(chunkID, chunk.DocumentID, chunk.Content, service.NotebookChunkMetadata(chunk))
			}
			zap.L().Info("BM25 index warmed up",
				zap.Int("chunk_count", bm25Index.GetDocCount()),
				zap.Int64("warmup_latency_ms", warmupTimer.ElapsedMs()),
			)
		}

		// 启动后台定时刷新（解决 API 进程和 Worker 进程独立、BM25 实例不共享的问题）
		// Worker 进程处理文档时更新自己的 BM25Index，API 进程需要定期从 Milvus 同步
		bm25Index.StartRefreshLoop(ctx, notebookVectorStore, 30*time.Second)
	}

	notebookService := service.NewNotebookService(notebookRepo, notebookVectorStore, embedder, chatLLM, &cfg.LLM, bm25Index)
	notebookArtifactService := service.NewNotebookArtifactService(notebookRepo, chatLLM)
	trustWorkflow := service.NewTrustWorkflow(hybridSearchSvc, chatLLM, cfg.TrustWorkflow.MaxRepairAttempts)
	sessionMemoryService := service.NewSessionMemoryService(chatRepo, chatLLM)
	notebookChatService := service.NewNotebookChatService(notebookRepo, documentRepo, notebookVectorStore, chatRepo, chatLLM, embedder, cfg.Chat.RetrievalTopK, hybridSearchSvc, intentRewriteSvc, bm25Index, trustWorkflow, &cfg.TrustWorkflow, &cfg.CitationGuard, llmService, &cfg.Multimodal, sessionMemoryService)

	// Initialize Note service
	noteRepo, err := repository.NewNoteRepository(database.DB)
	if err != nil {
		return nil, fmt.Errorf("initialize note repository: %w", err)
	}
	noteService := service.NewNoteService(noteRepo)

	authService := service.NewAuthService(userRepo, cfg.Auth.JWTSecret)
	documentService := service.NewDocumentService(documentRepo, llmService)
	chatService := service.NewChatService(llmService, chatRepo, cfg.Chat.HistoryLimit, cfg.Chat.RetrievalTopK, hybridSearchSvc, intentRewriteSvc, bm25Index)
	dashboardService := service.NewDashboardService(documentRepo, chatRepo)

	producer := worker.NewTaskProducer(&cfg.Cache.Redis)
	notebookExportService := service.NewNotebookExportService(notebookRepo, chatLLM, producer)

	authHandler := handlers.NewAuthHandler(authService)
	documentHandler := handlers.NewDocumentHandler(producer, documentService, notebookService, cfg.Upload)
	chatHandler := handlers.NewChatHandler(chatService, &cfg.LLM)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)
	searchHandler := handlers.NewSearchHandler(chatService)
	usageHandler := handlers.NewUsageHandler(dashboardService)
	notebookHandler := handlers.NewNotebookHandler(notebookService, notebookChatService, notebookArtifactService, notebookExportService, embedder)
	noteHandler := handlers.NewNoteHandler(noteService)
	vqaHandler := handlers.NewVQAHandler(llmService)

	// ========== Phase 1: 初始化结构化文档解析服务 ==========
	parserSvc := initParserService(cfg)

	return &Container{
		LLMService:              llmService,
		AuthService:             authService,
		DocumentService:         documentService,
		ChatService:             chatService,
		DashboardService:        dashboardService,
		TaskProducer:            producer,
		NotebookService:         notebookService,
		NotebookChatService:     notebookChatService,
		NotebookArtifactService: notebookArtifactService,
		NotebookExportService:   notebookExportService,
		NotebookRepository:      notebookRepo,
		NoteService:             noteService,
		NoteRepository:          noteRepo,
		VQAHandler:              vqaHandler,
		AuthHandler:             authHandler,
		DocumentHandler:         documentHandler,
		ChatHandler:             chatHandler,
		DashboardHandler:        dashboardHandler,
		SearchHandler:           searchHandler,
		UsageHandler:            usageHandler,
		NotebookHandler:         notebookHandler,
		NoteHandler:             noteHandler,
		DocumentRepository:      documentRepo,
		ParserService:           parserSvc,
		// Phase 2
		Tokenizer:       tokenizer,
		BM25Index:       bm25Index,
		HybridSearch:    hybridSearchSvc,
		IntentRewrite:   intentRewriteSvc,
		RerankerService: rerankerSvc,
	}, nil
}

// initPhase2Services 初始化 Phase 2: Hybrid RAG 相关服务
func initPhase2Services(
	cfg *configs.Config,
	notebookVectorStore repository.NotebookVectorStore,
	embedder embeddings.Embedder,
	chatLLM llms.Model,
	chatRepo repository.ChatRepository,
) (*service.Tokenizer, *service.BM25Index, service.RerankerService, service.HybridSearchService, service.IntentRewriteService) {

	// 1. 创建统一分词器
	tokenizer := service.NewTokenizer()

	// 2. 创建 BM25 索引
	bm25Index := service.NewBM25Index(tokenizer)

	// 3. 创建 Reranker（根据 COHERE_API_KEY 决定是否启用）
	var rerankerSvc service.RerankerService
	cohereKey := os.Getenv("COHERE_API_KEY")
	if cohereKey != "" {
		zap.L().Info("Cohere reranker enabled", zap.String("model", cfg.Reranker.Model))
		rerankerSvc = service.NewCohereReranker(cohereKey, &cfg.Reranker)
	} else {
		zap.L().Info("COHERE_API_KEY not set, reranker disabled (using fallback)")
		rerankerSvc = service.NewFallbackReranker()
	}

	// 4. 创建 Failover 策略
	failover := service.NewFailoverStrategy(&cfg.HybridSearch, tokenizer)

	// 5. 创建 IntentRewriteService（根据 .env 控制）
	var intentRewriteSvc service.IntentRewriteService
	intentCfg := cfg.IntentRewrite
	if intentCfg.Enabled {
		zap.L().Info("Intent routing enabled",
			zap.Bool("llm_rewrite", intentCfg.LLMRewriteEnabled),
			zap.Int("max_context_terms", intentCfg.MaxContextTerms),
		)
		intentRewriteSvc = service.NewIntentRewriteService(chatRepo, chatLLM, tokenizer, &intentCfg)
	} else {
		zap.L().Info("Intent routing disabled (ENABLE_INTENT_ROUTING not set)")
		// 创建一个空实现，直接返回原始查询
		intentRewriteSvc = service.NewIntentRewriteService(nil, nil, tokenizer, &configs.IntentRewriteConfig{
			Enabled:           false,
			LLMRewriteEnabled: false,
			MaxContextTerms:   3,
		})
	}

	// 6. 创建 HybridSearchService
	var hybridSearchSvc service.HybridSearchService
	if cfg.HybridSearch.Enabled {
		zap.L().Info("Hybrid search enabled",
			zap.Int("rrf_k", cfg.HybridSearch.RRFK),
			zap.Int("top_k", cfg.HybridSearch.TopK),
			zap.Int("rerank_top_k", cfg.HybridSearch.RerankTopK),
		)
		hybridSearchSvc = service.NewHybridSearchService(
			notebookVectorStore,
			bm25Index,
			rerankerSvc,
			failover,
			embedder,
			intentRewriteSvc,
			&cfg.HybridSearch,
		)
	} else {
		zap.L().Info("Hybrid search disabled, using pure dense retrieval")
		hybridSearchSvc = nil
	}

	return tokenizer, bm25Index, rerankerSvc, hybridSearchSvc, intentRewriteSvc
}

// initParserService 初始化结构化文档解析服务（含 OCR + VLM）
func initParserService(cfg *configs.Config) parser.ParserService {
	// 构建 ParserConfig
	parserCfg := &parser.ParserConfig{
		ChunkSize:              cfg.Parser.ChunkSize,
		ChunkOverlap:           cfg.Parser.ChunkOverlap,
		ChildChunkSize:         cfg.Parser.ChildChunkSize,
		ExtractTables:          cfg.Parser.ExtractTables,
		TableMaxRows:           cfg.Parser.TableMaxRows,
		TableMaxCols:           cfg.Parser.TableMaxCols,
		ExtractImages:          cfg.Parser.ExtractImages,
		ImageMinSize:           cfg.Parser.ImageMinSize,
		VLMEnabled:             cfg.VLM.Enabled || cfg.Parser.VLMEnabled,
		VLMBatchSize:           cfg.Parser.VLMBatchSize,
		SaveVisualRegions:      cfg.Multimodal.Enabled && cfg.Multimodal.SaveVisualRegions,
		VisualStorageRoot:      cfg.Multimodal.VisualStorageRoot,
		ChartExtractionEnabled: cfg.Multimodal.Enabled && cfg.Multimodal.ChartExtractionEnabled,
		OCRThreshold:           cfg.Parser.OCRThreshold,
		OCREnabled:             cfg.Parser.OCREnabled,
		DetectHeadings:         cfg.Parser.DetectHeadings,
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
		if vlmModel == "" {
			vlmModel = "gpt-4o-mini"
		}
		// 默认复用 OpenAI 配置
		if vlmAPIKey == "" {
			vlmAPIKey = cfg.LLM.Providers.OpenAI.APIKey
		}
		if vlmBaseURL == "" {
			vlmBaseURL = cfg.LLM.Providers.OpenAI.BaseURL
		}

		vlmProvider = parser.NewOpenAIVLMProvider(vlmAPIKey, vlmBaseURL, vlmModel)
	} else {
		vlmProvider = parser.NewFallbackVLMProvider()
	}

	return parser.NewDocumentParser(parserCfg, ocrProvider, vlmProvider)
}
