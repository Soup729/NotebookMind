package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"enterprise-pdf-ai/internal/configs"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"go.uber.org/zap"
)

const (
	// NotebookCollectionName is the Milvus collection for NotebookLM chunks
	NotebookCollectionName = "notebook_chunks"
	// NotebookVectorDim is the dimension for embedding vectors (text-embedding-3-small)
	NotebookVectorDim = 1536
)

// NotebookChunk represents a chunk stored in Milvus for NotebookLM
type NotebookChunk struct {
	ID          int64   `json:"id"`
	NotebookID  string  `json:"notebook_id"`
	DocumentID  string  `json:"document_id"`
	PageNumber  int64   `json:"page_number"`
	ChunkIndex  int64   `json:"chunk_index"`
	Content     string  `json:"content"`
	Vector      []float32 `json:"vector"`
}

// NotebookVectorStore defines the interface for notebook vector operations
type NotebookVectorStore interface {
	// InsertChunks inserts document chunks into Milvus
	InsertChunks(ctx context.Context, chunks []NotebookChunk, vectors [][]float32) error
	// Search searches for similar chunks within a notebook
	Search(ctx context.Context, queryVector []float32, topK int, notebookID string, docIDs []string) ([]NotebookChunk, []float32, error)
	// DeleteByDocument removes all chunks for a document
	DeleteByDocument(ctx context.Context, documentID string) error
	// DeleteByNotebook removes all chunks for a notebook
	DeleteByNotebook(ctx context.Context, notebookID string) error
}

// notebookMilvusRepo implements NotebookVectorStore using Milvus
type notebookMilvusRepo struct {
	cli       client.Client
	colName   string
	dimension int
}

// NewNotebookMilvusStore creates a new NotebookVectorStore with Milvus
func NewNotebookMilvusStore(ctx context.Context, cfg *configs.MilvusConfig) (NotebookVectorStore, error) {
	if strings.TrimSpace(cfg.Address) == "" {
		zap.L().Warn("Milvus address not configured, notebook vector store unavailable")
		return nil, fmt.Errorf("Milvus address is required")
	}

	apiKey := strings.TrimSpace(cfg.Password)
	if username := strings.TrimSpace(cfg.Username); username != "" && username != "db_admin" {
		apiKey = username + ":" + apiKey
	}

	cli, err := client.NewClient(ctx, client.Config{
		Address: cfg.Address,
		APIKey:  apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create Milvus client: %w", err)
	}

	collectionName := NotebookCollectionName
	if cfg.CollectionName != "" {
		collectionName = cfg.CollectionName + "_notebook"
	}

	// Ensure collection exists with compatible schema
	if err := ensureNotebookCollection(ctx, cli, collectionName, NotebookVectorDim); err != nil {
		return nil, fmt.Errorf("ensure notebook collection: %w", err)
	}

	if err := cli.LoadCollection(ctx, collectionName, false); err != nil {
		return nil, fmt.Errorf("load notebook collection: %w", err)
	}

	zap.L().Info("Notebook Milvus vector store ready",
		zap.String("collection", collectionName),
		zap.Int("dimension", NotebookVectorDim),
	)

	return &notebookMilvusRepo{
		cli:       cli,
		colName:   collectionName,
		dimension: NotebookVectorDim,
	}, nil
}

// InsertChunks inserts document chunks into Milvus
func (m *notebookMilvusRepo) InsertChunks(ctx context.Context, chunks []NotebookChunk, vectors [][]float32) error {
	if len(chunks) == 0 {
		return nil
	}

	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunks count (%d) != vectors count (%d)", len(chunks), len(vectors))
	}

	// Prepare column data
	notebookIDs := make([]string, len(chunks))
	documentIDs := make([]string, len(chunks))
	pageNumbers := make([]int64, len(chunks))
	chunkIndexes := make([]int64, len(chunks))
	contents := make([]string, len(chunks))

	for i, chunk := range chunks {
		notebookIDs[i] = chunk.NotebookID
		documentIDs[i] = chunk.DocumentID
		pageNumbers[i] = chunk.PageNumber
		chunkIndexes[i] = chunk.ChunkIndex
		contents[i] = chunk.Content
	}

	_, err := m.cli.Insert(
		ctx,
		m.colName,
		"",
		entity.NewColumnVarChar("notebook_id", notebookIDs),
		entity.NewColumnVarChar("document_id", documentIDs),
		entity.NewColumnInt64("page_number", pageNumbers),
		entity.NewColumnInt64("chunk_index", chunkIndexes),
		entity.NewColumnVarChar("content", contents),
		entity.NewColumnFloatVector("vector", m.dimension, vectors),
	)
	if err != nil {
		return fmt.Errorf("insert notebook chunks into Milvus: %w", err)
	}

	zap.L().Debug("inserted notebook chunks",
		zap.Int("count", len(chunks)),
		zap.String("notebook_id", chunks[0].NotebookID),
	)

	return nil
}

