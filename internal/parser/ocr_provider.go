package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

// rapidOCRResponse RapidOCR HTTP 服务的标准响应格式
type rapidOCRResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    []rapidOCRResultItem   `json:"data,omitempty"`
	Result  []rapidOCRResultItem   `json:"result,omitempty"` // 兼容不同版本
}

type rapidOCRResultItem struct {
	Text         string    `json:"text"`
	Box          []float64 `json:"box,omitempty"`
	Confidence   float64   `json:"confidence,omitempty"`
}

// RecognizePage 调用 RapidOCR 服务识别图片中的文字
func (r *RapidOCRProvider) RecognizePage(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	if !r.available {
		return "", fmt.Errorf("RapidOCR service is not configured")
	}

	// 构建 multipart/form-data 请求
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 写入图片文件字段
	part, err := writer.CreateFormFile("file", "page."+fileExtFromMime(mimeType))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		return "", fmt.Errorf("write image bytes: %w", err)
	}
	writer.Close()

	url := r.BaseURL + "/ocr"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("create ocr request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		r.available = false
		return "", fmt.Errorf("rapidocr request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read rapidocr response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rapidocr returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// 解析响应
	var ocrResp rapidOCRResponse
	if err := json.Unmarshal(respBody, &ocrResp); err != nil {
		return "", fmt.Errorf("parse rapidocr response: %w", err)
	}

	// 提取文本（兼容 data 和 result 两种字段名）
	items := ocrResp.Data
	if len(items) == 0 {
		items = ocrResp.Result
	}

	var textBuilder strings.Builder
	for i, item := range items {
		if i > 0 {
			textBuilder.WriteString("\n")
		}
		textBuilder.WriteString(item.Text)
	}

	result := strings.TrimSpace(textBuilder.String())
	zap.L().Debug("rapidocr recognition completed",
		zap.Int("status", resp.StatusCode),
		zap.Int("text_items", len(items)),
		zap.Int("result_len", len(result)),
	)

	return result, nil
}

// fileExtFromMime 根据 MIME 类型返回文件扩展名
func fileExtFromMime(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "jpg"
	}
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
