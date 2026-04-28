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
	UpdateSessionMemory(ctx context.Context, userID, sessionID, summary, memoryJSON string, messageCount int, updatedAt time.Time) error
	ClearSessionMemory(ctx context.Context, userID, sessionID string) error
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
	if err := ensureChatSessionSchema(db); err != nil {
		return nil, fmt.Errorf("ensure chat session schema: %w", err)
	}

	// 修复 session_id 列类型：从 bigint 改为 varchar(36)，以支持 UUID
	if err := fixChatMessageSessionIDType(db); err != nil {
		return nil, fmt.Errorf("fix chat_message session_id type: %w", err)
	}

	// 修复 id 列类型：从 bigint/bigserial 改为 varchar(36)，以支持 UUID 主键
	if err := fixChatMessageTypeIDColumn(db); err != nil {
		return nil, fmt.Errorf("fix chat_message id type: %w", err)
	}

	if err := db.AutoMigrate(&models.ChatMessage{}); err != nil {
		return nil, fmt.Errorf("auto migrate chat tables: %w", err)
	}

	return &chatRepository{db: db}, nil
}

func ensureChatSessionSchema(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.ChatSession{}) {
		return db.AutoMigrate(&models.ChatSession{})
	}

	statements := []string{
		`ALTER TABLE "chat_sessions" ADD COLUMN IF NOT EXISTS "notebook_id" varchar(36)`,
		`UPDATE "chat_sessions" SET "notebook_id" = '' WHERE "notebook_id" IS NULL`,
		`ALTER TABLE "chat_sessions" ALTER COLUMN "notebook_id" SET NOT NULL`,
		`ALTER TABLE "chat_sessions" ADD COLUMN IF NOT EXISTS "memory_summary" text`,
		`ALTER TABLE "chat_sessions" ADD COLUMN IF NOT EXISTS "memory_json" text`,
		`ALTER TABLE "chat_sessions" ADD COLUMN IF NOT EXISTS "memory_message_count" bigint NOT NULL DEFAULT 0`,
		`ALTER TABLE "chat_sessions" ADD COLUMN IF NOT EXISTS "memory_updated_at" timestamptz`,
		`CREATE INDEX IF NOT EXISTS "idx_chat_sessions_user_id" ON "chat_sessions" ("user_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_chat_sessions_notebook_id" ON "chat_sessions" ("notebook_id")`,
		`CREATE INDEX IF NOT EXISTS "idx_chat_sessions_last_message_at" ON "chat_sessions" ("last_message_at")`,
		`CREATE INDEX IF NOT EXISTS "idx_chat_sessions_created_at" ON "chat_sessions" ("created_at")`,
		`CREATE INDEX IF NOT EXISTS "idx_chat_sessions_memory_updated_at" ON "chat_sessions" ("memory_updated_at")`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
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
		Where("user_id = ? AND session_id = ?", userID, sessionID)
	if limit > 0 {
		query = query.Order("created_at desc").Limit(limit)
	} else {
		query = query.Order("created_at asc")
	}
	if err := query.Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("list session messages: %w", err)
	}
	if limit > 0 {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
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

func (r *chatRepository) UpdateSessionMemory(ctx context.Context, userID, sessionID, summary, memoryJSON string, messageCount int, updatedAt time.Time) error {
	updates := map[string]any{
		"memory_summary":       summary,
		"memory_json":          memoryJSON,
		"memory_message_count": messageCount,
		"memory_updated_at":    updatedAt,
		"updated_at":           updatedAt,
	}
	if err := r.db.WithContext(ctx).
		Model(&models.ChatSession{}).
		Where("user_id = ? AND id = ?", userID, sessionID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update session memory: %w", err)
	}
	return nil
}

func (r *chatRepository) ClearSessionMemory(ctx context.Context, userID, sessionID string) error {
	if err := r.db.WithContext(ctx).
		Model(&models.ChatSession{}).
		Where("user_id = ? AND id = ?", userID, sessionID).
		Updates(map[string]any{
			"memory_summary":       "",
			"memory_json":          "",
			"memory_message_count": 0,
			"memory_updated_at":    nil,
			"updated_at":           time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("clear session memory: %w", err)
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
