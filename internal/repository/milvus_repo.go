package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"enterprise-pdf-ai/internal/configs"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

type VectorStore interface {
	AddDocuments(ctx context.Context, docs []schema.Document) error
	SimilaritySearch(ctx context.Context, query string, k int, opts VectorSearchOptions) ([]schema.Document, error)
	DeleteDocuments(ctx context.Context, opts VectorSearchOptions) error
}

type milvusRepo struct {
	cli       client.Client
	embedder  embeddings.Embedder
	colName   string
	dimension int
}

var milvusRequiredFields = map[string]entity.FieldType{
	"id":          entity.FieldTypeInt64,
	"user_id":     entity.FieldTypeVarChar,
	"document_id": entity.FieldTypeVarChar,
	"file_name":   entity.FieldTypeVarChar,
	"chunk_index": entity.FieldTypeInt64,
	"text":        entity.FieldTypeVarChar,
	"vector":      entity.FieldTypeFloatVector,
}

func NewMilvusStore(ctx context.Context, cfg *configs.MilvusConfig, embedder embeddings.Embedder) (VectorStore, error) {
	zap.L().Info("initializing Milvus vector store",
		zap.String("address", cfg.Address),
		zap.String("collection", cfg.CollectionName),
	)

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

	collectionName, err := ensureCompatibleMilvusCollection(ctx, cli, cfg)
	if err != nil {
		return nil, err
	}

	if err := cli.LoadCollection(ctx, collectionName, false); err != nil {
		return nil, fmt.Errorf("load collection: %w", err)
	}

	zap.L().Info("Milvus vector store ready",
		zap.String("collection", collectionName),
		zap.Int("dimension", cfg.Dimension),
	)

	return &milvusRepo{
		cli:       cli,
		embedder:  embedder,
		colName:   collectionName,
		dimension: cfg.Dimension,
	}, nil
}

func (m *milvusRepo) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	texts := make([]string, 0, len(docs))
	userIDs := make([]string, 0, len(docs))
	documentIDs := make([]string, 0, len(docs))
	fileNames := make([]string, 0, len(docs))
	chunkIndexes := make([]int64, 0, len(docs))

	for _, doc := range docs {
		text := strings.TrimSpace(doc.PageContent)
		if text == "" {
			continue
		}
		userID := metadataString(doc.Metadata, "user_id")
		documentID := metadataString(doc.Metadata, "document_id")
		if userID == "" || documentID == "" {
			return fmt.Errorf("document metadata missing user_id or document_id")
		}

		texts = append(texts, text)
		userIDs = append(userIDs, userID)
		documentIDs = append(documentIDs, documentID)
		fileNames = append(fileNames, metadataString(doc.Metadata, "file_name"))
		chunkIndexes = append(chunkIndexes, int64(metadataInt(doc.Metadata, "chunk_index")))
	}
	if len(texts) == 0 {
		return nil
	}

	vectors, err := m.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed documents: %w", err)
	}

	_, err = m.cli.Insert(
		ctx,
		m.colName,
		"",
		entity.NewColumnVarChar("user_id", userIDs),
		entity.NewColumnVarChar("document_id", documentIDs),
		entity.NewColumnVarChar("file_name", fileNames),
		entity.NewColumnInt64("chunk_index", chunkIndexes),
		entity.NewColumnVarChar("text", texts),
		entity.NewColumnFloatVector("vector", m.dimension, vectors),
	)
	if err != nil {
		return fmt.Errorf("insert documents into Milvus: %w", err)
	}
	return nil
}

func (m *milvusRepo) SimilaritySearch(ctx context.Context, query string, k int, opts VectorSearchOptions) ([]schema.Document, error) {
	if strings.TrimSpace(opts.UserID) == "" {
		return nil, fmt.Errorf("user id is required for vector search")
	}

	vector, err := m.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	searchParam, err := entity.NewIndexIvfFlatSearchParam(10)
	if err != nil {
		return nil, fmt.Errorf("build Milvus search params: %w", err)
	}
	results, err := m.cli.Search(
		ctx,
		m.colName,
		nil,
		buildMilvusExpr(opts),
		[]string{"text", "user_id", "document_id", "file_name", "chunk_index"},
		[]entity.Vector{entity.FloatVector(vector)},
		"vector",
		entity.L2,
		k,
		searchParam,
	)
	if err != nil {
		return nil, fmt.Errorf("search Milvus: %w", err)
	}

	docs := make([]schema.Document, 0)
	for _, result := range results {
		textColumn, ok := result.Fields.GetColumn("text").(*entity.ColumnVarChar)
		if !ok {
			continue
		}
		userColumn, _ := result.Fields.GetColumn("user_id").(*entity.ColumnVarChar)
		documentColumn, _ := result.Fields.GetColumn("document_id").(*entity.ColumnVarChar)
		fileColumn, _ := result.Fields.GetColumn("file_name").(*entity.ColumnVarChar)
		chunkColumn, _ := result.Fields.GetColumn("chunk_index").(*entity.ColumnInt64)

		for i := 0; i < result.ResultCount; i++ {
			text, _ := textColumn.ValueByIdx(i)
			userID, _ := userColumn.ValueByIdx(i)
			documentID, _ := documentColumn.ValueByIdx(i)
			fileName, _ := fileColumn.ValueByIdx(i)
			chunkIndex, _ := chunkColumn.ValueByIdx(i)

			docs = append(docs, schema.Document{
				PageContent: text,
				Score:       result.Scores[i],
				Metadata: map[string]any{
					"user_id":     userID,
					"document_id": documentID,
					"file_name":   fileName,
					"chunk_index": chunkIndex,
				},
			})
		}
	}
	return docs, nil
}

