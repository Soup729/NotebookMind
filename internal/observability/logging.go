package observability

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// MetricsKey 定义上下文中的指标 key
type ctxKey string

const (
	MetricsContextKey ctxKey = "metrics_context"
)

// ChatMetrics 聊天请求的核心指标
type ChatMetrics struct {
	// 请求标识
	RequestID        string   `json:"request_id"`
	SessionID        string   `json:"session_id"`
	UserID           string   `json:"user_id"`
	ServiceType      string   `json:"service_type"` // "chat" | "notebook_chat"

	// 时间指标 (毫秒)
	TotalLatencyMs   int64    `json:"total_latency_ms"`
	RetrievalLatency int64    `json:"retrieval_latency_ms"`
	LLMLatencyMs     int64    `json:"llm_latency_ms"`

	// 检索质量
	RetrievedChunks  int      `json:"retrieved_chunks"`
	TopK             int      `json:"top_k"`

	// Token 用量
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`

	// 来源引用
	SourceCount       int      `json:"source_count"`
	SourceDocumentIDs []string `json:"source_document_ids"`

	// 错误信息
	ErrorType string `json:"error_type,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`

	// 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// ProcessingMetrics 文档处理指标
type ProcessingMetrics struct {
	TaskID           string `json:"task_id"`
	DocumentID       string `json:"document_id"`
	UserID           string `json:"user_id"`
	FileName         string `json:"file_name"`

	// 各阶段延迟
	ExtractLatencyMs  int64 `json:"extract_latency_ms"`
	SplitLatencyMs    int64 `json:"split_latency_ms"`
	EmbedLatencyMs    int64 `json:"embed_latency_ms"`
	IndexLatencyMs    int64 `json:"index_latency_ms"`
	GuideGenLatencyMs int64 `json:"guide_gen_latency_ms"`
	TotalLatencyMs    int64 `json:"total_latency_ms"`

	// 处理结果
	PageCount int    `json:"page_count"`
	ChunkCount int   `json:"chunk_count"`
	Status    string `json:"status"` // "success" | "failed"
	ErrorMsg  string `json:"error_msg,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// NewChatMetrics 创建聊天指标实例
func NewChatMetrics(requestID, sessionID, userID, serviceType string) *ChatMetrics {
	return &ChatMetrics{
		RequestID:   requestID,
		SessionID:   sessionID,
		UserID:      userID,
		ServiceType: serviceType,
		Timestamp:   time.Now(),
	}
}

// NewProcessingMetrics 创建文档处理指标实例
func NewProcessingMetrics(taskID, documentID, userID, fileName string) *ProcessingMetrics {
	return &ProcessingMetrics{
		TaskID:     taskID,
		DocumentID: documentID,
		UserID:     userID,
		FileName:   fileName,
		Timestamp:  time.Now(),
	}
}

// InitLogger 初始化全局日志器
func InitLogger(env string, logLevel string) error {
	var config zap.Config

	switch env {
	case "production":
		config = zap.NewProductionConfig()
	case "development":
		config = zap.NewDevelopmentConfig()
	default:
		config = zap.NewProductionConfig()
	}

	// 配置日志级别
	switch logLevel {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zapcore.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	default:
		config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}

	// 自定义编码器配置：时间格式 + 调用者信息
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.EncodeDuration = zapcore.MillisDurationEncoder

	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return err
	}

	zap.ReplaceGlobals(logger)

	return nil
}

// GetLogger 获取全局日志器
func GetLogger() *zap.Logger {
	return zap.L()
}

// LogChatRequest 记录完整的聊天请求指标（用于在线采集）
func LogChatRequest(metrics *ChatMetrics) {
	fields := []zap.Field{
		zap.String("event", "chat_request"),
		zap.String("request_id", metrics.RequestID),
		zap.String("session_id", metrics.SessionID),
		zap.String("user_id", metrics.UserID),
		zap.String("service_type", metrics.ServiceType),
		zap.Int64("total_latency_ms", metrics.TotalLatencyMs),
		zap.Int64("retrieval_latency_ms", metrics.RetrievalLatency),
		zap.Int64("llm_latency_ms", metrics.LLMLatencyMs),
		zap.Int("retrieved_chunks", metrics.RetrievedChunks),
		zap.Int("top_k", metrics.TopK),
		zap.Int("prompt_tokens", metrics.PromptTokens),
		zap.Int("completion_tokens", metrics.CompletionTokens),
		zap.Int("total_tokens", metrics.TotalTokens),
		zap.Int("source_count", metrics.SourceCount),
		zap.Strings("source_document_ids", metrics.SourceDocumentIDs),
		zap.Time("timestamp", metrics.Timestamp),
	}

	if metrics.ErrorType != "" {
		fields = append(fields,
			zap.String("error_type", metrics.ErrorType),
			zap.String("error_msg", metrics.ErrorMsg),
		)
		zap.L().Error("chat request completed with error", fields...)
	} else {
		zap.L().Info("chat request completed", fields...)
	}
}

// LogRetrievalEvent 记录检索事件详情
func LogRetrievalEvent(query string, topK int, retrievedCount int, latencyMs int64, documentIDs []string) {
	zap.L().Info("retrieval event",
		zap.String("event", "retrieval"),
		zap.String("query", query),
		zap.Int("top_k", topK),
		zap.Int("retrieved_count", retrievedCount),
		zap.Int64("latency_ms", latencyMs),
		zap.Strings("document_ids", documentIDs),
	)
}

// LogLLMCall 记录 LLM 调用事件
func LogLLMCall(callType string, promptTokens, completionTokens, totalTokens int, latencyMs int64, model string, err error) {
	fields := []zap.Field{
		zap.String("event", "llm_call"),
		zap.String("call_type", callType), // "generate" | "embed" | "reflection"
		zap.Int("prompt_tokens", promptTokens),
		zap.Int("completion_tokens", completionTokens),
		zap.Int("total_tokens", totalTokens),
		zap.Int64("latency_ms", latencyMs),
		zap.String("model", model),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		zap.L().Error("LLM call failed", fields...)
	} else {
		zap.L().Info("LLM call completed", fields...)
	}
}

// LogDocumentProcessing 记录文档处理事件
func LogDocumentProcessing(metrics *ProcessingMetrics) {
	fields := []zap.Field{
		zap.String("event", "document_processing"),
		zap.String("task_id", metrics.TaskID),
		zap.String("document_id", metrics.DocumentID),
		zap.String("user_id", metrics.UserID),
		zap.String("file_name", metrics.FileName),
		zap.Int64("extract_latency_ms", metrics.ExtractLatencyMs),
		zap.Int64("split_latency_ms", metrics.SplitLatencyMs),
		zap.Int64("embed_latency_ms", metrics.EmbedLatencyMs),
		zap.Int64("index_latency_ms", metrics.IndexLatencyMs),
		zap.Int64("guide_gen_latency_ms", metrics.GuideGenLatencyMs),
		zap.Int64("total_latency_ms", metrics.TotalLatencyMs),
		zap.Int("page_count", metrics.PageCount),
		zap.Int("chunk_count", metrics.ChunkCount),
		zap.String("status", metrics.Status),
		zap.Time("timestamp", metrics.Timestamp),
	}

	if metrics.ErrorMsg != "" {
		fields = append(fields, zap.String("error_msg", metrics.ErrorMsg))
		zap.L().Error("document processing failed", fields...)
	} else {
		zap.L().Info("document processing completed", fields...)
	}
}

// ContextWithMetrics 将指标注入 context
func ContextWithMetrics(ctx context.Context, m interface{}) context.Context {
	return context.WithValue(ctx, MetricsContextKey, m)
}

// MetricsFromContext 从 context 中提取指标
func MetricsFromContext(ctx context.Context) interface{} {
	return ctx.Value(MetricsContextKey)
}

// Stopwatch 简易计时器
type Stopwatch struct {
	start time.Time
}

// NewStopwatch 创建新计时器
func NewStopwatch() *Stopwatch {
	return &Stopwatch{start: time.Now()}
}

// ElapsedMs 返回经过的毫秒数
func (s *Stopwatch) ElapsedMs() int64 {
	return time.Since(s.start).Milliseconds()
}

// Elapsed 返回经过的时间
func (s *Stopwatch) Elapsed() time.Duration {
	return time.Since(s.start)
}

// Reset 重置计时器
func (s *Stopwatch) Reset() {
	s.start = time.Now()
}
