package database

import (
	"context"
	"fmt"
	"time"

	"NotebookAI/internal/configs"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitPostgres initializes the PostgreSQL connection pool via GORM
func InitPostgres(cfg *configs.PostgresConfig) (*gorm.DB, error) {
	zap.L().Info("Connecting to PostgreSQL...", zap.String("dsn", cfg.DSN))

	// Configure GORM logger based on Zap
	newLogger := gormlogger.Default.LogMode(gormlogger.Info) // Default Info, could be adjusted by environment

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql db: %w", err)
	}

	// Set connection pool limits
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	zap.L().Info("PostgreSQL connection established successfully")
	DB = db
	return DB, nil
}

// ClosePostgres cleanly shuts down the DB connection
func ClosePostgres() {
	if DB != nil {
		if sqlDB, err := DB.DB(); err == nil {
			zap.L().Info("Closing PostgreSQL connection")
			_ = sqlDB.Close()
		}
	}
}
