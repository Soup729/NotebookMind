package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/repository"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ModelProvider represents supported LLM providers
type ModelProvider string

const (
	ProviderOpenAI      ModelProvider = "openai"
	ProviderAnthropic   ModelProvider = "anthropic"
	ProviderGemini      ModelProvider = "gemini"
	ProviderAzureOpenAI ModelProvider = "azure-openai"
	ProviderCustom      ModelProvider = "custom"
)

// RetrievalOptions contains options for context retrieval
type RetrievalOptions struct {
	UserID      string
	DocumentIDs []string
}

// GeneratedAnswer represents an LLM response
type GeneratedAnswer struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// LLMService defines the interface for LLM operations
type LLMService interface {
	IndexDocuments(ctx context.Context, docs []schema.Document) error
	RetrieveContext(ctx context.Context, query string, topK int, opts RetrievalOptions) ([]schema.Document, error)
	DeleteDocumentChunks(ctx context.Context, userID, documentID string) error
	GenerateAnswer(ctx context.Context, prompt string) (*GeneratedAnswer, error)
	GenerateAnswerWithProvider(ctx context.Context, prompt string, provider ModelProvider) (*GeneratedAnswer, error)
	GenerateReflection(ctx context.Context, question, answer string, sources []string) (*ReflectionResult, error)
	AnswerWithImage(ctx context.Context, question string, imageData []byte, mimeType string) (*GeneratedAnswer, error)
}

// llmService implements LLMService with multi-provider support
type llmService struct {
	vectorStore repository.VectorStore
	llm         llms.Model
	embedder    embeddings.Embedder
	cfg         *configs.LLMConfig
	httpClient  *http.Client
}

// ReflectionResult contains the reflection analysis
type ReflectionResult struct {
	AccuracyScore       int      `json:"accuracy_score"`       // 1-5
	CompletenessScore   int      `json:"completeness_score"`   // 1-5
	SourceCoverage      []string `json:"source_coverage"`      // Which sources were used
	MissingAspects      []string `json:"missing_aspects"`      // What might be missing
	SuggestedImprovements []string `json:"suggested_improvements"` // How to improve
	ConfidenceLevel     string   `json:"confidence_level"`     // high, medium, low
}

// NewLLMService creates a new LLM service with multi-provider support
func NewLLMService(ctx context.Context, db *gorm.DB, llmCfg *configs.LLMConfig, milvusCfg *configs.MilvusConfig) (LLMService, error) {
	// Create HTTP client for multi-provider calls
	httpClient := &http.Client{}

	// Create embedder (always uses OpenAI for embeddings)
	embeddingClient, err := openai.New(
		openai.WithToken(llmCfg.Providers.OpenAI.APIKey),
		openai.WithBaseURL(llmCfg.Providers.OpenAI.BaseURL),
		openai.WithEmbeddingModel(llmCfg.Providers.OpenAI.EmbeddingModel),
	)
	if err != nil {
		return nil, fmt.Errorf("create OpenAI embedding client: %w", err)
	}
	embedder, err := embeddings.NewEmbedder(embeddingClient)
	if err != nil {
		return nil, fmt.Errorf("create embedder: %w", err)
	}

	// Create default LLM client based on provider
	defaultProvider := llmCfg.DefaultProvider
	if defaultProvider == "" {
		defaultProvider = "openai"
	}

	var chatClient llms.Model
	switch defaultProvider {
	case "openai":
		chatClient, err = openai.New(
			openai.WithToken(llmCfg.Providers.OpenAI.APIKey),
			openai.WithBaseURL(llmCfg.Providers.OpenAI.BaseURL),
			openai.WithModel(llmCfg.Providers.OpenAI.ChatModel),
		)
	case "anthropic":
		baseURL := llmCfg.Providers.Anthropic.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		chatClient, err = openai.New(
			openai.WithToken(llmCfg.Providers.Anthropic.APIKey),
			openai.WithBaseURL(baseURL),
			openai.WithModel(llmCfg.Providers.Anthropic.ChatModel),
		)
	case "gemini":
		baseURL := llmCfg.Providers.Gemini.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		chatClient, err = openai.New(
			openai.WithToken(llmCfg.Providers.Gemini.APIKey),
			openai.WithBaseURL(baseURL+"/v1"),
			openai.WithModel(llmCfg.Providers.Gemini.ChatModel),
		)
	case "custom":
		// Custom: use first configured custom model as default
		if len(llmCfg.Providers.CustomModels) > 0 && strings.TrimSpace(llmCfg.Providers.CustomModels[0].APIKey) != "" {
			cm := llmCfg.Providers.CustomModels[0]
			chatClient, err = openai.New(
				openai.WithToken(cm.APIKey),
				openai.WithBaseURL(cm.BaseURL),
				openai.WithModel(cm.ChatModel),
			)
		} else {
			return nil, fmt.Errorf("no custom model configured with valid API key")
		}
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", defaultProvider)
	}

	if err != nil {
		return nil, fmt.Errorf("create chat client for %s: %w", defaultProvider, err)
	}

	// Create vector store
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
		embedder:    embedder,
		cfg:         llmCfg,
		httpClient:  httpClient,
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
	return s.GenerateAnswerWithProvider(ctx, prompt, ModelProvider(s.cfg.DefaultProvider))
}

