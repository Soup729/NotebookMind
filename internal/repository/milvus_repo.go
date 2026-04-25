package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"NotebookAI/internal/configs"
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

// milvusRequiredFields 定义 v2 schema 必需的字段（含 Phase 1 扩展字段）
var milvusRequiredFields = map[string]entity.FieldType{
	"id":           entity.FieldTypeInt64,
	"user_id":      entity.FieldTypeVarChar,
	"document_id":  entity.FieldTypeVarChar,
	"file_name":    entity.FieldTypeVarChar,
	"chunk_index":  entity.FieldTypeInt64,
	"page_num":     entity.FieldTypeInt64,
	"chunk_type":   entity.FieldTypeVarChar,
	"chunk_role":   entity.FieldTypeVarChar,
	"parent_id":    entity.FieldTypeVarChar,
	"section_path": entity.FieldTypeVarChar,
	"bbox":         entity.FieldTypeVarChar,
	"text":         entity.FieldTypeVarChar,
	"vector":       entity.FieldTypeFloatVector,
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
	pageNums := make([]int64, 0, len(docs))
	chunkTypes := make([]string, 0, len(docs))
	chunkRoles := make([]string, 0, len(docs))
	parentIDs := make([]string, 0, len(docs))
	sectionPaths := make([]string, 0, len(docs))
	bboxes := make([]string, 0, len(docs))

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
		pageNums = append(pageNums, int64(metadataInt(doc.Metadata, "page_num")))
		chunkTypes = append(chunkTypes, metadataString(doc.Metadata, "chunk_type"))
		chunkRoles = append(chunkRoles, metadataString(doc.Metadata, "chunk_role"))
		parentIDs = append(parentIDs, metadataString(doc.Metadata, "parent_id"))

		// section_path: 序列化为 JSON 字符串
		sectionPathJSON := metadataSliceJSON(doc.Metadata, "section_path")
		sectionPaths = append(sectionPaths, sectionPathJSON)

		// bbox: 序列化为 JSON 字符串
		bboxJSON := metadataAnyJSON(doc.Metadata, "bbox")
		bboxes = append(bboxes, bboxJSON)
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
		entity.NewColumnInt64("page_num", pageNums),
		entity.NewColumnVarChar("chunk_type", chunkTypes),
		entity.NewColumnVarChar("chunk_role", chunkRoles),
		entity.NewColumnVarChar("parent_id", parentIDs),
		entity.NewColumnVarChar("section_path", sectionPaths),
		entity.NewColumnVarChar("bbox", bboxes),
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

	// 输出字段包含扩展字段
	outputFields := []string{
		"text", "user_id", "document_id", "file_name", "chunk_index",
		"page_num", "chunk_type", "chunk_role", "parent_id", "section_path", "bbox",
	}

	results, err := m.cli.Search(
		ctx,
		m.colName,
		nil,
		buildMilvusExpr(opts),
		outputFields,
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
		pageNumColumn, _ := result.Fields.GetColumn("page_num").(*entity.ColumnInt64)
		chunkTypeColumn, _ := result.Fields.GetColumn("chunk_type").(*entity.ColumnVarChar)
		chunkRoleColumn, _ := result.Fields.GetColumn("chunk_role").(*entity.ColumnVarChar)
		parentIDColumn, _ := result.Fields.GetColumn("parent_id").(*entity.ColumnVarChar)
		sectionPathColumn, _ := result.Fields.GetColumn("section_path").(*entity.ColumnVarChar)
		bboxColumn, _ := result.Fields.GetColumn("bbox").(*entity.ColumnVarChar)

		for i := 0; i < result.ResultCount; i++ {
			text, _ := textColumn.ValueByIdx(i)
			userID, _ := userColumn.ValueByIdx(i)
			documentID, _ := documentColumn.ValueByIdx(i)
			fileName, _ := fileColumn.ValueByIdx(i)
			chunkIndex, _ := chunkColumn.ValueByIdx(i)
			pageNum, _ := pageNumColumn.ValueByIdx(i)
			chunkType, _ := chunkTypeColumn.ValueByIdx(i)
			chunkRole, _ := chunkRoleColumn.ValueByIdx(i)
			parentID, _ := parentIDColumn.ValueByIdx(i)
			sectionPathStr, _ := sectionPathColumn.ValueByIdx(i)
			bboxStr, _ := bboxColumn.ValueByIdx(i)

			metadata := map[string]any{
				"user_id":     userID,
				"document_id": documentID,
				"file_name":   fileName,
				"chunk_index": chunkIndex,
				"page_num":    pageNum,
				"chunk_type":  chunkType,
				"chunk_role":  chunkRole,
				"parent_id":   parentID,
			}

			// 反序列化 section_path
			if sectionPathStr != "" {
				var sp []string
				if err := json.Unmarshal([]byte(sectionPathStr), &sp); err == nil {
					metadata["section_path"] = sp
				} else {
					metadata["section_path"] = sectionPathStr
				}
			}

			// 反序列化 bbox
			if bboxStr != "" {
				var bbox []float32
				if err := json.Unmarshal([]byte(bboxStr), &bbox); err == nil {
					metadata["bbox"] = bbox
				} else {
					metadata["bbox"] = bboxStr
				}
			}

			docs = append(docs, schema.Document{
				PageContent: text,
				Score:       result.Scores[i],
				Metadata:    metadata,
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

	// Phase 1: 查找已有兼容的 collection
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
			break // 不存在，后续处理
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

	// Phase 2: 没有兼容的 collection，需要创建。先尝试在 baseName 上创建。
	// 如果 baseName 已存在但不兼容，先删除旧 collection 释放空间。
	has, err := cli.HasCollection(ctx, baseName)
	if err != nil {
		return "", fmt.Errorf("check collection %s: %w", baseName, err)
	}
	if has {
		zap.L().Warn("dropping incompatible Milvus collection to reclaim slot",
			zap.String("collection", baseName),
		)
		if err := cli.DropCollection(ctx, baseName); err != nil {
			return "", fmt.Errorf("drop incompatible collection %s: %w", baseName, err)
		}
	}

	// 尝试创建新 collection
	if err := createMilvusCollection(ctx, cli, baseName, cfg.Dimension); err != nil {
		// 如果创建失败（可能是 collection 数量上限），尝试清理更多旧版本
		zap.L().Warn("failed to create collection, attempting to clean up old versions",
			zap.String("collection", baseName),
			zap.Error(err),
		)
		for version := 2; version <= 10; version++ {
			candidate := fmt.Sprintf("%s_v%d", baseName, version)
			exists, checkErr := cli.HasCollection(ctx, candidate)
			if checkErr != nil {
				continue
			}
			if exists {
				zap.L().Warn("dropping old versioned Milvus collection to reclaim slot",
					zap.String("collection", candidate),
				)
				if dropErr := cli.DropCollection(ctx, candidate); dropErr != nil {
					zap.L().Warn("failed to drop collection", zap.String("collection", candidate), zap.Error(dropErr))
					continue
				}
				// 清理一个后重试创建
				if createErr := createMilvusCollection(ctx, cli, baseName, cfg.Dimension); createErr == nil {
					return baseName, nil
				}
			}
		}
		return "", fmt.Errorf("unable to create compatible Milvus collection %s after cleanup: %w", baseName, err)
	}

	return baseName, nil
}

func createMilvusCollection(ctx context.Context, cli client.Client, collectionName string, dimension int) error {
	schemaDef := entity.NewSchema().
		WithName(collectionName).
		WithDescription("enterprise pdf chunks with structured metadata").
		WithAutoID(true).
		WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
		WithField(entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("file_name").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)).
		WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("page_num").WithDataType(entity.FieldTypeInt64)).
		WithField(entity.NewField().WithName("chunk_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
		WithField(entity.NewField().WithName("chunk_role").WithDataType(entity.FieldTypeVarChar).WithMaxLength(16)).
		WithField(entity.NewField().WithName("parent_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
		WithField(entity.NewField().WithName("section_path").WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024)).
		WithField(entity.NewField().WithName("bbox").WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
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

// ========== metadata 辅助函数（扩展） ==========

// metadataSliceJSON 将 metadata 中的字符串切片序列化为 JSON
func metadataSliceJSON(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case []string:
		if len(val) == 0 {
			return ""
		}
		b, _ := json.Marshal(val)
		return string(b)
	case string:
		return val
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}

// metadataAnyJSON 将 metadata 中的任意值序列化为 JSON
func metadataAnyJSON(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	default:
		b, _ := json.Marshal(val)
		return string(b)
	}
}
