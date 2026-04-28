package configs

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// AppConfig defines the application configuration structure
type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
	Port int    `mapstructure:"port"`
}

// LogConfig defines logging configuration
type LogConfig struct {
	Level      string `mapstructure:"level"`
	FilePath   string `mapstructure:"file_path"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

// PostgresConfig defines PostgreSQL configuration
type PostgresConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime"` // in seconds
}

// RedisConfig defines Redis configuration
type RedisConfig struct {
	Addr         string `mapstructure:"addr"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

type AuthConfig struct {
	JWTSecret string `mapstructure:"jwt_secret"`
}

type UploadConfig struct {
	LocalDir       string `mapstructure:"local_dir"`
	MaxFileSizeMB  int64  `mapstructure:"max_file_size_mb"`
	AllowedExtName string `mapstructure:"allowed_ext_name"`
}

type AsynqConfig struct {
	Concurrency int `mapstructure:"concurrency"`
}

// DatabaseConfig wraps all DB configs
type DatabaseConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
}

// CacheConfig wraps all cache configs
type CacheConfig struct {
	Redis RedisConfig `mapstructure:"redis"`
}

// MilvusConfig defines Zilliz Cloud / Milvus configuration
type MilvusConfig struct {
	Address        string `mapstructure:"address"`
	Username       string `mapstructure:"username"`
	Password       string `mapstructure:"password"`
	CollectionName string `mapstructure:"collection_name"`
	Dimension      int    `mapstructure:"dimension"`
}

// OpenAIConfig defines OpenAI settings
type OpenAIConfig struct {
	APIKey         string `mapstructure:"api_key"`
	BaseURL        string `mapstructure:"base_url"`
	EmbeddingModel string `mapstructure:"embedding_model"`
	ChatModel      string `mapstructure:"chat_model"`
	VisionModel    string `mapstructure:"vision_model"` // For VQA
}

// AnthropicConfig defines Anthropic Claude settings
type AnthropicConfig struct {
	APIKey      string `mapstructure:"api_key"`
	BaseURL     string `mapstructure:"base_url"`
	ChatModel   string `mapstructure:"chat_model"`
	VisionModel string `mapstructure:"vision_model"`
}

// GeminiConfig defines Google Gemini settings
type GeminiConfig struct {
	APIKey      string `mapstructure:"api_key"`
	BaseURL     string `mapstructure:"base_url"`
	ChatModel   string `mapstructure:"chat_model"`
	VisionModel string `mapstructure:"vision_model"`
}

// AzureOpenAIConfig defines Azure OpenAI settings
type AzureOpenAIConfig struct {
	APIKey         string `mapstructure:"api_key"`
	BaseURL        string `mapstructure:"base_url"`
	EmbeddingModel string `mapstructure:"embedding_model"`
	ChatModel      string `mapstructure:"chat_model"`
	APIVersion     string `mapstructure:"api_version"`
}

// CustomModelConfig defines a user-customized OpenAI-compatible model
// Users can add any LLM provider that exposes an OpenAI-compatible API
type CustomModelConfig struct {
	ID          string `mapstructure:"id"`   // 唯一标识，如 "deepseek", "qwen", "moonshot"
	Name        string `mapstructure:"name"` // 显示名称，如 "DeepSeek V3"
	APIKey      string `mapstructure:"api_key"`
	BaseURL     string `mapstructure:"base_url"`    // OpenAI 兼容的 base URL
	ChatModel   string `mapstructure:"chat_model"`  // 模型名称，如 "deepseek-chat"
	Description string `mapstructure:"description"` // 可选描述
}

// LLMProviderConfig is a union type for different LLM providers
type LLMProviderConfig struct {
	OpenAI       OpenAIConfig        `mapstructure:"openai"`
	Anthropic    AnthropicConfig     `mapstructure:"anthropic"`
	Gemini       GeminiConfig        `mapstructure:"gemini"`
	AzureOpenAI  AzureOpenAIConfig   `mapstructure:"azure-openai"`
	CustomModels []CustomModelConfig `mapstructure:"custom_models"` // 用户自定义模型
}

// LLMConfig wraps all LLM related configs
type LLMConfig struct {
	DefaultProvider string            `mapstructure:"default_provider"`
	Providers       LLMProviderConfig `mapstructure:"providers"`
}

// ModelInfo represents an available model for frontend selection
type ModelInfo struct {
	ID          string `json:"id"`         // "openai:gpt-4o-mini", "custom:deepseek"
	Provider    string `json:"provider"`   // "openai", "anthropic", "gemini", "custom", "deepseek", etc.
	Name        string `json:"name"`       // 显示名称
	ModelName   string `json:"model_name"` // 实际模型标识（传给 API 的）
	BaseURL     string `json:"base_url,omitempty"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

// GetAvailableModels returns all configured models for user selection
func (c *LLMConfig) GetAvailableModels() []ModelInfo {
	models := make([]ModelInfo, 0)

	defaultProvider := c.DefaultProvider
	if defaultProvider == "" {
		defaultProvider = "openai"
	}

	// OpenAI models
	if apiKey := strings.TrimSpace(c.Providers.OpenAI.APIKey); apiKey != "" {
		model := c.Providers.OpenAI.ChatModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		models = append(models, ModelInfo{
			ID:        "openai:" + model,
			Provider:  "openai",
			Name:      fmt.Sprintf("OpenAI: %s", model),
			ModelName: model,
			BaseURL:   c.Providers.OpenAI.BaseURL,
			IsDefault: defaultProvider == "openai",
		})
	}

	// Anthropic
	if apiKey := strings.TrimSpace(c.Providers.Anthropic.APIKey); apiKey != "" {
		model := c.Providers.Anthropic.ChatModel
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		baseURL := c.Providers.Anthropic.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com"
		}
		models = append(models, ModelInfo{
			ID:        "anthropic:" + model,
			Provider:  "anthropic",
			Name:      fmt.Sprintf("Anthropic: %s", model),
			ModelName: model,
			BaseURL:   baseURL,
			IsDefault: defaultProvider == "anthropic",
		})
	}

	// Gemini
	if apiKey := strings.TrimSpace(c.Providers.Gemini.APIKey); apiKey != "" {
		model := c.Providers.Gemini.ChatModel
		if model == "" {
			model = "gemini-2.0-flash"
		}
		baseURL := c.Providers.Gemini.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		models = append(models, ModelInfo{
			ID:        "gemini:" + model,
			Provider:  "gemini",
			Name:      fmt.Sprintf("Gemini: %s", model),
			ModelName: model,
			BaseURL:   baseURL,
			IsDefault: defaultProvider == "gemini",
		})
	}

	// Custom / OpenAI-compatible models (DeepSeek, Qwen, Moonshot, etc.)
	for _, cm := range c.Providers.CustomModels {
		if strings.TrimSpace(cm.APIKey) == "" {
			continue
		}
		displayName := cm.Name
		if displayName == "" {
			displayName = cm.ID
		}
		models = append(models, ModelInfo{
			ID:          "custom:" + cm.ID,
			Provider:    "custom",
			Name:        displayName,
			ModelName:   cm.ChatModel,
			BaseURL:     cm.BaseURL,
			Description: cm.Description,
			IsDefault:   false,
		})
	}

	return models
}