func (s *llmService) GenerateAnswerWithProvider(ctx context.Context, prompt string, provider ModelProvider) (*GeneratedAnswer, error) {
	var client llms.Model
	var err error

	switch provider {
	case ProviderOpenAI:
		client, err = openai.New(
			openai.WithToken(s.cfg.Providers.OpenAI.APIKey),
			openai.WithBaseURL(s.cfg.Providers.OpenAI.BaseURL),
			openai.WithModel(s.cfg.Providers.OpenAI.ChatModel),
		)
	case ProviderAnthropic:
		baseURL := s.cfg.Providers.Anthropic.BaseURL
		if baseURL == "" {
			baseURL = "https://api.anthropic.com/v1"
		}
		client, err = openai.New(
			openai.WithToken(s.cfg.Providers.Anthropic.APIKey),
			openai.WithBaseURL(baseURL),
			openai.WithModel(s.cfg.Providers.Anthropic.ChatModel),
		)
	case ProviderGemini:
		baseURL := s.cfg.Providers.Gemini.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com"
		}
		client, err = openai.New(
			openai.WithToken(s.cfg.Providers.Gemini.APIKey),
			openai.WithBaseURL(baseURL+"/v1"),
			openai.WithModel(s.cfg.Providers.Gemini.ChatModel),
		)
	case ProviderCustom:
		// Custom model: fallback to first configured, or default provider
		if len(s.cfg.Providers.CustomModels) > 0 {
			cm := s.cfg.Providers.CustomModels[0]
			client, err = openai.New(
				openai.WithToken(cm.APIKey),
				openai.WithBaseURL(cm.BaseURL),
				openai.WithModel(cm.ChatModel),
			)
		} else {
			client = s.llm
		}
	default:
		// Try to match as custom model ID (e.g., "custom:deepseek")
		if strings.HasPrefix(string(provider), "custom:") {
			customID := strings.TrimPrefix(string(provider), "custom:")
			for _, cm := range s.cfg.Providers.CustomModels {
				if cm.ID == customID && strings.TrimSpace(cm.APIKey) != "" {
					client, err = openai.New(
						openai.WithToken(cm.APIKey),
						openai.WithBaseURL(cm.BaseURL),
						openai.WithModel(cm.ChatModel),
					)
					break
				}
			}
		}
		if client == nil && err == nil {
			return nil, fmt.Errorf("unknown provider/model: %s", provider)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("create client for %s: %w", provider, err)
	}

	response, err := llms.GenerateFromSinglePrompt(ctx, client, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate response: %w", err)
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

// GenerateReflection analyzes the answer and provides feedback
func (s *llmService) GenerateReflection(ctx context.Context, question, answer string, sources []string) (*ReflectionResult, error) {
	sourcesStr := strings.Join(sources, "\n")
	if sourcesStr == "" {
		sourcesStr = "No sources provided"
	}

	prompt := fmt.Sprintf(`Analyze the following question, answer pair and sources. Provide a detailed reflection.

Question: %s

Answer: %s

Sources:
%s

Provide a reflection in JSON format with these fields:
- accuracy_score: 1-5 rating of how accurate the answer is to the question
- completeness_score: 1-5 rating of how complete the answer is
- source_coverage: list of which sources were actually used in the answer
- missing_aspects: list of aspects that might be missing or could be improved
- suggested_improvements: list of specific suggestions to improve the answer
- confidence_level: "high", "medium", or "low" based on source relevance

Return ONLY valid JSON, no markdown formatting.`, question, answer, sourcesStr)

	response, err := llms.GenerateFromSinglePrompt(ctx, s.llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate reflection: %w", err)
	}

	// Parse JSON response (simplified)
	text := strings.TrimSpace(response)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimSuffix(text, "```")
	text = strings.Trim(text, " \n")

	result := &ReflectionResult{
		AccuracyScore:       4,
		CompletenessScore:   4,
		SourceCoverage:      sources,
		MissingAspects:      []string{},
		SuggestedImprovements: []string{},
		ConfidenceLevel:     "medium",
	}

	// Try to parse as JSON if it looks like JSON
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		// Basic field extraction
		if strings.Contains(text, `"accuracy_score"`) {
			// Extract scores using simple string matching
			result.AccuracyScore = extractJSONInt(text, "accuracy_score", 4)
			result.CompletenessScore = extractJSONInt(text, "completeness_score", 4)
			result.ConfidenceLevel = extractJSONString(text, "confidence_level", "medium")
		}
	}

	return result, nil
}

// AnswerWithImage handles visual question answering
func (s *llmService) AnswerWithImage(ctx context.Context, question string, imageData []byte, mimeType string) (*GeneratedAnswer, error) {
	// Check which provider is configured for vision
	provider := ModelProvider(s.cfg.DefaultProvider)

	// Base64 encode the image
	encodedImage := base64.StdEncoding.EncodeToString(imageData)

	switch provider {
	case ProviderOpenAI:
		return s.answerWithOpenAIVision(ctx, question, encodedImage, mimeType)
	case ProviderAnthropic:
		return s.answerWithAnthropicVision(ctx, question, encodedImage, mimeType)
	case ProviderGemini:
		return s.answerWithGeminiVision(ctx, question, encodedImage, mimeType)
	default:
		// Default to OpenAI vision
		return s.answerWithOpenAIVision(ctx, question, encodedImage, mimeType)
	}
}

// answerWithOpenAIVision uses OpenAI's GPT-4V for image understanding
func (s *llmService) answerWithOpenAIVision(ctx context.Context, question, base64Image, mimeType string) (*GeneratedAnswer, error) {
	model := s.cfg.Providers.OpenAI.VisionModel
	if model == "" {
		model = "gpt-4o-mini"
	}

	url := strings.TrimSuffix(s.cfg.Providers.OpenAI.BaseURL, "/v1") + "/v1/chat/completions"

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": question},
					{"type": "image_url", "image_url": map[string]string{
						"url": fmt.Sprintf("data:%s;base64,%s", mimeType, base64Image),
					}},
				},
			},
		},
		"max_tokens": 4096,
	}

	reqJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.Providers.OpenAI.APIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vision request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vision API error: %s", string(body))
	}

	// Parse OpenAI response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse vision response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no response from vision model")
	}

	return &GeneratedAnswer{
		Text:             result.Choices[0].Message.Content,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
		TotalTokens:      result.Usage.TotalTokens,
	}, nil
}