func (m *milvusRepo) DeleteDocuments(ctx context.Context, opts VectorSearchOptions) error {
	expr := buildMilvusExpr(opts)
	if expr == "" {
		return fmt.Errorf("delete filter is empty")
	}
	if err := m.cli.Delete(ctx, m.colName, "", expr); err != nil {
		return fmt.Errorf("delete Milvus documents: %w", err)
	}
	return nil
}

func buildMilvusExpr(opts VectorSearchOptions) string {
	parts := make([]string, 0, 2)
	if userID := strings.TrimSpace(opts.UserID); userID != "" {
		parts = append(parts, fmt.Sprintf("user_id == %q", userID))
	}
	if len(opts.DocumentIDs) > 0 {
		quoted := make([]string, 0, len(opts.DocumentIDs))
		for _, documentID := range opts.DocumentIDs {
			documentID = strings.TrimSpace(documentID)
			if documentID == "" {
				continue
			}
			quoted = append(quoted, strconv.Quote(documentID))
		}
		if len(quoted) > 0 {
			parts = append(parts, fmt.Sprintf("document_id in [%s]", strings.Join(quoted, ",")))
		}
	}
	return strings.Join(parts, " && ")
}

func ensureCompatibleMilvusCollection(ctx context.Context, cli client.Client, cfg *configs.MilvusConfig) (string, error) {
	baseName := strings.TrimSpace(cfg.CollectionName)
	if baseName == "" {
		return "", fmt.Errorf("Milvus collection name is required")
	}

	for version := 0; version < 10; version++ {
		candidate := baseName
		if version > 0 {
			candidate = fmt.Sprintf("%s_v%d", baseName, version+1)
		}

		has, err := cli.HasCollection(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("check collection %s: %w", candidate, err)
		}
		if !has {
			if err := createMilvusCollection(ctx, cli, candidate, cfg.Dimension); err != nil {
				return "", err
			}
			if version > 0 {
				zap.L().Warn("existing Milvus collection schema is incompatible; using a new compatible collection instead",
					zap.String("requested_collection", baseName),
					zap.String("effective_collection", candidate),
				)
			}
			return candidate, nil
		}

		description, err := cli.DescribeCollection(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("describe collection %s: %w", candidate, err)
		}
		if err := validateMilvusCollectionSchema(description, cfg.Dimension); err == nil {
			if version > 0 {
				zap.L().Warn("requested Milvus collection is incompatible; reusing an existing compatible fallback collection",
					zap.String("requested_collection", baseName),
					zap.String("effective_collection", candidate),
				)
			}
			return candidate, nil
		} else {
			zap.L().Warn("Milvus collection schema is incompatible with the current application",
				zap.String("collection", candidate),
				zap.Error(err),
			)
		}
	}

	return "", fmt.Errorf("unable to find or create a compatible Milvus collection for %s", baseName)
}

func createMilvusCollection(ctx context.Context, cli client.Client, collectionName string, dimension int) error {
	schemaDef := entity.NewSchema().
		WithName(collectionName).
		WithDescription("enterprise pdf chunks").
		WithAutoID(true).
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("file_name").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)).
		WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
		WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(int64(dimension)))

	if err := cli.CreateCollection(ctx, schemaDef, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("create collection %s: %w", collectionName, err)
	}

	index, err := entity.NewIndexIvfFlat(entity.L2, 1024)
	if err != nil {
		return fmt.Errorf("create Milvus index definition: %w", err)
	}
	if err := cli.CreateIndex(ctx, collectionName, "vector", index, false); err != nil {
		return fmt.Errorf("create collection index for %s: %w", collectionName, err)
	}
	return nil
}

func validateMilvusCollectionSchema(collection *entity.Collection, expectedDimension int) error {
	if collection == nil || collection.Schema == nil {
		return fmt.Errorf("collection schema is empty")
	}

	fieldsByName := make(map[string]*entity.Field, len(collection.Schema.Fields))
	for _, field := range collection.Schema.Fields {
		fieldsByName[field.Name] = field
	}

	for fieldName, fieldType := range milvusRequiredFields {
		field, ok := fieldsByName[fieldName]
		if !ok {
			return fmt.Errorf("missing field %q", fieldName)
		}
		if field.DataType != fieldType {
			return fmt.Errorf("field %q has unexpected type %v", fieldName, field.DataType)
		}
	}

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