type ChatConfig struct {
	HistoryLimit  int `mapstructure:"history_limit"`
	RetrievalTopK int `mapstructure:"retrieval_top_k"`
}

// ParserConfig defines document parser configuration
type ParserConfig struct {
	ChunkSize      int     `mapstructure:"chunk_size"`       // Parent chunk max chars, default 1000
	ChunkOverlap   int     `mapstructure:"chunk_overlap"`    // Parent chunk overlap, default 200
	ChildChunkSize int     `mapstructure:"child_chunk_size"` // Child chunk (for recall) max chars, default 300
	ExtractTables  bool    `mapstructure:"extract_tables"`   // Extract tables, default true
	TableMaxRows   int     `mapstructure:"table_max_rows"`   // Max rows per table, default 100
	TableMaxCols   int     `mapstructure:"table_max_cols"`   // Max cols per table, default 20
	ExtractImages  bool    `mapstructure:"extract_images"`   // Extract images, default true
	ImageMinSize   int     `mapstructure:"image_min_size"`   // Min image size in px, default 50
	VLMEnabled     bool    `mapstructure:"vlm_enabled"`      // Enable VLM for image description, default false
	VLMBatchSize   int     `mapstructure:"vlm_batch_size"`   // VLM batch size, default 5
	OCRThreshold   float32 `mapstructure:"ocr_threshold"`    // Text density threshold for OCR trigger, default 0.1
	OCREnabled     bool    `mapstructure:"ocr_enabled"`      // Enable OCR capability, default true
	DetectHeadings bool    `mapstructure:"detect_headings"`  // Detect heading hierarchy, default true
}

