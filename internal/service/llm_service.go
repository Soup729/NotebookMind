package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/repository"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type RetrievalOptions struct {
	UserID      string
	DocumentIDs []string
}

type GeneratedAnswer struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type LLMService interface {
	IndexDocuments(ctx context.Context, docs []schema.Document) error
	RetrieveContext(ctx context.Context, query string, topK int, opts RetrievalOptions) ([]schema.Document, error)
	DeleteDocumentChunks(ctx context.Context, userID, documentID string) error
	GenerateAnswer(ctx context.Context, prompt string) (*GeneratedAnswer, error)
}

type llmService struct {
	vectorStore repository.VectorStore
	llm         llms.Model
}

func NewLLMService(ctx context.Context, db *gorm.DB, llmCfg *configs.LLMConfig, milvusCfg *configs.MilvusConfig) (LLMService, error) {
	embeddingClient, err := openai.New(
		openai.WithToken(llmCfg.OpenAI.APIKey),
		openai.WithBaseURL(llmCfg.OpenAI.BaseURL),
		openai.WithEmbeddingModel(llmCfg.OpenAI.EmbeddingModel),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenAI embedding client: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embeddingClient)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	chatClient, err := openai.New(
		openai.WithToken(llmCfg.OpenAI.APIKey),
		openai.WithBaseURL(llmCfg.OpenAI.BaseURL),
		openai.WithModel(llmCfg.OpenAI.ChatModel),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenAI chat client: %w", err)
	}

	var vectorStore repository.VectorStore
	if shouldUseLocalVectorStoreFallback(milvusCfg) {
		zap.L().Warn("Milvus config incomplete, falling back to PostgreSQL-backed local vector store")
		vectorStore, err = repository.NewLocalStore(db, embedder)
		if err != nil {
			return nil, fmt.Errorf("create local vector store: %w", err)
		}
	} else {
		vectorStore, err = repository.NewMilvusStore(ctx, milvusCfg, embedder)
		if err != nil {
			return nil, fmt.Errorf("create Milvus vector store: %w", err)
		}
	}

	return &llmService{
		vectorStore: vectorStore,
		llm:         chatClient,
	}, nil
}

func (s *llmService) IndexDocuments(ctx context.Context, docs []schema.Document) error {
	if err := s.vectorStore.AddDocuments(ctx, docs); err != nil {
		return fmt.Errorf("index documents: %w", err)
	}
	return nil
}

func (s *llmService) RetrieveContext(ctx context.Context, query string, topK int, opts RetrievalOptions) ([]schema.Document, error) {
	docs, err := s.vectorStore.SimilaritySearch(ctx, query, topK, repository.VectorSearchOptions{
		UserID:      opts.UserID,
		DocumentIDs: opts.DocumentIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("retrieve context: %w", err)
	}
	return docs, nil
}

func (s *llmService) DeleteDocumentChunks(ctx context.Context, userID, documentID string) error {
	if err := s.vectorStore.DeleteDocuments(ctx, repository.VectorSearchOptions{
		UserID:      userID,
		DocumentIDs: []string{documentID},
	}); err != nil {
		return fmt.Errorf("delete vector chunks: %w", err)
	}
	return nil
}

func (s *llmService) GenerateAnswer(ctx context.Context, prompt string) (*GeneratedAnswer, error) {
	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate llm response: %w", err)
	}

	text := strings.TrimSpace(response)
	if text == "" {
		return nil, fmt.Errorf("llm returned empty response")
	}

	promptTokens := estimateTokens(prompt)
	completionTokens := estimateTokens(text)
	return &GeneratedAnswer{
		Text:             text,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}, nil
}

func shouldUseLocalVectorStoreFallback(cfg *configs.MilvusConfig) bool {
	if cfg == nil {
		return true
	}
	return strings.TrimSpace(cfg.Address) == "" || strings.TrimSpace(cfg.Password) == ""
}

func estimateTokens(input string) int {
	if input == "" {
		return 0
	}
	return (utf8.RuneCountInString(input) + 3) / 4
}
