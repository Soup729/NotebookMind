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

// LLMProviderConfig is a union type for different LLM providers
type LLMProviderConfig struct {
	OpenAI       OpenAIConfig       `mapstructure:"openai"`
	Anthropic    AnthropicConfig    `mapstructure:"anthropic"`
	Gemini       GeminiConfig       `mapstructure:"gemini"`
	AzureOpenAI  AzureOpenAIConfig `mapstructure:"azure-openai"`
}

// LLMConfig wraps all LLM related configs
type LLMConfig struct {
	DefaultProvider string             `mapstructure:"default_provider"`
	Providers       LLMProviderConfig `mapstructure:"providers"`
}

type ChatConfig struct {
	HistoryLimit  int `mapstructure:"history_limit"`
	RetrievalTopK int `mapstructure:"retrieval_top_k"`
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
	default:
		return fmt.Errorf("unsupported LLM provider: %s", defaultProvider)
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
