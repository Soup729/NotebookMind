package repository

import (
	"context"
	"fmt"
	"time"

	"enterprise-pdf-ai/internal/models"
	"gorm.io/gorm"
)

// NotebookRepository defines operations for Notebook entities
type NotebookRepository interface {
	Create(ctx context.Context, notebook *models.Notebook) error
	GetByID(ctx context.Context, userID, notebookID string) (*models.Notebook, error)
	ListByUser(ctx context.Context, userID string) ([]models.Notebook, error)
	Update(ctx context.Context, notebook *models.Notebook) error
	Delete(ctx context.Context, userID, notebookID string) error

	// Document associations
	AddDocument(ctx context.Context, notebookID, documentID string) error
	RemoveDocument(ctx context.Context, notebookID, documentID string) error
	ListDocuments(ctx context.Context, notebookID string) ([]models.Document, error)
	GetDocumentIDs(ctx context.Context, notebookID string) ([]string, error)

	// DocumentGuide operations
	UpsertGuide(ctx context.Context, guide *models.DocumentGuide) error
	GetGuide(ctx context.Context, documentID string) (*models.DocumentGuide, error)
}

// notebookRepository implements NotebookRepository
type notebookRepository struct {
	db *gorm.DB
}

// NewNotebookRepository creates a new NotebookRepository instance
func NewNotebookRepository(db *gorm.DB) (NotebookRepository, error) {
	if err := db.AutoMigrate(&models.Notebook{}, &models.NotebookDocument{}, &models.DocumentGuide{}); err != nil {
		return nil, fmt.Errorf("auto migrate notebooks: %w", err)
	}
	return &notebookRepository{db: db}, nil
}

func (r *notebookRepository) Create(ctx context.Context, notebook *models.Notebook) error {
	if err := r.db.WithContext(ctx).Create(notebook).Error; err != nil {
		return fmt.Errorf("create notebook: %w", err)
	}
	return nil
}

func (r *notebookRepository) GetByID(ctx context.Context, userID, notebookID string) (*models.Notebook, error) {
	var notebook models.Notebook
	if err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", notebookID, userID).
		First(&notebook).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get notebook: %w", err)
	}
	return &notebook, nil
}

func (r *notebookRepository) ListByUser(ctx context.Context, userID string) ([]models.Notebook, error) {
	var notebooks []models.Notebook
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at desc").
		Find(&notebooks).Error; err != nil {
		return nil, fmt.Errorf("list notebooks: %w", err)
	}
	return notebooks, nil
}

func (r *notebookRepository) Update(ctx context.Context, notebook *models.Notebook) error {
	notebook.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(notebook).Error; err != nil {
		return fmt.Errorf("update notebook: %w", err)
	}
	return nil
}

