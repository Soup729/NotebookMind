package service

import (
	"context"

	"NotebookAI/internal/models"
)

type GraphSemanticHit struct {
	ItemID      string  `json:"item_id"`
	CanonicalID string  `json:"canonical_id"`
	ItemType    string  `json:"item_type"`
	Score       float32 `json:"score"`
}

type KnowledgeGraphSemanticIndex interface {
	Status() string
	UpsertItems(ctx context.Context, items []models.NotebookGraphItem) error
	DeleteByDocument(ctx context.Context, documentID string) error
	DeleteByNotebook(ctx context.Context, notebookID string) error
	Search(ctx context.Context, notebookID string, query string, topK int) ([]GraphSemanticHit, error)
}

type noopKnowledgeGraphSemanticIndex struct{}

func NewNoopKnowledgeGraphSemanticIndex() KnowledgeGraphSemanticIndex {
	return noopKnowledgeGraphSemanticIndex{}
}

func (noopKnowledgeGraphSemanticIndex) Status() string {
	return models.GraphSemanticIndexDisabled
}

func (noopKnowledgeGraphSemanticIndex) UpsertItems(context.Context, []models.NotebookGraphItem) error {
	return nil
}

func (noopKnowledgeGraphSemanticIndex) DeleteByDocument(context.Context, string) error {
	return nil
}

func (noopKnowledgeGraphSemanticIndex) DeleteByNotebook(context.Context, string) error {
	return nil
}

func (noopKnowledgeGraphSemanticIndex) Search(context.Context, string, string, int) ([]GraphSemanticHit, error) {
	return nil, nil
}
