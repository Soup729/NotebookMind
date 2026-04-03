package service

import (
	"context"
	"fmt"
	"strings"

	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/repository"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
)

// LLMService 定义了核心 RAG 业务接口
type LLMService interface {
	IndexDocuments(ctx context.Context, docs []schema.Document) error
	RetrieveContext(ctx context.Context, query string, topK int) ([]schema.Document, error)
	GenerateAnswer(ctx context.Context, prompt string) (string, error)
}

type llmService struct {
	vectorStore repository.VectorStore
	embedder    embeddings.Embedder
	llm         llms.Model
}

// NewLLMService 初始化大模型服务，包含 Embeddings 客户端，并将其实例化注入到 Milvus Repository 中
func NewLLMService(ctx context.Context, llmCfg *configs.LLMConfig, milvusCfg *configs.MilvusConfig) (LLMService, error) {
	// 1. 初始化 OpenAI 客户端 (专门用于生成 Embedding)
	zap.L().Info("Initializing OpenAI client for embeddings",
		zap.String("model", llmCfg.OpenAI.EmbeddingModel),
		zap.String("base_url", llmCfg.OpenAI.BaseURL))

	embeddingLLM, err := openai.New(
		openai.WithToken(llmCfg.OpenAI.APIKey),
		openai.WithBaseURL(llmCfg.OpenAI.BaseURL),
		openai.WithEmbeddingModel(llmCfg.OpenAI.EmbeddingModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create openai client: %w", err)
	}

	// 2. 包装成 Embedder 接口
	embedder, err := embeddings.NewEmbedder(embeddingLLM)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	chatLLM, err := openai.New(
		openai.WithToken(llmCfg.OpenAI.APIKey),
		openai.WithBaseURL(llmCfg.OpenAI.BaseURL),
		openai.WithModel(llmCfg.OpenAI.ChatModel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat llm client: %w", err)
	}

	// 3. 实例化并注入到 VectorStore (Milvus)
	store, err := repository.NewMilvusStore(ctx, milvusCfg, embedder)
	if err != nil {
		return nil, fmt.Errorf("failed to init vector store: %w", err)
	}

	return &llmService{
		vectorStore: store,
		embedder:    embedder,
		llm:         chatLLM,
	}, nil
}

// IndexDocuments 将外部处理好的 Document 切片，通过 Embedder 转换为向量后存入 Milvus
func (s *llmService) IndexDocuments(ctx context.Context, docs []schema.Document) error {
	zap.L().Info("Indexing documents via LLMService", zap.Int("docs_count", len(docs)))
	// 这里 VectorStore 内部会自动调用 Embedder 产生向量并存入
	return s.vectorStore.AddDocuments(ctx, docs)
}

// RetrieveContext 接收用户的自然语言 query，进行向量相似度检索召回上下文
func (s *llmService) RetrieveContext(ctx context.Context, query string, topK int) ([]schema.Document, error) {
	zap.L().Info("Retrieving context via LLMService", zap.String("query", query), zap.Int("topK", topK))
	// VectorStore 内部会自动将 query 通过 Embedder 转成向量，再在 Milvus 中检索
	return s.vectorStore.SimilaritySearch(ctx, query, topK)
}

func (s *llmService) GenerateAnswer(ctx context.Context, prompt string) (string, error) {
	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return "", fmt.Errorf("generate llm response: %w", err)
	}

	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return "", fmt.Errorf("llm returned empty response")
	}

	return trimmed, nil
}
