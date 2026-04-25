package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"NotebookAI/internal/configs"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"go.uber.org/zap"
)

const (
	// NotebookCollectionName is the Milvus collection for NotebookLM chunks
	NotebookCollectionName = "notebook_chunks"
)

// NotebookChunk represents a chunk stored in Milvus for NotebookLM
type NotebookChunk struct {
	ID          int64       `json:"id"`
	NotebookID  string      `json:"notebook_id"`
	DocumentID  string      `json:"document_id"`
	PageNumber  int64       `json:"page_number"`
	ChunkIndex  int64       `json:"chunk_index"`
	Content     string      `json:"content"`
	ChunkType   string      `json:"chunk_type,omitempty"`
	ChunkRole   string      `json:"chunk_role,omitempty"`
	ParentID    string      `json:"parent_id,omitempty"`
	SectionPath string      `json:"section_path,omitempty"` // JSON array
	BBox        string      `json:"bbox,omitempty"`         // JSON array
	Vector      []float32   `json:"vector"`
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
	// GetAllChunks retrieves all chunks (used for BM25 index warmup)
	GetAllChunks(ctx context.Context) ([]NotebookChunk, error)
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
	if err := ensureNotebookCollection(ctx, cli, collectionName, cfg.Dimension); err != nil {
		return nil, fmt.Errorf("ensure notebook collection: %w", err)
	}

	if err := cli.LoadCollection(ctx, collectionName, false); err != nil {
		return nil, fmt.Errorf("load notebook collection: %w", err)
	}

	zap.L().Info("Notebook Milvus vector store ready",
		zap.String("collection", collectionName),
		zap.Int("dimension", cfg.Dimension),
	)

	return &notebookMilvusRepo{
		cli:       cli,
		colName:   collectionName,
		dimension: cfg.Dimension,
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
	chunkTypes := make([]string, len(chunks))
	chunkRoles := make([]string, len(chunks))
	parentIDs := make([]string, len(chunks))
	sectionPaths := make([]string, len(chunks))
	bboxes := make([]string, len(chunks))

	for i, chunk := range chunks {
		notebookIDs[i] = chunk.NotebookID
		documentIDs[i] = chunk.DocumentID
		pageNumbers[i] = chunk.PageNumber
		chunkIndexes[i] = chunk.ChunkIndex
		contents[i] = chunk.Content
		chunkTypes[i] = chunk.ChunkType
		chunkRoles[i] = chunk.ChunkRole
		parentIDs[i] = chunk.ParentID
		sectionPaths[i] = chunk.SectionPath
		bboxes[i] = chunk.BBox
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
		entity.NewColumnVarChar("chunk_type", chunkTypes),
		entity.NewColumnVarChar("chunk_role", chunkRoles),
		entity.NewColumnVarChar("parent_id", parentIDs),
		entity.NewColumnVarChar("section_path", sectionPaths),
		entity.NewColumnVarChar("bbox", bboxes),
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

	outputFields := []string{
		"notebook_id", "document_id", "page_number", "chunk_index", "content",
		"chunk_type", "chunk_role", "parent_id", "section_path", "bbox",
	}

	results, err := m.cli.Search(
		ctx,
		m.colName,
		nil,
		expr,
		outputFields,
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
		chunkTypeCol, _ := result.Fields.GetColumn("chunk_type").(*entity.ColumnVarChar)
		chunkRoleCol, _ := result.Fields.GetColumn("chunk_role").(*entity.ColumnVarChar)
		parentIDCol, _ := result.Fields.GetColumn("parent_id").(*entity.ColumnVarChar)
		sectionPathCol, _ := result.Fields.GetColumn("section_path").(*entity.ColumnVarChar)
		bboxCol, _ := result.Fields.GetColumn("bbox").(*entity.ColumnVarChar)

		for i := 0; i < result.ResultCount; i++ {
			notebookIDVal, _ := notebookCol.ValueByIdx(i)
			docIDVal, _ := docCol.ValueByIdx(i)
			pageNum, _ := pageCol.ValueByIdx(i)
			chunkIdx, _ := chunkIdxCol.ValueByIdx(i)
			content, _ := contentCol.ValueByIdx(i)
			chunkType, _ := chunkTypeCol.ValueByIdx(i)
			chunkRole, _ := chunkRoleCol.ValueByIdx(i)
			parentID, _ := parentIDCol.ValueByIdx(i)
			sectionPath, _ := sectionPathCol.ValueByIdx(i)
			bbox, _ := bboxCol.ValueByIdx(i)

			chunks = append(chunks, NotebookChunk{
				NotebookID:  notebookIDVal,
				DocumentID:  docIDVal,
				PageNumber:  pageNum,
				ChunkIndex:  chunkIdx,
				Content:     content,
				ChunkType:   chunkType,
				ChunkRole:   chunkRole,
				ParentID:    parentID,
				SectionPath: sectionPath,
				BBox:        bbox,
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

// GetAllChunks retrieves all chunks for BM25 index warmup
func (m *notebookMilvusRepo) GetAllChunks(ctx context.Context) ([]NotebookChunk, error) {
	outputFields := []string{
		"document_id", "content", "chunk_index",
	}

	// Query all records
	resultSet, err := m.cli.Query(
		ctx,
		m.colName,
		nil,
		"id > 0", // match all records
		outputFields,
	)
	if err != nil {
		return nil, fmt.Errorf("query all chunks from Milvus: %w", err)
	}

	if resultSet.Len() == 0 {
		return nil, nil
	}

	docCol, ok := resultSet.GetColumn("document_id").(*entity.ColumnVarChar)
	if !ok || docCol == nil {
		return nil, nil
	}
	contentCol, ok := resultSet.GetColumn("content").(*entity.ColumnVarChar)
	if !ok || contentCol == nil {
		return nil, nil
	}
	chunkIdxCol, _ := resultSet.GetColumn("chunk_index").(*entity.ColumnInt64)

	chunks := make([]NotebookChunk, 0, docCol.Len())
	for i := 0; i < docCol.Len(); i++ {
		docID, _ := docCol.ValueByIdx(i)
		content, _ := contentCol.ValueByIdx(i)
		var chunkIdx int64
		if chunkIdxCol != nil {
			chunkIdx, _ = chunkIdxCol.ValueByIdx(i)
		}

		chunks = append(chunks, NotebookChunk{
			DocumentID: docID,
			Content:    content,
			ChunkIndex: chunkIdx,
		})
	}

	return chunks, nil
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

	if err := validateNotebookSchema(description, dimension); err != nil {
		// Schema 不兼容，删除旧 collection 重建
		zap.L().Warn("notebook collection schema is incompatible, dropping and recreating",
			zap.String("collection", collectionName),
			zap.Error(err),
		)
		if dropErr := cli.DropCollection(ctx, collectionName); dropErr != nil {
			return fmt.Errorf("drop incompatible notebook collection %s: %w", collectionName, dropErr)
		}
		return createNotebookCollection(ctx, cli, collectionName, dimension)
	}

	return nil
}

// createNotebookCollection creates a new collection with notebook-specific schema
func createNotebookCollection(ctx context.Context, cli client.Client, collectionName string, dimension int) error {
	schemaDef := entity.NewSchema().
		WithName(collectionName).
		WithDescription("NotebookLM document chunks with structured metadata").
		WithAutoID(true).
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName("notebook_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("page_number").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("chunk_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
		WithField(entity.NewField().WithName("chunk_role").WithDataType(entity.FieldTypeVarChar).WithMaxLength(16)).
		WithField(entity.NewField().WithName("parent_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("section_path").WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024)).
		WithField(entity.NewField().WithName("bbox").WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
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
		"notebook_id":   entity.FieldTypeVarChar,
		"document_id":   entity.FieldTypeVarChar,
		"page_number":   entity.FieldTypeInt64,
		"chunk_index":   entity.FieldTypeInt64,
		"content":       entity.FieldTypeVarChar,
		"chunk_type":    entity.FieldTypeVarChar,
		"chunk_role":    entity.FieldTypeVarChar,
		"parent_id":     entity.FieldTypeVarChar,
		"section_path":  entity.FieldTypeVarChar,
		"bbox":          entity.FieldTypeVarChar,
		"vector":        entity.FieldTypeFloatVector,
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