// answerWithAnthropicVision uses Claude for image understanding
func (s *llmService) answerWithAnthropicVision(ctx context.Context, question, base64Image, mimeType string) (*GeneratedAnswer, error) {
	model := s.cfg.Providers.Anthropic.VisionModel
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	url := s.cfg.Providers.Anthropic.BaseURL + "/v1/messages"

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": question},
					{"type": "image", "source": map[string]string{
						"type":      "base64",
						"media_type": mimeType,
						"data":      base64Image,
					}},
				},
			},
		},
		"max_tokens": 4096,
	}

	reqJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.cfg.Providers.Anthropic.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vision request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vision API error: %s", string(body))
	}

	// Parse Anthropic response
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse vision response: %w", err)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("no response from vision model")
	}

	return &GeneratedAnswer{
		Text:             result.Content[0].Text,
		PromptTokens:     result.Usage.InputTokens,
		CompletionTokens: result.Usage.OutputTokens,
		TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
	}, nil
}

// answerWithGeminiVision uses Google Gemini for image understanding
func (s *llmService) answerWithGeminiVision(ctx context.Context, question, base64Image, mimeType string) (*GeneratedAnswer, error) {
	model := s.cfg.Providers.Gemini.ChatModel
	if model == "" {
		model = "gemini-2.0-flash"
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		s.cfg.Providers.Gemini.BaseURL, model, s.cfg.Providers.Gemini.APIKey)

	mediaType := mimeType
	if mediaType == "image/jpeg" {
		mediaType = "image/jpg"
	}

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": question},
					{"inline_data": map[string]string{
						"mime_type": mediaType,
						"data":       base64Image,
					}},
				},
			},
		},
	}

	reqJSON, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vision request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vision API error: %s", string(body))
	}

	// Parse Gemini response
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse vision response: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from vision model")
	}

	return &GeneratedAnswer{
		Text:             result.Candidates[0].Content.Parts[0].Text,
		PromptTokens:     result.UsageMetadata.PromptTokenCount,
		CompletionTokens: result.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      result.UsageMetadata.TotalTokenCount,
	}, nil
}

// Helper functions

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

func extractJSONInt(text, key string, defaultVal int) int {
	pattern := fmt.Sprintf(`"%s":%d`, key, defaultVal)
	if strings.Contains(text, pattern) {
		// Simple extraction
		idx := strings.Index(text, fmt.Sprintf(`"%s":`, key))
		if idx >= 0 {
			start := idx + len(fmt.Sprintf(`"%s":`, key))
			end := start
			for end < len(text) && (text[end] >= '0' && text[end] <= '9') {
				end++
			}
			if end > start {
				val := 0
				fmt.Sscanf(text[start:end], "%d", &val)
				if val >= 1 && val <= 5 {
					return val
				}
			}
		}
	}
	return defaultVal
}

func extractJSONString(text, key, defaultVal string) string {
	pattern := fmt.Sprintf(`"%s":"`, key)
	idx := strings.Index(text, pattern)
	if idx >= 0 {
		start := idx + len(pattern)
		end := start
		for end < len(text) && text[end] != '"' {
			end++
		}
		if end > start {
			return text[start:end]
		}
	}
	return defaultVal
}