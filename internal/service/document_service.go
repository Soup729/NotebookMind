package service

import (
	"context"
	"fmt"
	"os"

	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
)

type DocumentService interface {
	Create(ctx context.Context, document *models.Document) error
	Get(ctx context.Context, userID, documentID string) (*models.Document, error)
	List(ctx context.Context, userID string) ([]models.Document, error)
	Delete(ctx context.Context, userID, documentID string) error
}

type documentService struct {
	documents repository.DocumentRepository
	vectors   LLMService
}

func NewDocumentService(documents repository.DocumentRepository, vectors LLMService) DocumentService {
	return &documentService{
		documents: documents,
		vectors:   vectors,
	}
}

func (s *documentService) Create(ctx context.Context, document *models.Document) error {
	if err := s.documents.Create(ctx, document); err != nil {
		return fmt.Errorf("create document: %w", err)
	}
	return nil
}

func (s *documentService) Get(ctx context.Context, userID, documentID string) (*models.Document, error) {
	document, err := s.documents.GetByID(ctx, userID, documentID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	return document, nil
}

func (s *documentService) List(ctx context.Context, userID string) ([]models.Document, error) {
	documents, err := s.documents.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return documents, nil
}

func (s *documentService) Delete(ctx context.Context, userID, documentID string) error {
	document, err := s.documents.GetByID(ctx, userID, documentID)
	if err != nil {
		return fmt.Errorf("load document before delete: %w", err)
	}

	if err := s.vectors.DeleteDocumentChunks(ctx, userID, documentID); err != nil {
		return fmt.Errorf("delete vector chunks: %w", err)
	}
	if err := s.documents.DeleteByID(ctx, userID, documentID); err != nil {
		return fmt.Errorf("delete document metadata: %w", err)
	}
	if document.StoredPath != "" {
		if err := os.Remove(document.StoredPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove document file: %w", err)
		}
	}
	return nil
}
