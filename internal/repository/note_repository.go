package repository

import (
	"context"
	"fmt"

	"NotebookAI/internal/models"

	"gorm.io/gorm"
)

// ============ Note Repository ============

// NoteRepository 笔记仓库接口
type NoteRepository interface {
	Create(ctx context.Context, note *models.Note) error
	Update(ctx context.Context, note *models.Note) error
	Delete(ctx context.Context, userID, noteID string) error
	GetByID(ctx context.Context, userID, noteID string) (*models.Note, error)
	List(ctx context.Context, userID string, filter *ListNotesFilter, page, pageSize int) ([]*models.Note, int, error)
}

// ListNotesFilter 笔记列表过滤器
type ListNotesFilter struct {
	NotebookID string
	SessionID  string
	Type       string
	Tag        string // LIKE 匹配
	PinnedOnly bool
}

// noteRepository 实现
type noteRepository struct {
	db *gorm.DB
}

// NewNoteRepository 创建笔记仓库
func NewNoteRepository(db *gorm.DB) (NoteRepository, error) {
	// 自动迁移
	if err := db.AutoMigrate(&models.Note{}); err != nil {
		return nil, fmt.Errorf("migrate note: %w", err)
	}

	return &noteRepository{db: db}, nil
}

// Create 创建笔记
func (r *noteRepository) Create(ctx context.Context, note *models.Note) error {
	return r.db.WithContext(ctx).Create(note).Error
}

// Update 更新笔记
func (r *noteRepository) Update(ctx context.Context, note *models.Note) error {
	return r.db.WithContext(ctx).Save(note).Error
}

// Delete 删除笔记
func (r *noteRepository) Delete(ctx context.Context, userID, noteID string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", noteID, userID).
		Delete(&models.Note{}).Error
}

// GetByID 获取笔记
func (r *noteRepository) GetByID(ctx context.Context, userID, noteID string) (*models.Note, error) {
	var note models.Note
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", noteID, userID).
		First(&note).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &note, nil
}

// List 列出笔记
func (r *noteRepository) List(ctx context.Context, userID string, filter *ListNotesFilter, page, pageSize int) ([]*models.Note, int, error) {
	var notes []*models.Note
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Note{}).Where("user_id = ?", userID)

	// 应用过滤器
	if filter != nil {
		if filter.NotebookID != "" {
			query = query.Where("notebook_id = ?", filter.NotebookID)
		}
		if filter.SessionID != "" {
			query = query.Where("session_id = ?", filter.SessionID)
		}
		if filter.Type != "" {
			query = query.Where("type = ?", filter.Type)
		}
		if filter.Tag != "" {
			query = query.Where("tags LIKE ?", "%"+filter.Tag+"%")
		}
		if filter.PinnedOnly {
			query = query.Where("is_pinned = ?", true)
		}
	}

	// 统计总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.
		Order("is_pinned DESC, updated_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&notes).Error
	if err != nil {
		return nil, 0, err
	}

	return notes, int(total), nil
}
