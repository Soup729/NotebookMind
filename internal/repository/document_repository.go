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
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", documentID, userID).Delete(&models.Document{})
	if result.Error != nil {
		return fmt.Errorf("delete document: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
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
