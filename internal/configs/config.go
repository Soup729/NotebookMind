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
}

// LLMConfig wraps all LLM related configs
type LLMConfig struct {
	OpenAI OpenAIConfig `mapstructure:"openai"`
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
	cfg.Database.Postgres.DSN = envString("POSTGRES_DSN", cfg.Database.Postgres.DSN)
	cfg.Cache.Redis.Addr = envString("REDIS_ADDR", cfg.Cache.Redis.Addr)
	cfg.Cache.Redis.Password = envString("REDIS_PASSWORD", cfg.Cache.Redis.Password)
	cfg.Milvus.Address = envString("MILVUS_ADDRESS", cfg.Milvus.Address)
	cfg.Milvus.Username = envString("MILVUS_USERNAME", cfg.Milvus.Username)
	cfg.Milvus.Password = envString("MILVUS_PASSWORD", cfg.Milvus.Password)
	cfg.Milvus.CollectionName = envString("MILVUS_COLLECTION_NAME", cfg.Milvus.CollectionName)
	cfg.LLM.OpenAI.APIKey = envString("OPENAI_API_KEY", cfg.LLM.OpenAI.APIKey)
	cfg.LLM.OpenAI.BaseURL = envString("OPENAI_BASE_URL", cfg.LLM.OpenAI.BaseURL)
	cfg.LLM.OpenAI.EmbeddingModel = envString("OPENAI_EMBEDDING_MODEL", cfg.LLM.OpenAI.EmbeddingModel)
	cfg.LLM.OpenAI.ChatModel = envString("OPENAI_CHAT_MODEL", cfg.LLM.OpenAI.ChatModel)
	cfg.Auth.JWTSecret = envString("JWT_SECRET", cfg.Auth.JWTSecret)
	cfg.Upload.LocalDir = envString("UPLOAD_LOCAL_DIR", cfg.Upload.LocalDir)
	cfg.Upload.AllowedExtName = envString("UPLOAD_ALLOWED_EXT", cfg.Upload.AllowedExtName)
	cfg.Upload.MaxFileSizeMB = envInt64("UPLOAD_MAX_FILE_SIZE_MB", cfg.Upload.MaxFileSizeMB)
	cfg.Asynq.Concurrency = envInt("ASYNQ_CONCURRENCY", cfg.Asynq.Concurrency)
	cfg.Chat.HistoryLimit = envInt("CHAT_HISTORY_LIMIT", cfg.Chat.HistoryLimit)
	cfg.Chat.RetrievalTopK = envInt("CHAT_RETRIEVAL_TOP_K", cfg.Chat.RetrievalTopK)
}

func validateConfig(cfg *Config) error {
	if strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return fmt.Errorf("missing required config JWT_SECRET")
	}
	if strings.TrimSpace(cfg.LLM.OpenAI.APIKey) == "" {
		return fmt.Errorf("missing required config OPENAI_API_KEY")
	}
	milvusAddress := strings.TrimSpace(cfg.Milvus.Address)
	milvusPassword := strings.TrimSpace(cfg.Milvus.Password)
	if milvusAddress != "" && milvusPassword == "" {
		return fmt.Errorf("MILVUS_PASSWORD is required when MILVUS_ADDRESS is set")
	}
	if milvusAddress == "" && milvusPassword != "" {
		return fmt.Errorf("MILVUS_ADDRESS is required when MILVUS_PASSWORD is set")
	}
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
