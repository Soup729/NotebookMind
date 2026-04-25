package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"NotebookAI/internal/configs"

	"go.uber.org/zap"
)

// ============ Reranker 服务 (Phase 2) ============

// RerankerService Reranker 接口
type RerankerService interface {
	Rerank(ctx context.Context, query string, candidates []HybridResult, topK int) ([]HybridResult, error)
}

// ============ Cohere Reranker ============

// cohereReranker 基于 Cohere API 的 Reranker
type cohereReranker struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewCohereReranker 创建 Cohere Reranker
func NewCohereReranker(apiKey string, cfg *configs.RerankerConfig) RerankerService {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &cohereReranker{
		apiKey:  apiKey,
		model:   cfg.Model,
		baseURL: cfg.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// cohereRequest Cohere Rerank API 请求体
type cohereRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

// cohereResponse Cohere Rerank API 响应体
type cohereResponse struct {
	Results []cohereResult `json:"results"`
	Meta    struct {
		BilledUnits struct {
			SearchUnits int `json:"search_units"`
		} `json:"billed_units"`
	} `json:"meta"`
}

type cohereResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// Rerank 使用 Cohere API 重排序
func (r *cohereReranker) Rerank(ctx context.Context, query string, candidates []HybridResult, topK int) ([]HybridResult, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	if topK <= 0 || topK > len(candidates) {
		topK = len(candidates)
	}

	// 构建文档列表
	documents := make([]string, len(candidates))
	for i, c := range candidates {
		documents[i] = c.Content
	}

	reqBody := cohereRequest{
		Model:     r.model,
		Query:     query,
		Documents: documents,
		TopN:      topK,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	url := r.baseURL + "/rerank"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cohere rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cohere rerank API error (status %d): %s", resp.StatusCode, string(body))
	}

	var cohereResp cohereResponse
	if err := json.Unmarshal(body, &cohereResp); err != nil {
		return nil, fmt.Errorf("parse rerank response: %w", err)
	}

	// 根据 Cohere 返回的索引重新排序
	results := make([]HybridResult, 0, len(cohereResp.Results))
	for _, r := range cohereResp.Results {
		if r.Index < len(candidates) {
			result := candidates[r.Index]
			result.Score = float32(r.RelevanceScore)
			results = append(results, result)
		}
	}

	zap.L().Debug("cohere rerank completed",
		zap.Int("input_count", len(candidates)),
		zap.Int("output_count", len(results)),
		zap.Int("billed_units", cohereResp.Meta.BilledUnits.SearchUnits),
	)

	return results, nil
}

// ============ Fallback Reranker（未配置 Cohere API Key 时使用） ============

// fallbackReranker 降级 Reranker：直接返回原始排序
type fallbackReranker struct{}

// NewFallbackReranker 创建降级 Reranker
func NewFallbackReranker() RerankerService {
	return &fallbackReranker{}
}

// Rerank 直接返回原始排序（跳过重排）
func (r *fallbackReranker) Rerank(ctx context.Context, query string, candidates []HybridResult, topK int) ([]HybridResult, error) {
	if len(candidates) <= topK {
		return candidates, nil
	}
	return candidates[:topK], nil
}