// OCRConfig defines OCR service configuration
type OCRConfig struct {
	Provider string `mapstructure:"provider"` // "rapidocr" or "fallback"
	BaseURL  string `mapstructure:"base_url"` // RapidOCR service URL
}

// VLMConfig defines Vision-Language Model configuration
type VLMConfig struct {
	Enabled bool   `mapstructure:"enabled"` // Enable VLM image description
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"` // e.g. "gpt-4o-mini"
}

// MultimodalConfig defines lightweight document-level visual RAG settings.
type MultimodalConfig struct {
	Enabled                 bool    `mapstructure:"enabled"`
	SaveVisualRegions       bool    `mapstructure:"save_visual_regions"`
	VisualStorageRoot       string  `mapstructure:"visual_storage_root"`
	ChartExtractionEnabled  bool    `mapstructure:"chart_extraction_enabled"`
	VisualRerankEnabled     bool    `mapstructure:"visual_rerank_enabled"`
	VisualGenerationEnabled bool    `mapstructure:"visual_generation_enabled"`
	MaxVisualEvidence       int     `mapstructure:"max_visual_evidence"`
	MinVisualScore          float32 `mapstructure:"min_visual_score"`
}

// KnowledgeGraphConfig controls the notebook-level graph and its optional semantic index.
type KnowledgeGraphConfig struct {
	Enabled       bool                              `mapstructure:"enabled"`
	Extraction    KnowledgeGraphExtractionConfig    `mapstructure:"extraction"`
	SemanticIndex KnowledgeGraphSemanticIndexConfig `mapstructure:"semantic_index"`
}

type KnowledgeGraphExtractionConfig struct {
	MaxChunksPerDocument int    `mapstructure:"max_chunks_per_document"`
	MaxEntitiesPerChunk  int    `mapstructure:"max_entities_per_chunk"`
	RelationStrategy     string `mapstructure:"relation_strategy"`
}

type KnowledgeGraphSemanticIndexConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Provider   string `mapstructure:"provider"`
	Collection string `mapstructure:"collection"`
	Async      bool   `mapstructure:"async"`
	FailOpen   bool   `mapstructure:"fail_open"`
}

// HybridSearchConfig defines hybrid search configuration (Phase 2)
type HybridSearchConfig struct {
	Enabled       bool    `mapstructure:"enabled"`
	RRFK          int     `mapstructure:"rrf_k"`          // RRF 常数（默认 60）
	TopK          int     `mapstructure:"top_k"`          // 初始召回数量（默认 20）
	RerankTopK    int     `mapstructure:"rerank_top_k"`   // Rerank 后返回数量（默认 5）
	MinConfidence float32 `mapstructure:"min_confidence"` // Failover 阈值（默认 0.3）
	MaxRetries    int     `mapstructure:"max_retries"`    // 最大重试次数（默认 1）
}

// RerankerConfig defines reranker configuration (Phase 2)
type RerankerConfig struct {
	Provider string `mapstructure:"provider"` // "cohere" | "fallback"
	Model    string `mapstructure:"model"`    // 默认 "rerank-v3.5"
	BaseURL  string `mapstructure:"base_url"` // 默认 Cohere API URL
	Timeout  int    `mapstructure:"timeout"`  // 秒，默认 5
}

// IntentRewriteConfig defines intent rewrite configuration (Phase 2)
type IntentRewriteConfig struct {
	Enabled           bool `mapstructure:"enabled"`             // 由 ENABLE_INTENT_ROUTING 控制
	LLMRewriteEnabled bool `mapstructure:"llm_rewrite_enabled"` // 由 ENABLE_QUERY_REWRITE 控制
	MaxContextTerms   int  `mapstructure:"max_context_terms"`   // 默认 3
}

// TrustWorkflowConfig defines trustworthy generation workflow configuration (Phase 3)
type TrustWorkflowConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	HighRiskOnly      bool `mapstructure:"high_risk_only"`
	MaxRepairAttempts int  `mapstructure:"max_repair_attempts"`
}

// CitationGuardConfig defines lightweight citation binding and validation settings.
type CitationGuardConfig struct {
	Enabled                   bool    `mapstructure:"enabled"`
	HighRiskOnly              bool    `mapstructure:"high_risk_only"`
	RepairEnabled             bool    `mapstructure:"repair_enabled"`
	MaxRepairAttempts         int     `mapstructure:"max_repair_attempts"`
	RequireParagraphCitations bool    `mapstructure:"require_paragraph_citations"`
	ValidateNumbers           bool    `mapstructure:"validate_numbers"`
	ValidateEntityPhrases     bool    `mapstructure:"validate_entity_phrases"`
	MinCitationCoverage       float64 `mapstructure:"min_citation_coverage"`
	FailClosedForHighRisk     bool    `mapstructure:"fail_closed_for_high_risk"`
}

