package repository

import (
	"context"
	"fmt"
	"time"

	"NotebookAI/internal/models"
	"gorm.io/gorm"
)

type ChatRepository interface {
	CreateSession(ctx context.Context, session *models.ChatSession) error
	GetSession(ctx context.Context, userID, sessionID string) (*models.ChatSession, error)
	ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error)
	ListSessionsByNotebook(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error)
	SaveMessage(ctx context.Context, message *models.ChatMessage) error
	ListSessionMessages(ctx context.Context, userID, sessionID string, limit int) ([]models.ChatMessage, error)
	UpdateSessionActivity(ctx context.Context, userID, sessionID, title string, activityAt time.Time) error
	CountSessions(ctx context.Context, userID string) (int64, error)
	CountMessages(ctx context.Context, userID string) (int64, error)
	SumTokens(ctx context.Context, userID string) (int64, error)
	DailyTokenUsage(ctx context.Context, userID string, days int) ([]DailyUsageRow, error)
	DeleteSession(ctx context.Context, sessionID string) error
	DeleteMessagesBySession(ctx context.Context, sessionID string) error
}

type DailyUsageRow struct {
	Day    time.Time
	Tokens int64
}

type chatRepository struct {
	db *gorm.DB
}

func NewChatRepository(db *gorm.DB) (ChatRepository, error) {
	// 手动处理 notebook_id 迁移：先添加可空列，更新现有数据，再设为 NOT NULL
	if err := db.Exec(`ALTER TABLE "chat_sessions" ADD "notebook_id" varchar(36)`).Error; err != nil {
		// 忽略 "column already exists" 错误
		if !isColumnExistsError(err) {
			return nil, fmt.Errorf("add notebook_id column: %w", err)
		}
	}

	// 更新现有 NULL 值为空字符串
	if err := db.Exec(`UPDATE "chat_sessions" SET "notebook_id" = '' WHERE "notebook_id" IS NULL`).Error; err != nil {
		return nil, fmt.Errorf("update null notebook_id: %w", err)
	}

	// 设置 NOT NULL 约束
	if err := db.Exec(`ALTER TABLE "chat_sessions" ALTER COLUMN "notebook_id" SET NOT NULL`).Error; err != nil {
		// 忽略如果约束已存在
		if !isNotNullError(err) {
			return nil, fmt.Errorf("set notebook_id not null: %w", err)
		}
	}

	// 修复 session_id 列类型：从 bigint 改为 varchar(36)，以支持 UUID
	if err := fixChatMessageSessionIDType(db); err != nil {
		return nil, fmt.Errorf("fix chat_message session_id type: %w", err)
	}

	// 修复 id 列类型：从 bigint/bigserial 改为 varchar(36)，以支持 UUID 主键
	if err := fixChatMessageTypeIDColumn(db); err != nil {
		return nil, fmt.Errorf("fix chat_message id type: %w", err)
	}

	if err := db.AutoMigrate(&models.ChatSession{}, &models.ChatMessage{}); err != nil {
		return nil, fmt.Errorf("auto migrate chat tables: %w", err)
	}

	return &chatRepository{db: db}, nil
}

// fixChatMessageSessionIDType 将 chat_messages.session_id 从 bigint 改为 varchar(36)
func fixChatMessageSessionIDType(db *gorm.DB) error {
	// 检查当前列类型
	var columnType string
	row := db.Raw(`SELECT data_type FROM information_schema.columns
		WHERE table_name = 'chat_messages' AND column_name = 'session_id'`).Row()
	if row != nil {
		_ = row.Scan(&columnType)
	}

	// 如果是 bigint 或 integer 等数值类型，则改为 varchar(36)
	if columnType == "bigint" || columnType == "integer" || columnType == "smallint" || columnType == "bigserial" {
		// 先删除外键约束（如果存在）
		db.Exec(`ALTER TABLE "chat_messages" DROP CONSTRAINT IF EXISTS "fk_chat_messages_session"`)

		// 修改列类型
		if err := db.Exec(`ALTER TABLE "chat_messages" ALTER COLUMN "session_id" TYPE varchar(36)`).Error; err != nil {
			return fmt.Errorf("alter session_id type: %w", err)
		}
	}
	return nil
}

