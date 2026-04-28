package repository

import (
	"context"
	"fmt"
	"time"

	"NotebookAI/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotebookGraphRepository interface {
	UpsertState(ctx context.Context, state *models.NotebookGraphState) error
	GetState(ctx context.Context, notebookID string) (*models.NotebookGraphState, error)
	ReplaceDocumentItems(ctx context.Context, notebookID, documentID string, items []models.NotebookGraphItem) error
	DeleteDocumentItems(ctx context.Context, documentID string) error
	DeleteNotebookItems(ctx context.Context, notebookID string) error
	ListNotebookItems(ctx context.Context, notebookID string) ([]models.NotebookGraphItem, error)
}

type notebookGraphRepository struct {
	db *gorm.DB
}

func NewNotebookGraphRepository(db *gorm.DB) (NotebookGraphRepository, error) {
	if err := db.AutoMigrate(&models.NotebookGraphState{}, &models.NotebookGraphItem{}); err != nil {
		return nil, fmt.Errorf("auto migrate notebook graph: %w", err)
	}
	return &notebookGraphRepository{db: db}, nil
}

func (r *notebookGraphRepository) UpsertState(ctx context.Context, state *models.NotebookGraphState) error {
	if state == nil {
		return nil
	}
	state.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "notebook_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"status",
				"semantic_index_status",
				"version",
				"entity_count",
				"relation_count",
				"last_error",
				"semantic_index_error",
				"updated_at",
			}),
		}).
		Create(state).Error; err != nil {
		return fmt.Errorf("upsert notebook graph state: %w", err)
	}
	return nil
}

func (r *notebookGraphRepository) GetState(ctx context.Context, notebookID string) (*models.NotebookGraphState, error) {
	var state models.NotebookGraphState
	if err := r.db.WithContext(ctx).Where("notebook_id = ?", notebookID).First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get notebook graph state: %w", err)
	}
	return &state, nil
}

func (r *notebookGraphRepository) ReplaceDocumentItems(ctx context.Context, notebookID, documentID string, items []models.NotebookGraphItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("notebook_id = ? AND document_id = ?", notebookID, documentID).
			Delete(&models.NotebookGraphItem{}).Error; err != nil {
			return fmt.Errorf("delete existing notebook graph items: %w", err)
		}
		if len(items) == 0 {
			return nil
		}
		now := time.Now()
		for i := range items {
			if items[i].CreatedAt.IsZero() {
				items[i].CreatedAt = now
			}
			items[i].UpdatedAt = now
		}
		if err := tx.CreateInBatches(items, 100).Error; err != nil {
			return fmt.Errorf("insert notebook graph items: %w", err)
		}
		return nil
	})
}

func (r *notebookGraphRepository) DeleteDocumentItems(ctx context.Context, documentID string) error {
	if err := r.db.WithContext(ctx).Where("document_id = ?", documentID).Delete(&models.NotebookGraphItem{}).Error; err != nil {
		return fmt.Errorf("delete notebook graph document items: %w", err)
	}
	return nil
}

func (r *notebookGraphRepository) DeleteNotebookItems(ctx context.Context, notebookID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("notebook_id = ?", notebookID).Delete(&models.NotebookGraphItem{}).Error; err != nil {
			return fmt.Errorf("delete notebook graph items: %w", err)
		}
		if err := tx.Where("notebook_id = ?", notebookID).Delete(&models.NotebookGraphState{}).Error; err != nil {
			return fmt.Errorf("delete notebook graph state: %w", err)
		}
		return nil
	})
}

func (r *notebookGraphRepository) ListNotebookItems(ctx context.Context, notebookID string) ([]models.NotebookGraphItem, error) {
	var items []models.NotebookGraphItem
	if err := r.db.WithContext(ctx).
		Where("notebook_id = ?", notebookID).
		Order("item_type asc, weight desc, confidence desc, created_at asc").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list notebook graph items: %w", err)
	}
	return items, nil
}
