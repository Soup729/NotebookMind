package service

import (
	"context"
	"os"
	"testing"
	"time"

	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/platform/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/schema"
)

func init() {
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:7897")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:7897")
}

func TestLLMServiceAndMilvusIntegration(t *testing.T) {
	if os.Getenv("RUN_LLM_INTEGRATION_TEST") != "1" {
		t.Skip("set RUN_LLM_INTEGRATION_TEST=1 to run integration test")
	}

	openAIKey := os.Getenv("OPENAI_API_KEY")
	milvusAddress := os.Getenv("MILVUS_ADDRESS")
	milvusPassword := os.Getenv("MILVUS_PASSWORD")
	if openAIKey == "" || milvusAddress == "" || milvusPassword == "" {
		t.Skip("OPENAI_API_KEY, MILVUS_ADDRESS, MILVUS_PASSWORD are required for integration test")
	}

	llmCfg := &configs.LLMConfig{
		OpenAI: configs.OpenAIConfig{
			APIKey:         openAIKey,
			BaseURL:        "https://api.openai.com/v1",
			EmbeddingModel: "text-embedding-3-small",
			ChatModel:      "gpt-4o-mini",
		},
	}

	milvusCfg := &configs.MilvusConfig{
		Address:        milvusAddress,
		Password:       milvusPassword,
		CollectionName: "test_pdf_chunks",
		Dimension:      1536,
	}

	_ = logger.InitLogger(&configs.LogConfig{Level: "debug"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc, err := NewLLMService(ctx, nil, llmCfg, milvusCfg)
	require.NoError(t, err)
	require.NotNil(t, svc)

	mockDocs := []schema.Document{
		{
			PageContent: "Langchaingo is a Go port of the popular LangChain framework.",
			Metadata: map[string]any{
				"user_id":     "test-user",
				"document_id": "doc-1",
				"file_name":   "doc1.pdf",
				"chunk_index": 0,
			},
		},
		{
			PageContent: "Milvus is an open-source vector database built to power embedding similarity search and AI applications.",
			Metadata: map[string]any{
				"user_id":     "test-user",
				"document_id": "doc-2",
				"file_name":   "doc2.pdf",
				"chunk_index": 0,
			},
		},
	}

	err = svc.IndexDocuments(ctx, mockDocs)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	results, err := svc.RetrieveContext(ctx, "What database is used for vector search?", 1, RetrievalOptions{UserID: "test-user"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].PageContent, "Milvus")
}
