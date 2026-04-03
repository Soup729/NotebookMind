package repository

import (
	"context"
	"fmt"
	"strconv" // 替代 time，用于维度转换

	"enterprise-pdf-ai/internal/configs"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

// VectorStore 定义接口
type VectorStore interface {
	AddDocuments(ctx context.Context, docs []schema.Document) error
	SimilaritySearch(ctx context.Context, query string, k int) ([]schema.Document, error)
}

type milvusRepo struct {
	cli       client.Client
	embedder  embeddings.Embedder
	colName   string
	dimension int
}

func NewMilvusStore(ctx context.Context, cfg *configs.MilvusConfig, embedder embeddings.Embedder) (VectorStore, error) {
	zap.L().Info("Initializing Milvus vector store with SDK v2.4.1...",
		zap.String("address", cfg.Address),
		zap.String("collection", cfg.CollectionName))

	apiKey := cfg.Password
	if cfg.Username != "" && cfg.Username != "db_admin" {
		apiKey = cfg.Username + ":" + cfg.Password
	}

	c, err := client.NewClient(ctx, client.Config{
		Address: cfg.Address,
		APIKey:  apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create milvus client: %w", err)
	}

	has, err := c.HasCollection(ctx, cfg.CollectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to check collection: %w", err)
	}

	if !has {
		zap.L().Info("Collection not found, creating...", zap.String("collection", cfg.CollectionName))

		// ⭐ 修复点 1：使用结构体字面量定义 Fields，避免 Builder 方法缺失错误
		fields := []*entity.Field{
			{
				Name:       "id",
				DataType:   entity.FieldTypeInt64,
				PrimaryKey: true,
				AutoID:     true, // 显式设置字段级 AutoID
			},
			{
				Name:     "text",
				DataType: entity.FieldTypeVarChar,
				TypeParams: map[string]string{
					entity.TypeParamMaxLength: "65535",
				},
			},
			{
				Name:     "vector",
				DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{
					entity.TypeParamDim: strconv.Itoa(cfg.Dimension),
				},
			},
		}

		// ⭐ 修复点 2：直接使用结构体定义 Schema
		collSchema := &entity.Schema{
			CollectionName: cfg.CollectionName,
			AutoID:         true,
			Fields:         fields,
			Description:    "RAG PDF chunks",
		}

		err = c.CreateCollection(ctx, collSchema, entity.DefaultShardNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to create collection: %w", err)
		}

		// 创建索引
		idx, _ := entity.NewIndexIvfFlat(entity.L2, 1024)
		err = c.CreateIndex(ctx, cfg.CollectionName, "vector", idx, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create index: %w", err)
		}
	}

	// 加载集合
	err = c.LoadCollection(ctx, cfg.CollectionName, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load collection: %w", err)
	}

	return &milvusRepo{
		cli:       c,
		embedder:  embedder,
		colName:   cfg.CollectionName,
		dimension: cfg.Dimension,
	}, nil
}

func (m *milvusRepo) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if len(docs) == 0 {
		return nil
	}

	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}

	vectors, err := m.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to embed documents: %w", err)
	}

	// ⭐ 修复点 3：NewColumnFloatVector 第二个参数需要 int，不是 int64
	textCol := entity.NewColumnVarChar("text", texts)
	vecCol := entity.NewColumnFloatVector("vector", m.dimension, vectors)

	_, err = m.cli.Insert(ctx, m.colName, "", textCol, vecCol)
	if err != nil {
		return fmt.Errorf("milvus insert failed: %w", err)
	}

	return nil
}

func (m *milvusRepo) SimilaritySearch(ctx context.Context, query string, k int) ([]schema.Document, error) {
	vector, err := m.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	sp, _ := entity.NewIndexIvfFlatSearchParam(10)
	results, err := m.cli.Search(
		ctx, m.colName, nil, "", []string{"text"},
		[]entity.Vector{entity.FloatVector(vector)},
		"vector", entity.L2, k, sp,
	)
	if err != nil {
		return nil, fmt.Errorf("milvus search failed: %w", err)
	}

	var docs []schema.Document
	for _, sr := range results {
		col := sr.Fields.GetColumn("text")
		if textCol, ok := col.(*entity.ColumnVarChar); ok {
			for i := 0; i < sr.ResultCount; i++ {
				val, _ := textCol.ValueByIdx(i)
				docs = append(docs, schema.Document{
					PageContent: val,
					Score:       sr.Scores[i],
				})
			}
		}
	}
	return docs, nil
}