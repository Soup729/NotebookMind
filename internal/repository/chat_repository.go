package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type ChatMessage struct {
	ID        uint      `gorm:"primaryKey"`
	SessionID string    `gorm:"size:64;index;not null"`
	UserID    string    `gorm:"size:64;index;not null"`
	Role      string    `gorm:"size:16;not null"`
	Content   string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"index"`
}

type ChatRepository interface {
	SaveMessage(ctx context.Context, message *ChatMessage) error
	ListSessionMessages(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error)
}

type chatRepository struct {
	db *gorm.DB
}

func NewChatRepository(db *gorm.DB) (ChatRepository, error) {
	if err := db.AutoMigrate(&ChatMessage{}); err != nil {
		return nil, fmt.Errorf("auto migrate chat_message: %w", err)
	}

	return &chatRepository{db: db}, nil
}

func (r *chatRepository) SaveMessage(ctx context.Context, message *ChatMessage) error {
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("save chat message: %w", err)
	}

	return nil
}

func (r *chatRepository) ListSessionMessages(ctx context.Context, userID, sessionID string, limit int) ([]ChatMessage, error) {
	var messages []ChatMessage
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("created_at asc").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("query session messages: %w", err)
	}

	return messages, nil
}
