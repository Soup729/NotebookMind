package cache

import (
	"context"
	"fmt"
	"time"

	"enterprise-pdf-ai/internal/configs"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var RedisClient *redis.Client

// InitRedis initializes the Redis connection pool
func InitRedis(cfg *configs.RedisConfig) (*redis.Client, error) {
	zap.L().Info("Connecting to Redis...", zap.String("addr", cfg.Addr))

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	zap.L().Info("Redis connection established successfully")
	RedisClient = client
	return RedisClient, nil
}

// CloseRedis cleanly shuts down the Redis connection
func CloseRedis() {
	if RedisClient != nil {
		zap.L().Info("Closing Redis connection")
		_ = RedisClient.Close()
	}
}