// Search searches for similar chunks within a notebook
func (m *notebookMilvusRepo) Search(ctx context.Context, queryVector []float32, topK int, notebookID string, docIDs []string) ([]NotebookChunk, []float32, error) {
	if notebookID == "" && len(docIDs) == 0 {
		return nil, nil, fmt.Errorf("notebook_id or document_ids is required")
	}

	// Build filter expression
	expr := buildNotebookSearchExpr(notebookID, docIDs)

	searchParam, err := entity.NewIndexIvfFlatSearchParam(10)
	if err != nil {
		return nil, nil, fmt.Errorf("build Milvus search params: %w", err)
	}

	results, err := m.cli.Search(
		ctx,
		m.colName,
		nil,
		expr,
		[]string{"notebook_id", "document_id", "page_number", "chunk_index", "content"},
		[]entity.Vector{entity.FloatVector(queryVector)},
		"vector",
		entity.L2,
		topK,
		searchParam,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("search Milvus: %w", err)
	}

	var chunks []NotebookChunk
	var scores []float32

	for _, result := range results {
		notebookCol, _ := result.Fields.GetColumn("notebook_id").(*entity.ColumnVarChar)
		docCol, _ := result.Fields.GetColumn("document_id").(*entity.ColumnVarChar)
		pageCol, _ := result.Fields.GetColumn("page_number").(*entity.ColumnInt64)
		chunkIdxCol, _ := result.Fields.GetColumn("chunk_index").(*entity.ColumnInt64)
		contentCol, _ := result.Fields.GetColumn("content").(*entity.ColumnVarChar)

		for i := 0; i < result.ResultCount; i++ {
			notebookIDVal, _ := notebookCol.ValueByIdx(i)
			docIDVal, _ := docCol.ValueByIdx(i)
			pageNum, _ := pageCol.ValueByIdx(i)
			chunkIdx, _ := chunkIdxCol.ValueByIdx(i)
			content, _ := contentCol.ValueByIdx(i)

			chunks = append(chunks, NotebookChunk{
				NotebookID:  notebookIDVal,
				DocumentID:  docIDVal,
				PageNumber:  pageNum,
				ChunkIndex:  chunkIdx,
				Content:     content,
			})
			scores = append(scores, result.Scores[i])
		}
	}

	return chunks, scores, nil
}

// DeleteByDocument removes all chunks for a document
func (m *notebookMilvusRepo) DeleteByDocument(ctx context.Context, documentID string) error {
	expr := fmt.Sprintf(`document_id == "%s"`, documentID)
	if err := m.cli.Delete(ctx, m.colName, "", expr); err != nil {
		return fmt.Errorf("delete document chunks from Milvus: %w", err)
	}
	return nil
}

// DeleteByNotebook removes all chunks for a notebook
func (m *notebookMilvusRepo) DeleteByNotebook(ctx context.Context, notebookID string) error {
	expr := fmt.Sprintf(`notebook_id == "%s"`, notebookID)
	if err := m.cli.Delete(ctx, m.colName, "", expr); err != nil {
		return fmt.Errorf("delete notebook chunks from Milvus: %w", err)
	}
	return nil
}

