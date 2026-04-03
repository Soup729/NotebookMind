package logger

import (
	"os"

	"enterprise-pdf-ai/internal/configs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

// InitLogger initializes the global zap logger with lumberjack for log rotation
func InitLogger(cfg *configs.LogConfig) error {
	// Setup log rotation
	writeSyncer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSize, // megabytes
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge, // days
		Compress:   cfg.Compress,
	})

	// Add console output for development
	consoleSyncer := zapcore.AddSync(os.Stdout)
	coreSyncer := zapcore.NewMultiWriteSyncer(writeSyncer, consoleSyncer)

	// Setup encoder (JSON format for enterprise logs)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// Set log level
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel // Default to info if parse fails
	}

	core := zapcore.NewCore(encoder, coreSyncer, level)

	// Add caller and stacktrace options
	Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// Replace global zap logger
	zap.ReplaceGlobals(Log)

	return nil
}

// Sync flushes any buffered log entries
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
