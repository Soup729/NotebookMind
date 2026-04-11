package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// OpenAIVLMProvider 使用 OpenAI Vision API 作为 VLM
type OpenAIVLMProvider struct {
	APIKey   string
	BaseURL  string
	Model    string // 如 "gpt-4o-mini", "gpt-4o"
	Client   *http.Client
	available bool
}

// NewOpenAIVLMProvider 创建 OpenAI VLM 客户端
func NewOpenAIVLMProvider(apiKey, baseURL, model string) *OpenAIVLMProvider {
	if apiKey == "" || model == "" {
		return &OpenAIVLMProvider{available: false}
	}

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIVLMProvider{
		APIKey:   apiKey,
		BaseURL:  strings.TrimSuffix(baseURL, "/v1"),
		Model:    model,
		Client:   &http.Client{},
		available: true,
	}
}

// IsAvailable 检查 VLM 是否可用
func (o *OpenAIVLMProvider) IsAvailable() bool {
	return o.available
}

// DescribeImage 调用 OpenAI Vision API 生成图片描述
func (o *OpenAIVLMProvider) DescribeImage(ctx context.Context, imageBytes []byte, mimeType string, prompt string) (string, error) {
	if !o.available {
		return "", fmt.Errorf("OpenAI VLM is not configured")
	}

	url := o.BaseURL + "/v1/chat/completions"

	encodedImage := base64.StdEncoding.EncodeToString(imageBytes)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	reqBody := map[string]any{
		"model": o.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{
						"url": fmt.Sprintf("data:%s;base64,%s", mimeType, encodedImage),
					}},
				},
			},
		},
		"max_tokens": 500,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create vlm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.Client.Do(req)
	if err != nil {
		o.available = false
		return "", fmt.Errorf("vlm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody := make([]byte, 8192)
	n, _ := resp.Body.Read(respBody)
	respBody = respBody[:n]

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vlm api error status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		zap.L().Warn("failed to parse vlm response", zap.Error(err))
		return "", fmt.Errorf("parse vlm response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no vlm response choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// FallbackVLMProvider 降级 VLM（不生成描述）
type FallbackVLMProvider struct{}

// NewFallbackVLMProvider 创建降级 VLM
func NewFallbackVLMProvider() *FallbackVLMProvider {
	return &FallbackVLMProvider{}
}

func (f *FallbackVLMProvider) IsAvailable() bool { return true }

func (f *FallbackVLMProvider) DescribeImage(ctx context.Context, imageBytes []byte, mimeType, prompt string) (string, error) {
	return "", fmt.Errorf("fallback VLM: no vision capability available")
}