func buildNotebookSearchExpr(notebookID string, docIDs []string) string {
	var parts []string

	if notebookID != "" {
		parts = append(parts, fmt.Sprintf(`notebook_id == "%s"`, notebookID))
	}

	if len(docIDs) > 0 {
		quoted := make([]string, 0, len(docIDs))
		for _, docID := range docIDs {
			docID = strings.TrimSpace(docID)
			if docID == "" {
				continue
			}
			quoted = append(quoted, fmt.Sprintf(`"%s"`, docID))
		}
		if len(quoted) > 0 {
			parts = append(parts, fmt.Sprintf("document_id in [%s]", strings.Join(quoted, ",")))
		}
	}

	return strings.Join(parts, " && ")
}

// ensureNotebookCollection ensures the notebook collection exists with correct schema
func ensureNotebookCollection(ctx context.Context, cli client.Client, collectionName string, dimension int) error {
	has, err := cli.HasCollection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("check collection existence: %w", err)
	}

	if !has {
		return createNotebookCollection(ctx, cli, collectionName, dimension)
	}

	// Verify schema compatibility
	description, err := cli.DescribeCollection(ctx, collectionName)
	if err != nil {
		return fmt.Errorf("describe collection: %w", err)
	}

	return validateNotebookSchema(description, dimension)
}

// createNotebookCollection creates a new collection with notebook-specific schema
func createNotebookCollection(ctx context.Context, cli client.Client, collectionName string, dimension int) error {
	schemaDef := entity.NewSchema().
		WithName(collectionName).
		WithDescription("NotebookLM document chunks with page numbers").
		WithAutoID(true).
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName("notebook_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("page_number").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dimension)))

	if err := cli.CreateCollection(ctx, schemaDef, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("create notebook collection %s: %w", collectionName, err)
	}

	// Create index on vector field
	index, err := entity.NewIndexIvfFlat(entity.L2, 1024)
	if err != nil {
		return fmt.Errorf("create notebook index definition: %w", err)
	}
	if err := cli.CreateIndex(ctx, collectionName, "vector", index, false); err != nil {
		return fmt.Errorf("create notebook vector index: %w", err)
	}

	zap.L().Info("created notebook collection", zap.String("name", collectionName), zap.Int("dim", dimension))
	return nil
}

// validateNotebookSchema validates that the collection has the expected schema
func validateNotebookSchema(collection *entity.Collection, expectedDimension int) error {
	if collection == nil || collection.Schema == nil {
		return fmt.Errorf("collection schema is empty")
	}

	requiredFields := map[string]entity.FieldType{
		"notebook_id":  entity.FieldTypeVarChar,
		"document_id":  entity.FieldTypeVarChar,
		"page_number":  entity.FieldTypeInt64,
		"chunk_index":  entity.FieldTypeInt64,
		"content":      entity.FieldTypeVarChar,
		"vector":       entity.FieldTypeFloatVector,
	}

	fieldsByName := make(map[string]*entity.Field)
	for _, field := range collection.Schema.Fields {
		fieldsByName[field.Name] = field
	}

	for fieldName, fieldType := range requiredFields {
		field, ok := fieldsByName[fieldName]
		if !ok {
			return fmt.Errorf("missing field %q", fieldName)
		}
		if field.DataType != fieldType {
			return fmt.Errorf("field %q has unexpected type %v", fieldName, field.DataType)
		}
	}

	// Validate vector dimension
	vectorField := fieldsByName["vector"]
	vectorDim := strings.TrimSpace(vectorField.TypeParams[entity.TypeParamDim])
	if vectorDim == "" {
		return fmt.Errorf("vector field dimension is missing")
	}
	parsedDim, err := strconv.Atoi(vectorDim)
	if err != nil {
		return fmt.Errorf("parse vector dimension: %w", err)
	}
	if parsedDim != expectedDimension {
		return fmt.Errorf("vector dimension mismatch: got %d want %d", parsedDim, expectedDimension)
	}

	return nil
}