func (r *notebookRepository) Delete(ctx context.Context, userID, notebookID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete document associations
		if err := tx.Where("notebook_id = ?", notebookID).Delete(&models.NotebookDocument{}).Error; err != nil {
			return fmt.Errorf("delete notebook documents: %w", err)
		}

		// Update document notebook_id to null
		if err := tx.Model(&models.Document{}).Where("notebook_id = ?", notebookID).Update("notebook_id", nil).Error; err != nil {
			return fmt.Errorf("unlink documents: %w", err)
		}

		// Delete the notebook
		result := tx.Where("id = ? AND user_id = ?", notebookID, userID).Delete(&models.Notebook{})
		if result.Error != nil {
			return fmt.Errorf("delete notebook: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (r *notebookRepository) AddDocument(ctx context.Context, notebookID, documentID string) error {
	assoc := &models.NotebookDocument{
		ID:         generateUUID(),
		NotebookID: notebookID,
		DocumentID: documentID,
		AddedAt:    time.Now(),
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if already exists
		var count int64
		if err := tx.Model(&models.NotebookDocument{}).
			Where("notebook_id = ? AND document_id = ?", notebookID, documentID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check existing association: %w", err)
		}
		if count > 0 {
			return nil // Already exists
		}

		if err := tx.Create(assoc).Error; err != nil {
			return fmt.Errorf("create notebook document: %w", err)
		}

		// Update notebook document count
		if err := tx.Model(&models.Notebook{}).Where("id = ?", notebookID).
			Update("document_cnt", gorm.Expr("document_cnt + 1")).Error; err != nil {
			return fmt.Errorf("update notebook count: %w", err)
		}

		// Update document's notebook_id
		if err := tx.Model(&models.Document{}).Where("id = ?", documentID).
			Update("notebook_id", notebookID).Error; err != nil {
			return fmt.Errorf("update document notebook: %w", err)
		}

		return nil
	})
}

func (r *notebookRepository) RemoveDocument(ctx context.Context, notebookID, documentID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("notebook_id = ? AND document_id = ?", notebookID, documentID).
			Delete(&models.NotebookDocument{})
		if result.Error != nil {
			return fmt.Errorf("delete notebook document: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}

		// Update notebook document count
		if err := tx.Model(&models.Notebook{}).Where("id = ?", notebookID).
			Update("document_cnt", gorm.Expr("document_cnt - 1")).Error; err != nil {
			return fmt.Errorf("update notebook count: %w", err)
		}

		// Clear document's notebook_id
		if err := tx.Model(&models.Document{}).Where("id = ? AND notebook_id = ?", documentID, notebookID).
			Update("notebook_id", nil).Error; err != nil {
			return fmt.Errorf("clear document notebook: %w", err)
		}

		return nil
	})
}

func (r *notebookRepository) ListDocuments(ctx context.Context, notebookID string) ([]models.Document, error) {
	var documents []models.Document
	if err := r.db.WithContext(ctx).
		Joins("JOIN notebook_documents ON notebook_documents.document_id = documents.id").
		Where("notebook_documents.notebook_id = ?", notebookID).
		Order("notebook_documents.added_at desc").
		Find(&documents).Error; err != nil {
		return nil, fmt.Errorf("list notebook documents: %w", err)
	}
	return documents, nil
}

func (r *notebookRepository) GetDocumentIDs(ctx context.Context, notebookID string) ([]string, error) {
	var docIDs []string
	if err := r.db.WithContext(ctx).
		Model(&models.NotebookDocument{}).
		Where("notebook_id = ?", notebookID).
		Pluck("document_id", &docIDs).Error; err != nil {
		return nil, fmt.Errorf("get notebook document ids: %w", err)
	}
	return docIDs, nil
}

func (r *notebookRepository) UpsertGuide(ctx context.Context, guide *models.DocumentGuide) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.DocumentGuide
		err := tx.Where("document_id = ?", guide.DocumentID).First(&existing).Error
		if err == nil {
			// Update existing
			guide.ID = existing.ID
			guide.CreatedAt = existing.CreatedAt
			guide.UpdatedAt = time.Now()
			if err := tx.Save(guide).Error; err != nil {
				return fmt.Errorf("update document guide: %w", err)
			}
		} else if err == gorm.ErrRecordNotFound {
			// Create new
			now := time.Now()
			guide.GeneratedAt = &now
			guide.CreatedAt = now
			guide.UpdatedAt = now
			if err := tx.Create(guide).Error; err != nil {
				return fmt.Errorf("create document guide: %w", err)
			}
		} else {
			return fmt.Errorf("check existing guide: %w", err)
		}

		// Update document status
		if err := tx.Model(&models.Document{}).Where("id = ?", guide.DocumentID).
			Updates(map[string]any{
				"guide_status": guide.Status,
				"guide_error":  guide.ErrorMsg,
				"summary":      guide.Summary,
				"faq_json":     guide.FaqJSON,
			}).Error; err != nil {
			return fmt.Errorf("update document guide status: %w", err)
		}

		return nil
	})
}

func (r *notebookRepository) GetGuide(ctx context.Context, documentID string) (*models.DocumentGuide, error) {
	var guide models.DocumentGuide
	if err := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		First(&guide).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get document guide: %w", err)
	}
	return &guide, nil
}

func generateUUID() string {
	// Using google/uuid would be better, simplified here
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