// fixChatMessageTypeIDColumn 将 chat_messages.id 从 bigint/bigserial 改为 varchar(36)，以支持 UUID 主键
func fixChatMessageTypeIDColumn(db *gorm.DB) error {
	// 检查当前列类型
	var columnType string
	row := db.Raw(`SELECT data_type FROM information_schema.columns
		WHERE table_name = 'chat_messages' AND column_name = 'id'`).Row()
	if row != nil {
		_ = row.Scan(&columnType)
	}

	// 如果是 bigint 或 bigserial 等数值类型，则改为 varchar(36)
	if columnType == "bigint" || columnType == "bigserial" || columnType == "integer" {
		// PostgreSQL: 先 drop default (serial), 再改类型, 再加 NOT NULL
		db.Exec(`ALTER TABLE "chat_messages" ALTER COLUMN "id" DROP DEFAULT`)
		if err := db.Exec(`ALTER TABLE "chat_messages" ALTER COLUMN "id" TYPE varchar(36) USING id::varchar`).Error; err != nil {
			return fmt.Errorf("alter chat_message id type to varchar(36): %w", err)
		}
		// 确保 NOT NULL 约束
		db.Exec(`ALTER TABLE "chat_messages" ALTER COLUMN "id" SET NOT NULL`)
	}
	return nil
}

func isColumnExistsError(err error) bool {
	return err != nil && (contains(err.Error(), "already exists") || contains(err.Error(), "duplicate column"))
}

func isNotNullError(err error) bool {
	return err != nil && contains(err.Error(), "already exists")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (r *chatRepository) CreateSession(ctx context.Context, session *models.ChatSession) error {
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return fmt.Errorf("create chat session: %w", err)
	}
	return nil
}

func (r *chatRepository) GetSession(ctx context.Context, userID, sessionID string) (*models.ChatSession, error) {
	var session models.ChatSession
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND id = ?", userID, sessionID).
		First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get chat session: %w", err)
	}
	return &session, nil
}

func (r *chatRepository) ListSessions(ctx context.Context, userID string) ([]models.ChatSession, error) {
	var sessions []models.ChatSession
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_message_at desc").
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	return sessions, nil
}

// ListSessionsByNotebook 查询指定笔记本下的所有会话
func (r *chatRepository) ListSessionsByNotebook(ctx context.Context, userID, notebookID string) ([]models.ChatSession, error) {
	var sessions []models.ChatSession
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND notebook_id = ?", userID, notebookID).
		Order("last_message_at desc").
		Find(&sessions).Error; err != nil {
		return nil, fmt.Errorf("list sessions by notebook: %w", err)
	}
	return sessions, nil
}

func (r *chatRepository) SaveMessage(ctx context.Context, message *models.ChatMessage) error {
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("save chat message: %w", err)
	}
	return nil
}

func (r *chatRepository) ListSessionMessages(ctx context.Context, userID, sessionID string, limit int) ([]models.ChatMessage, error) {
	var messages []models.ChatMessage
	query := r.db.WithContext(ctx).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Order("created_at asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("list session messages: %w", err)
	}
	return messages, nil
}

func (r *chatRepository) UpdateSessionActivity(ctx context.Context, userID, sessionID, title string, activityAt time.Time) error {
	updates := map[string]any{
		"last_message_at": activityAt,
		"updated_at":      activityAt,
	}
	if title != "" {
		updates["title"] = title
	}
	if err := r.db.WithContext(ctx).
		Model(&models.ChatSession{}).
		Where("user_id = ? AND id = ?", userID, sessionID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update session activity: %w", err)
	}
	return nil
}

func (r *chatRepository) CountSessions(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.ChatSession{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return count, nil
}

func (r *chatRepository) CountMessages(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.ChatMessage{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func (r *chatRepository) SumTokens(ctx context.Context, userID string) (int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).
		Model(&models.ChatMessage{}).
		Where("user_id = ?", userID).
		Select("coalesce(sum(total_tokens), 0)").
		Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("sum tokens: %w", err)
	}
	return total, nil
}

func (r *chatRepository) DailyTokenUsage(ctx context.Context, userID string, days int) ([]DailyUsageRow, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days+1)
	rows := make([]DailyUsageRow, 0)
	query := `
		SELECT date(created_at) AS day, coalesce(sum(total_tokens), 0) AS tokens
		FROM chat_messages
		WHERE user_id = ? AND created_at >= ?
		GROUP BY date(created_at)
		ORDER BY day ASC
	`
	if err := r.db.WithContext(ctx).Raw(query, userID, since).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("daily token usage: %w", err)
	}
	return rows, nil
}

func (r *chatRepository) DeleteSession(ctx context.Context, sessionID string) error {
	result := r.db.WithContext(ctx).Delete(&models.ChatSession{}, "id = ?", sessionID)
	if result.Error != nil {
		return fmt.Errorf("delete session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

func (r *chatRepository) DeleteMessagesBySession(ctx context.Context, sessionID string) error {
	result := r.db.WithContext(ctx).Delete(&models.ChatMessage{}, "session_id = ?", sessionID)
	if result.Error != nil {
		return fmt.Errorf("delete messages by session: %w", result.Error)
	}
	return nil
}