// Config represents the global configuration
type Config struct {
	App            AppConfig            `mapstructure:"app"`
	Log            LogConfig            `mapstructure:"log"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Cache          CacheConfig          `mapstructure:"cache"`
	Milvus         MilvusConfig         `mapstructure:"milvus"`
	LLM            LLMConfig            `mapstructure:"llm"`
	Auth           AuthConfig           `mapstructure:"auth"`
	Upload         UploadConfig         `mapstructure:"upload"`
	Asynq          AsynqConfig          `mapstructure:"asynq"`
	Chat           ChatConfig           `mapstructure:"chat"`
	Parser         ParserConfig         `mapstructure:"parser"`
	OCR            OCRConfig            `mapstructure:"ocr"`
	VLM            VLMConfig            `mapstructure:"vlm"`
	Multimodal     MultimodalConfig     `mapstructure:"multimodal"`
	KnowledgeGraph KnowledgeGraphConfig `mapstructure:"knowledge_graph"`
	HybridSearch   HybridSearchConfig   `mapstructure:"hybrid_search"`
	Reranker       RerankerConfig       `mapstructure:"reranker"`
	IntentRewrite  IntentRewriteConfig  `mapstructure:"intent_rewrite"`
	TrustWorkflow  TrustWorkflowConfig  `mapstructure:"trust_workflow"`
	CitationGuard  CitationGuardConfig  `mapstructure:"citation_guard"`
}

// Global config instance
var Global *Config

// LoadConfig initializes Viper and loads configuration
func LoadConfig(configPath string) (*Config, error) {
	_ = godotenv.Load()

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	overrideFromEnv(&cfg)
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	Global = &cfg
	return Global, nil
}

func overrideFromEnv(cfg *Config) {
	// Database
	cfg.Database.Postgres.DSN = envString("POSTGRES_DSN", cfg.Database.Postgres.DSN)

	// Cache
	cfg.Cache.Redis.Addr = envString("REDIS_ADDR", cfg.Cache.Redis.Addr)
	cfg.Cache.Redis.Password = envString("REDIS_PASSWORD", cfg.Cache.Redis.Password)

	// Milvus
	cfg.Milvus.Address = envString("MILVUS_ADDRESS", cfg.Milvus.Address)
	cfg.Milvus.Username = envString("MILVUS_USERNAME", cfg.Milvus.Username)
	cfg.Milvus.Password = envString("MILVUS_PASSWORD", cfg.Milvus.Password)
	cfg.Milvus.CollectionName = envString("MILVUS_COLLECTION_NAME", cfg.Milvus.CollectionName)
	if d := os.Getenv("MILVUS_DIMENSION"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			cfg.Milvus.Dimension = v
		}
	}

	// LLM - Default provider
	cfg.LLM.DefaultProvider = envString("LLM_DEFAULT_PROVIDER", cfg.LLM.DefaultProvider)

	// LLM - OpenAI
	cfg.LLM.Providers.OpenAI.APIKey = envString("OPENAI_API_KEY", cfg.LLM.Providers.OpenAI.APIKey)
	cfg.LLM.Providers.OpenAI.BaseURL = envString("OPENAI_BASE_URL", cfg.LLM.Providers.OpenAI.BaseURL)
	cfg.LLM.Providers.OpenAI.EmbeddingModel = envString("OPENAI_EMBEDDING_MODEL", cfg.LLM.Providers.OpenAI.EmbeddingModel)
	cfg.LLM.Providers.OpenAI.ChatModel = envString("OPENAI_CHAT_MODEL", cfg.LLM.Providers.OpenAI.ChatModel)
	cfg.LLM.Providers.OpenAI.VisionModel = envString("OPENAI_VISION_MODEL", cfg.LLM.Providers.OpenAI.VisionModel)

	// LLM - Anthropic
	cfg.LLM.Providers.Anthropic.APIKey = envString("ANTHROPIC_API_KEY", cfg.LLM.Providers.Anthropic.APIKey)
	cfg.LLM.Providers.Anthropic.BaseURL = envString("ANTHROPIC_BASE_URL", cfg.LLM.Providers.Anthropic.BaseURL)
	cfg.LLM.Providers.Anthropic.ChatModel = envString("ANTHROPIC_CHAT_MODEL", cfg.LLM.Providers.Anthropic.ChatModel)
	cfg.LLM.Providers.Anthropic.VisionModel = envString("ANTHROPIC_VISION_MODEL", cfg.LLM.Providers.Anthropic.VisionModel)

	// LLM - Gemini
	cfg.LLM.Providers.Gemini.APIKey = envString("GEMINI_API_KEY", cfg.LLM.Providers.Gemini.APIKey)
	cfg.LLM.Providers.Gemini.BaseURL = envString("GEMINI_BASE_URL", cfg.LLM.Providers.Gemini.BaseURL)
	cfg.LLM.Providers.Gemini.ChatModel = envString("GEMINI_CHAT_MODEL", cfg.LLM.Providers.Gemini.ChatModel)
	cfg.LLM.Providers.Gemini.VisionModel = envString("GEMINI_VISION_MODEL", cfg.LLM.Providers.Gemini.VisionModel)

	// Auth
	cfg.Auth.JWTSecret = envString("JWT_SECRET", cfg.Auth.JWTSecret)

	// Upload
	cfg.Upload.LocalDir = envString("UPLOAD_LOCAL_DIR", cfg.Upload.LocalDir)
	cfg.Upload.AllowedExtName = envString("UPLOAD_ALLOWED_EXT", cfg.Upload.AllowedExtName)
	cfg.Upload.MaxFileSizeMB = envInt64("UPLOAD_MAX_FILE_SIZE_MB", cfg.Upload.MaxFileSizeMB)

	// Asynq
	cfg.Asynq.Concurrency = envInt("ASYNQ_CONCURRENCY", cfg.Asynq.Concurrency)

	// Chat
	cfg.Chat.HistoryLimit = envInt("CHAT_HISTORY_LIMIT", cfg.Chat.HistoryLimit)
	cfg.Chat.RetrievalTopK = envInt("CHAT_RETRIEVAL_TOP_K", cfg.Chat.RetrievalTopK)

	// Parser
	cfg.Parser.ChunkSize = envInt("PARSER_CHUNK_SIZE", cfg.Parser.ChunkSize)
	cfg.Parser.ChunkOverlap = envInt("PARSER_CHUNK_OVERLAP", cfg.Parser.ChunkOverlap)
	cfg.Parser.ChildChunkSize = envInt("PARSER_CHILD_CHUNK_SIZE", cfg.Parser.ChildChunkSize)
	if cfg.Parser.ChunkSize == 0 {
		cfg.Parser.ChunkSize = 1000
	}
	if cfg.Parser.ChunkOverlap == 0 {
		cfg.Parser.ChunkOverlap = 200
	}
	if cfg.Parser.ChildChunkSize == 0 {
		cfg.Parser.ChildChunkSize = 300
	}

	// OCR
	cfg.OCR.BaseURL = envString("OCR_BASE_URL", cfg.OCR.BaseURL)
	cfg.OCR.Provider = envString("OCR_PROVIDER", cfg.OCR.Provider)
	if cfg.OCR.Provider == "" {
		cfg.OCR.Provider = "fallback"
	}

	// VLM
	cfg.VLM.APIKey = envString("VLM_API_KEY", cfg.VLM.APIKey)
	cfg.VLM.BaseURL = envString("VLM_BASE_URL", cfg.VLM.BaseURL)
	cfg.VLM.Model = envString("VLM_MODEL", cfg.VLM.Model)

	// Phase 4: lightweight document-level multimodal RAG
	cfg.Multimodal.Enabled = envBool("MULTIMODAL_ENABLED", cfg.Multimodal.Enabled)
	if !cfg.Multimodal.Enabled && os.Getenv("MULTIMODAL_ENABLED") == "" {
		cfg.Multimodal.Enabled = true
	}
	cfg.Multimodal.SaveVisualRegions = envBool("MULTIMODAL_SAVE_VISUAL_REGIONS", cfg.Multimodal.SaveVisualRegions)
	if !cfg.Multimodal.SaveVisualRegions && os.Getenv("MULTIMODAL_SAVE_VISUAL_REGIONS") == "" {
		cfg.Multimodal.SaveVisualRegions = true
	}
	if cfg.Multimodal.VisualStorageRoot == "" {
		cfg.Multimodal.VisualStorageRoot = "storage/visual"
	}
	cfg.Multimodal.VisualStorageRoot = envString("MULTIMODAL_VISUAL_STORAGE_ROOT", cfg.Multimodal.VisualStorageRoot)
	cfg.Multimodal.ChartExtractionEnabled = envBool("MULTIMODAL_CHART_EXTRACTION_ENABLED", cfg.Multimodal.ChartExtractionEnabled)
	if !cfg.Multimodal.ChartExtractionEnabled && os.Getenv("MULTIMODAL_CHART_EXTRACTION_ENABLED") == "" {
		cfg.Multimodal.ChartExtractionEnabled = true
	}
	cfg.Multimodal.VisualRerankEnabled = envBool("MULTIMODAL_VISUAL_RERANK_ENABLED", cfg.Multimodal.VisualRerankEnabled)
	if !cfg.Multimodal.VisualRerankEnabled && os.Getenv("MULTIMODAL_VISUAL_RERANK_ENABLED") == "" {
		cfg.Multimodal.VisualRerankEnabled = true
	}
	cfg.Multimodal.VisualGenerationEnabled = envBool("MULTIMODAL_VISUAL_GENERATION_ENABLED", cfg.Multimodal.VisualGenerationEnabled)
	if cfg.Multimodal.MaxVisualEvidence == 0 {
		cfg.Multimodal.MaxVisualEvidence = 2
	}
	cfg.Multimodal.MaxVisualEvidence = envInt("MULTIMODAL_MAX_VISUAL_EVIDENCE", cfg.Multimodal.MaxVisualEvidence)
	if cfg.Multimodal.MinVisualScore == 0 {
		cfg.Multimodal.MinVisualScore = 0.35
	}
	cfg.Multimodal.MinVisualScore = float32(envFloat64("MULTIMODAL_MIN_VISUAL_SCORE", float64(cfg.Multimodal.MinVisualScore)))

	// Notebook knowledge graph. Semantic index is off by default so local/dev
	// deployments do not depend on Milvus for graph rendering.
	cfg.KnowledgeGraph.Enabled = envBool("KNOWLEDGE_GRAPH_ENABLED", cfg.KnowledgeGraph.Enabled)
	if !cfg.KnowledgeGraph.Enabled && os.Getenv("KNOWLEDGE_GRAPH_ENABLED") == "" {
		cfg.KnowledgeGraph.Enabled = true
	}
	if cfg.KnowledgeGraph.Extraction.MaxChunksPerDocument == 0 {
		cfg.KnowledgeGraph.Extraction.MaxChunksPerDocument = 14
	}
	if cfg.KnowledgeGraph.Extraction.MaxEntitiesPerChunk == 0 {
		cfg.KnowledgeGraph.Extraction.MaxEntitiesPerChunk = 5
	}
	if cfg.KnowledgeGraph.Extraction.RelationStrategy == "" {
		cfg.KnowledgeGraph.Extraction.RelationStrategy = "conservative"
	}
	cfg.KnowledgeGraph.Extraction.MaxChunksPerDocument = envInt("KNOWLEDGE_GRAPH_MAX_CHUNKS_PER_DOCUMENT", cfg.KnowledgeGraph.Extraction.MaxChunksPerDocument)
	cfg.KnowledgeGraph.Extraction.MaxEntitiesPerChunk = envInt("KNOWLEDGE_GRAPH_MAX_ENTITIES_PER_CHUNK", cfg.KnowledgeGraph.Extraction.MaxEntitiesPerChunk)
	cfg.KnowledgeGraph.Extraction.RelationStrategy = envString("KNOWLEDGE_GRAPH_RELATION_STRATEGY", cfg.KnowledgeGraph.Extraction.RelationStrategy)
	cfg.KnowledgeGraph.SemanticIndex.Enabled = envBool("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_ENABLED", cfg.KnowledgeGraph.SemanticIndex.Enabled)
	cfg.KnowledgeGraph.SemanticIndex.Provider = envString("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_PROVIDER", cfg.KnowledgeGraph.SemanticIndex.Provider)
	if cfg.KnowledgeGraph.SemanticIndex.Provider == "" {
		cfg.KnowledgeGraph.SemanticIndex.Provider = "noop"
	}
	cfg.KnowledgeGraph.SemanticIndex.Collection = envString("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_COLLECTION", cfg.KnowledgeGraph.SemanticIndex.Collection)
	if cfg.KnowledgeGraph.SemanticIndex.Collection == "" {
		cfg.KnowledgeGraph.SemanticIndex.Collection = "notebook_graph_vectors"
	}
	cfg.KnowledgeGraph.SemanticIndex.Async = envBool("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_ASYNC", cfg.KnowledgeGraph.SemanticIndex.Async)
	cfg.KnowledgeGraph.SemanticIndex.FailOpen = envBool("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_FAIL_OPEN", cfg.KnowledgeGraph.SemanticIndex.FailOpen)
	if !cfg.KnowledgeGraph.SemanticIndex.FailOpen && os.Getenv("KNOWLEDGE_GRAPH_SEMANTIC_INDEX_FAIL_OPEN") == "" {
		cfg.KnowledgeGraph.SemanticIndex.FailOpen = true
	}

	// Phase 2: Hybrid Search
	cfg.HybridSearch.Enabled = envBool("HYBRID_SEARCH_ENABLED", cfg.HybridSearch.Enabled)
	if cfg.HybridSearch.RRFK == 0 {
		cfg.HybridSearch.RRFK = 60
	}
	if cfg.HybridSearch.TopK == 0 {
		cfg.HybridSearch.TopK = 20
	}
	if cfg.HybridSearch.RerankTopK == 0 {
		cfg.HybridSearch.RerankTopK = 5
	}
	if cfg.HybridSearch.MinConfidence == 0 {
		cfg.HybridSearch.MinConfidence = 0.3
	}
	if cfg.HybridSearch.MaxRetries == 0 {
		cfg.HybridSearch.MaxRetries = 1
	}
	cfg.HybridSearch.TopK = envInt("HYBRID_SEARCH_TOP_K", cfg.HybridSearch.TopK)
	cfg.HybridSearch.RerankTopK = envInt("HYBRID_SEARCH_RERANK_TOP_K", cfg.HybridSearch.RerankTopK)

	// Phase 2: Reranker (API Key 从环境变量读取)
	cfg.Reranker.Provider = envString("RERANKER_PROVIDER", cfg.Reranker.Provider)
	cfg.Reranker.Model = envString("RERANKER_MODEL", cfg.Reranker.Model)
	cfg.Reranker.BaseURL = envString("RERANKER_BASE_URL", cfg.Reranker.BaseURL)
	cfg.Reranker.Timeout = envInt("RERANKER_TIMEOUT", cfg.Reranker.Timeout)
	if cfg.Reranker.Provider == "" {
		cfg.Reranker.Provider = "cohere"
	}
	if cfg.Reranker.Model == "" {
		cfg.Reranker.Model = "rerank-v3.5"
	}
	if cfg.Reranker.BaseURL == "" {
		cfg.Reranker.BaseURL = "https://api.cohere.com/v2"
	}
	if cfg.Reranker.Timeout == 0 {
		cfg.Reranker.Timeout = 5
	}

	// Phase 2: Intent Rewrite (由 .env 开关控制)
	cfg.IntentRewrite.Enabled = envBool("ENABLE_INTENT_ROUTING", cfg.IntentRewrite.Enabled)
	cfg.IntentRewrite.LLMRewriteEnabled = envBool("ENABLE_QUERY_REWRITE", cfg.IntentRewrite.LLMRewriteEnabled)
	if cfg.IntentRewrite.MaxContextTerms == 0 {
		cfg.IntentRewrite.MaxContextTerms = 3
	}
	cfg.IntentRewrite.MaxContextTerms = envInt("INTENT_MAX_CONTEXT_TERMS", cfg.IntentRewrite.MaxContextTerms)

	// Phase 3: Trustworthy Generation Workflow
	cfg.TrustWorkflow.Enabled = envBool("ENABLE_TRUST_WORKFLOW", cfg.TrustWorkflow.Enabled)
	cfg.TrustWorkflow.HighRiskOnly = envBool("TRUST_WORKFLOW_HIGH_RISK_ONLY", cfg.TrustWorkflow.HighRiskOnly)
	if cfg.TrustWorkflow.MaxRepairAttempts == 0 {
		cfg.TrustWorkflow.MaxRepairAttempts = 1
	}
	cfg.TrustWorkflow.MaxRepairAttempts = envInt("TRUST_WORKFLOW_MAX_REPAIR_ATTEMPTS", cfg.TrustWorkflow.MaxRepairAttempts)

	// Lightweight citation guard for default notebook chat
	cfg.CitationGuard.Enabled = envBool("ENABLE_CITATION_GUARD", cfg.CitationGuard.Enabled)
	if !cfg.CitationGuard.Enabled && os.Getenv("ENABLE_CITATION_GUARD") == "" {
		cfg.CitationGuard.Enabled = true
	}
	cfg.CitationGuard.HighRiskOnly = envBool("CITATION_GUARD_HIGH_RISK_ONLY", cfg.CitationGuard.HighRiskOnly)
	cfg.CitationGuard.RepairEnabled = envBool("CITATION_GUARD_REPAIR_ENABLED", cfg.CitationGuard.RepairEnabled)
	cfg.CitationGuard.MaxRepairAttempts = envInt("CITATION_GUARD_MAX_REPAIR_ATTEMPTS", cfg.CitationGuard.MaxRepairAttempts)
	if !cfg.CitationGuard.RequireParagraphCitations {
		cfg.CitationGuard.RequireParagraphCitations = true
	}
	cfg.CitationGuard.RequireParagraphCitations = envBool("CITATION_GUARD_REQUIRE_PARAGRAPH_CITATIONS", cfg.CitationGuard.RequireParagraphCitations)
	if !cfg.CitationGuard.ValidateNumbers {
		cfg.CitationGuard.ValidateNumbers = true
	}
	cfg.CitationGuard.ValidateNumbers = envBool("CITATION_GUARD_VALIDATE_NUMBERS", cfg.CitationGuard.ValidateNumbers)
	if cfg.CitationGuard.MinCitationCoverage == 0 {
		cfg.CitationGuard.MinCitationCoverage = 0.8
	}
	cfg.CitationGuard.MinCitationCoverage = envFloat64("CITATION_GUARD_MIN_COVERAGE", cfg.CitationGuard.MinCitationCoverage)
	if !cfg.CitationGuard.FailClosedForHighRisk {
		cfg.CitationGuard.FailClosedForHighRisk = true
	}
	cfg.CitationGuard.FailClosedForHighRisk = envBool("CITATION_GUARD_FAIL_CLOSED_FOR_HIGH_RISK", cfg.CitationGuard.FailClosedForHighRisk)
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return fmt.Errorf("missing required config JWT_SECRET")
	}

	// Validate LLM provider configuration
	defaultProvider := cfg.LLM.DefaultProvider
	if defaultProvider == "" {
		defaultProvider = "openai"
	}

	var hasValidProvider bool
	switch defaultProvider {
	case "openai":
		hasValidProvider = strings.TrimSpace(cfg.LLM.Providers.OpenAI.APIKey) != ""
	case "anthropic":
		hasValidProvider = strings.TrimSpace(cfg.LLM.Providers.Anthropic.APIKey) != ""
	case "gemini":
		hasValidProvider = strings.TrimSpace(cfg.LLM.Providers.Gemini.APIKey) != ""
	case "custom":
		// Custom: 至少有一个自定义模型配置了 API Key
		for _, cm := range cfg.LLM.Providers.CustomModels {
			if strings.TrimSpace(cm.APIKey) != "" {
				hasValidProvider = true
				break
			}
		}
	default:
		return fmt.Errorf("unsupported LLM provider: %s (supported: openai, anthropic, gemini, custom)", defaultProvider)
	}

	if !hasValidProvider {
		return fmt.Errorf("missing API key for default LLM provider: %s", defaultProvider)
	}

	// Validate Milvus configuration
	milvusAddress := strings.TrimSpace(cfg.Milvus.Address)
	milvusPassword := strings.TrimSpace(cfg.Milvus.Password)
	if milvusAddress != "" && milvusPassword == "" {
		return fmt.Errorf("MILVUS_PASSWORD is required when MILVUS_ADDRESS is set")
	}
	if milvusAddress == "" && milvusPassword != "" {
		return fmt.Errorf("MILVUS_ADDRESS is required when MILVUS_PASSWORD is set")
	}

	// Validate Chat configuration
	if cfg.Chat.HistoryLimit <= 0 {
		return fmt.Errorf("CHAT_HISTORY_LIMIT must be greater than 0")
	}
	if cfg.Chat.RetrievalTopK <= 0 {
		return fmt.Errorf("CHAT_RETRIEVAL_TOP_K must be greater than 0")
	}
	if cfg.Asynq.Concurrency <= 0 {
		return fmt.Errorf("ASYNQ_CONCURRENCY must be greater than 0")
	}

	return nil
}

func envString(key, current string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return current
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return current
	}
	return trimmed
}

func envInt(key string, current int) int {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return current
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return current
	}
	return parsed
}

func envInt64(key string, current int64) int64 {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return current
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil {
		return current
	}
	return parsed
}

func envBool(key string, current bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return current
	}
	return strings.ToLower(strings.TrimSpace(val)) == "true"
}

func envFloat32(key string, current float32) float32 {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return current
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 32)
	if err != nil {
		return current
	}
	return float32(parsed)
}

func envFloat64(key string, current float64) float64 {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return current
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return current
	}
	return parsed
}
