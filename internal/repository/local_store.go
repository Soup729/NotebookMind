package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"NotebookAI/internal/models"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"gorm.io/gorm"
)

type VectorSearchOptions struct {
	UserID      string
	DocumentIDs []string
}

type localVectorStore struct {
	db       *gorm.DB
	embedder embeddings.Embedder
}

func NewLocalStore(db *gorm.DB, embedder embeddings.Embedder) (VectorStore, error) {
	if err := db.AutoMigrate(&models.DocumentChunk{}); err != nil {
		return nil, fmt.Errorf("auto migrate document chunks: %w", err)
	}
	return &localVectorStore{
		db:       db,
		embedder: embedder,
	}, nil
}

func (s *localVectorStore) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	texts := make([]string, 0, len(docs))
	filteredDocs := make([]schema.Document, 0, len(docs))
	for _, doc := range docs {
		content := strings.TrimSpace(doc.PageContent)
		if content == "" {
			continue
		}
		doc.PageContent = content
		filteredDocs = append(filteredDocs, doc)
		texts = append(texts, content)
	}
	if len(filteredDocs) == 0 {
		return nil
	}

	vectors, err := s.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed documents for local store: %w", err)
	}

	chunks := make([]models.DocumentChunk, 0, len(filteredDocs))
	for i, doc := range filteredDocs {
		userID := metadataString(doc.Metadata, "user_id")
		documentID := metadataString(doc.Metadata, "document_id")
		fileName := metadataString(doc.Metadata, "file_name")
		if userID == "" || documentID == "" {
			return fmt.Errorf("document metadata missing user_id or document_id")
		}

		encoded, err := json.Marshal(vectors[i])
		if err != nil {
			return fmt.Errorf("marshal embedding: %w", err)
		}

		chunks = append(chunks, models.DocumentChunk{
			ID:            uuid.NewString(),
			UserID:        userID,
			DocumentID:    documentID,
			FileName:      fileName,
			ChunkIndex:    metadataInt(doc.Metadata, "chunk_index"),
			Content:       doc.PageContent,
			EmbeddingJSON: string(encoded),
		})
	}

	if err := s.db.WithContext(ctx).CreateInBatches(chunks, 100).Error; err != nil {
		return fmt.Errorf("persist document chunks: %w", err)
	}
	return nil
}

func (s *localVectorStore) SimilaritySearch(ctx context.Context, query string, k int, opts VectorSearchOptions) ([]schema.Document, error) {
	if strings.TrimSpace(opts.UserID) == "" {
		return nil, fmt.Errorf("user id is required for vector search")
	}

	queryVector, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query for local store: %w", err)
	}

	var chunks []models.DocumentChunk
	dbQuery := s.db.WithContext(ctx).Where("user_id = ?", opts.UserID)
	if len(opts.DocumentIDs) > 0 {
		dbQuery = dbQuery.Where("document_id IN ?", opts.DocumentIDs)
	}
	if err := dbQuery.Find(&chunks).Error; err != nil {
		return nil, fmt.Errorf("load document chunks: %w", err)
	}
	if len(chunks) == 0 {
		return []schema.Document{}, nil
	}

	type scoredChunk struct {
		chunk models.DocumentChunk
		score float32
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for _, chunk := range chunks {
		var embedding []float32
		if err := json.Unmarshal([]byte(chunk.EmbeddingJSON), &embedding); err != nil {
			return nil, fmt.Errorf("unmarshal chunk embedding: %w", err)
		}
		scored = append(scored, scoredChunk{
			chunk: chunk,
			score: cosineSimilarity(queryVector, embedding),
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if k > len(scored) {
		k = len(scored)
	}

	docs := make([]schema.Document, 0, k)
	for i := 0; i < k; i++ {
		chunk := scored[i].chunk
		docs = append(docs, schema.Document{
			PageContent: chunk.Content,
			Score:       scored[i].score,
			Metadata: map[string]any{
				"user_id":     chunk.UserID,
				"document_id": chunk.DocumentID,
				"file_name":   chunk.FileName,
				"chunk_index": chunk.ChunkIndex,
			},
		})
	}
	return docs, nil
}

func (s *localVectorStore) DeleteDocuments(ctx context.Context, opts VectorSearchOptions) error {
	if strings.TrimSpace(opts.UserID) == "" {
		return fmt.Errorf("user id is required for delete")
	}
	query := s.db.WithContext(ctx).Where("user_id = ?", opts.UserID)
	if len(opts.DocumentIDs) > 0 {
		query = query.Where("document_id IN ?", opts.DocumentIDs)
	}
	if err := query.Delete(&models.DocumentChunk{}).Error; err != nil {
		return fmt.Errorf("delete document chunks: %w", err)
	}
	return nil
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		normA += af * af
		normB += bf * bf
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
