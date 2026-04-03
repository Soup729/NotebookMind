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

// TestLLMServiceAndMilvusIntegration 是一个集成测试
// 注意：运行此测试前，需确保本地 Milvus 运行在 localhost:19530，且具备有效的 OpenAI API Key。
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
		Username:       "",
		Password:       milvusPassword,
		CollectionName: "test_pdf_chunks",
		Dimension:      1536,
	}

	_ = logger.InitLogger(&configs.LogConfig{Level: "debug"}) // 为了看清楚内部执行

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2. 初始化 Service
	svc, err := NewLLMService(ctx, llmCfg, milvusCfg)
	require.NoError(t, err, "Failed to initialize LLMService")
	require.NotNil(t, svc)

	// 3. 准备假数据 (Fake PDF Chunks)
	mockDocs := []schema.Document{
		{
			PageContent: "Langchaingo is a Go port of the popular LangChain framework.",
			Metadata:    map[string]any{"source": "doc1.pdf", "page": 1},
		},
		{
			PageContent: "Milvus is an open-source vector database built to power embedding similarity search and AI applications.",
			Metadata:    map[string]any{"source": "doc2.pdf", "page": 2},
		},
	}

	// 4. 执行 Index (向量化并存入 Milvus)
	err = svc.IndexDocuments(ctx, mockDocs)
	require.NoError(t, err, "Failed to index documents")

	// 等待 Milvus 索引建立完毕 (可微调时间)
	time.Sleep(2 * time.Second)

	// 5. 执行 Retrieve (基于语义的相似度搜索)
	query := "What database is used for vector search?"
	results, err := svc.RetrieveContext(ctx, query, 1) // 取最相关的一条

	require.NoError(t, err, "Failed to retrieve context")
	assert.Len(t, results, 1, "Should retrieve exactly 1 document")

	// 断言：召回的内容应该和 doc2.pdf 相关
	t.Logf("Retrieved content: %s", results[0].PageContent)
	assert.Contains(t, results[0].PageContent, "Milvus", "The retrieved document should mention Milvus")
}
