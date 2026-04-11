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
	ID          string `mapstructure:"id"`           // 唯一标识，如 "deepseek", "qwen", "moonshot"
	Name        string `mapstructure:"name"`          // 显示名称，如 "DeepSeek V3"
	APIKey      string `mapstructure:"api_key"`
	BaseURL     string `mapstructure:"base_url"`       // OpenAI 兼容的 base URL
	ChatModel   string `mapstructure:"chat_model"`    // 模型名称，如 "deepseek-chat"
	Description string `mapstructure:"description"`  // 可选描述
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
	DefaultProvider string             `mapstructure:"default_provider"`
	Providers       LLMProviderConfig `mapstructure:"providers"`
}

// ModelInfo represents an available model for frontend selection
type ModelInfo struct {
	ID          string `json:"id"`           // "openai:gpt-4o-mini", "custom:deepseek"
	Provider    string `json:"provider"`     // "openai", "anthropic", "gemini", "custom", "deepseek", etc.
	Name        string `json:"name"`         // 显示名称
	ModelName   string `json:"model_name"`   // 实际模型标识（传给 API 的）
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
	ChunkSize        int     `mapstructure:"chunk_size"`         // Parent chunk max chars, default 1000
	ChunkOverlap     int     `mapstructure:"chunk_overlap"`      // Parent chunk overlap, default 200
	ChildChunkSize   int     `mapstructure:"child_chunk_size"`   // Child chunk (for recall) max chars, default 300
	ExtractTables    bool    `mapstructure:"extract_tables"`     // Extract tables, default true
	TableMaxRows     int     `mapstructure:"table_max_rows"`     // Max rows per table, default 100
	TableMaxCols     int     `mapstructure:"table_max_cols"`     // Max cols per table, default 20
	ExtractImages    bool    `mapstructure:"extract_images"`     // Extract images, default true
	ImageMinSize     int     `mapstructure:"image_min_size"`    // Min image size in px, default 50
	VLMEnabled       bool    `mapstructure:"vlm_enabled"`        // Enable VLM for image description, default false
	VLMBatchSize     int     `mapstructure:"vlm_batch_size"`     // VLM batch size, default 5
	OCRThreshold     float32 `mapstructure:"ocr_threshold"`      // Text density threshold for OCR trigger, default 0.1
	OCREnabled       bool    `mapstructure:"ocr_enabled"`        // Enable OCR capability, default true
	DetectHeadings   bool    `mapstructure:"detect_headings"`    // Detect heading hierarchy, default true
}

// OCRConfig defines OCR service configuration
type OCRConfig struct {
	Provider string `mapstructure:"provider"` // "rapidocr" or "fallback"
	BaseURL  string `mapstructure:"base_url"` // RapidOCR service URL
}

// VLMConfig defines Vision-Language Model configuration
type VLMConfig struct {
	Enabled bool   `mapstructure:"enabled"`  // Enable VLM image description
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`   // e.g. "gpt-4o-mini"
}

// Config represents the global configuration
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Log      LogConfig      `mapstructure:"log"`
	Database DatabaseConfig `mapstructure:"database"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Milvus   MilvusConfig   `mapstructure:"milvus"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Upload   UploadConfig   `mapstructure:"upload"`
	Asynq    AsynqConfig    `mapstructure:"asynq"`
	Chat     ChatConfig     `mapstructure:"chat"`
	Parser   ParserConfig   `mapstructure:"parser"`
	OCR      OCRConfig      `mapstructure:"ocr"`
	VLM      VLMConfig      `mapstructure:"vlm"`
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
	if cfg.Parser.ChunkSize == 0 { cfg.Parser.ChunkSize = 1000 }
	if cfg.Parser.ChunkOverlap == 0 { cfg.Parser.ChunkOverlap = 200 }
	if cfg.Parser.ChildChunkSize == 0 { cfg.Parser.ChildChunkSize = 300 }
	
	// OCR
	cfg.OCR.BaseURL = envString("OCR_BASE_URL", cfg.OCR.BaseURL)
	cfg.OCR.Provider = envString("OCR_PROVIDER", cfg.OCR.Provider)
	if cfg.OCR.Provider == "" { cfg.OCR.Provider = "fallback" }

	// VLM
	cfg.VLM.APIKey = envString("VLM_API_KEY", cfg.VLM.APIKey)
	cfg.VLM.BaseURL = envString("VLM_BASE_URL", cfg.VLM.BaseURL)
	cfg.VLM.Model = envString("VLM_MODEL", cfg.VLM.Model)
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
