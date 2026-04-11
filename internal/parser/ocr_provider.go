package parser

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// RapidOCRProvider 基于 RapidOCR HTTP 服务的 OCR 实现
type RapidOCRProvider struct {
	BaseURL    string
	HTTPClient *http.Client
	available  bool
}

// NewRapidOCRProvider 创建 RapidOCR 客户端
func NewRapidOCRProvider(baseURL string) *RapidOCRProvider {
	if baseURL == "" {
		return &RapidOCRProvider{available: false}
	}
	return &RapidOCRProvider{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 0, // 使用默认超时
		},
		available: true,
	}
}

// IsAvailable 检查 OCR 服务是否可用
func (r *RapidOCRProvider) IsAvailable() bool {
	return r.available
}

// RecognizePage 调用 RapidOCR 服务识别图片中的文字
func (r *RapidOCRProvider) RecognizePage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	if !r.available {
		return "", fmt.Errorf("RapidOCR service is not configured")
	}

	url := r.BaseURL + "/ocr"

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("create ocr request: %w", err)
	}
	req.Header.Set("Content-Type", mimeType)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "image/jpeg")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		r.available = false
		return "", fmt.Errorf("rapidocr request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rapidocr returned status %d", resp.StatusCode)
	}

	zap.L().Debug("rapidocr recognition completed",
		zap.Int("status", resp.StatusCode),
	)

	// 框架预留：实际集成需根据 RapidOCR API 响应格式解码 JSON
	// 当前返回空字符串，由调用方降级处理
	return "", fmt.Errorf("rapidocr integration requires response body parsing - framework placeholder")
}

// FallbackOCRProvider 降级 OCR 提供者（不执行实际 OCR）
type FallbackOCRProvider struct{}

// NewFallbackOCRProvider 创建降级 OCR 提供者
func NewFallbackOCRProvider() *FallbackOCRProvider {
	return &FallbackOCRProvider{}
}

// IsAvailable 总是返回 true（作为降级方案始终可用）
func (f *FallbackOCRProvider) IsAvailable() bool {
	return true
}

// RecognizePage 返回空（表示无法增强文本）
func (f *FallbackOCRProvider) RecognizePage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	return "", fmt.Errorf("fallback OCR: no OCR capability available, using plain text extraction")
}
