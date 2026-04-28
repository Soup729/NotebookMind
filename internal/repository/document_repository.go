package repository

import (
	"context"
	"fmt"
	"time"

	"NotebookAI/internal/models"
	"gorm.io/gorm"
)

type DocumentRepository interface {
	Create(ctx context.Context, document *models.Document) error
	GetByID(ctx context.Context, userID, documentID string) (*models.Document, error)
	GetByIDForWorker(ctx context.Context, documentID string) (*models.Document, error)
	// GetNamesByIDs 批量获取文档名称（用于减少 N+1 查询）
	GetNamesByIDs(ctx context.Context, userID string, docIDs []string) map[string]string
	ListByUser(ctx context.Context, userID string) ([]models.Document, error)
	UpdateProcessingResult(ctx context.Context, documentID string, status string, chunkCount int, errorMessage string) error
	DeleteByID(ctx context.Context, userID, documentID string) error
	CountByUser(ctx context.Context, userID string) (int64, error)
	CountCompletedByUser(ctx context.Context, userID string) (int64, error)
}

type documentRepository struct {
	db *gorm.DB
}

func NewDocumentRepository(db *gorm.DB) (DocumentRepository, error) {
	if err := db.AutoMigrate(&models.Document{}); err != nil {
		return nil, fmt.Errorf("auto migrate documents: %w", err)
	}
	return &documentRepository{db: db}, nil
}

func (r *documentRepository) Create(ctx context.Context, document *models.Document) error {
	if err := r.db.WithContext(ctx).Create(document).Error; err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	return nil
}

func (r *documentRepository) GetByID(ctx context.Context, userID, documentID string) (*models.Document, error) {
	var document models.Document
	if err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", documentID, userID).
		First(&document).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get document: %w", err)
	}
	return &document, nil
}

func (r *documentRepository) GetByIDForWorker(ctx context.Context, documentID string) (*models.Document, error) {
	var document models.Document
	if err := r.db.WithContext(ctx).Where("id = ?", documentID).First(&document).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get document for worker: %w", err)
	}
	return &document, nil
}

// GetNamesByIDs 批量获取文档 ID -> 文件名的映射（单次 SQL 查询，解决 N+1 问题）
func (r *documentRepository) GetNamesByIDs(ctx context.Context, userID string, docIDs []string) map[string]string {
	if len(docIDs) == 0 {
		return make(map[string]string)
	}

	var results []struct {
		ID       string
		FileName string
	}

	r.db.WithContext(ctx).
		Model(&models.Document{}).
		Where("user_id = ? AND id IN ?", userID, docIDs).
		Select("id, file_name").
		Find(&results)

	nameMap := make(map[string]string, len(results))
	for _, r := range results {
		nameMap[r.ID] = r.FileName
	}
	return nameMap
}

func (r *documentRepository) ListByUser(ctx context.Context, userID string) ([]models.Document, error) {
	var documents []models.Document
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&documents).Error; err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return documents, nil
}

func (r *documentRepository) UpdateProcessingResult(ctx context.Context, documentID string, status string, chunkCount int, errorMessage string) error {
	updates := map[string]any{
		"status":        status,
		"chunk_count":   chunkCount,
		"error_message": errorMessage,
		"updated_at":    time.Now(),
	}
	if status == models.DocumentStatusCompleted {
		now := time.Now()
		updates["processed_at"] = &now
	}
	if err := r.db.WithContext(ctx).
		Model(&models.Document{}).
		Where("id = ?", documentID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update document processing result: %w", err)
	}
	return nil
}

func (r *documentRepository) DeleteByID(ctx context.Context, userID, documentID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document models.Document
		if err := tx.Where("id = ? AND user_id = ?", documentID, userID).First(&document).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNotFound
			}
			return fmt.Errorf("load document: %w", err)
		}

		if err := tx.Where("document_id = ?", documentID).Delete(&models.DocumentGuide{}).Error; err != nil {
			return fmt.Errorf("delete document guide: %w", err)
		}

		var notebookIDs []string
		if err := tx.Model(&models.NotebookDocument{}).
			Where("document_id = ?", documentID).
			Pluck("notebook_id", &notebookIDs).Error; err != nil {
			return fmt.Errorf("list notebook links: %w", err)
		}

		if err := tx.Where("document_id = ?", documentID).Delete(&models.NotebookDocument{}).Error; err != nil {
			return fmt.Errorf("delete notebook document links: %w", err)
		}
		for _, notebookID := range notebookIDs {
			if err := tx.Model(&models.Notebook{}).Where("id = ? AND document_cnt > 0", notebookID).
				Update("document_cnt", gorm.Expr("document_cnt - 1")).Error; err != nil {
				return fmt.Errorf("update notebook document count: %w", err)
			}
		}

		result := tx.Where("id = ? AND user_id = ?", documentID, userID).Delete(&models.Document{})
		if result.Error != nil {
			return fmt.Errorf("delete document: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *documentRepository) CountByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Document{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count documents: %w", err)
	}
	return count, nil
}

func (r *documentRepository) CountCompletedByUser(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.Document{}).
		Where("user_id = ? AND status = ?", userID, models.DocumentStatusCompleted).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count completed documents: %w", err)
	}
	return count, nil
}
