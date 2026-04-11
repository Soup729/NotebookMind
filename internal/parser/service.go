package parser

import (
	"context"
)

// ParserService 定义文档解析服务接口
type ParserService interface {
	// ParseDocument 解析 PDF 文档，返回结构化块列表
	ParseDocument(ctx context.Context, filePath string, userID, documentID string) (*ParseResult, error)
	
	// BuildChunks 从结构化块构建父子 chunk（用于向量入库）
	BuildChunks(result *ParseResult, userID, documentID string) ([]*Chunk, []*Chunk)
}

// OCRProvider OCR 服务接口
type OCRProvider interface {
	// RecognizePage 识别单页图片，返回文本
	RecognizePage(ctx context.Context, imageBytes []byte, mimeType string) (string, error)
	// IsAvailable 检查 OCR 服务是否可用
	IsAvailable() bool
}

// VLMProvider 视觉语言模型接口（用于图片描述生成）
type VLMProvider interface {
	// DescribeImage 为图片生成文字描述
	DescribeImage(ctx context.Context, imageBytes []byte, mimeType string, prompt string) (string, error)
	// IsAvailable 检查 VLM 服务是否可用
	IsAvailable() bool
}

// ParserConfig 解析器配置
type ParserConfig struct {
	// 分块配置
	ChunkSize     int `json:"chunk_size"`      // 父块最大字符数，默认 1000
	ChunkOverlap  int `json:"chunk_overlap"`    // 父块重叠字符数，默认 200
	ChildChunkSize int `json:"child_chunk_size"` // 子块（用于召回）最大字符数，默认 300
	
	// 表格配置
	ExtractTables   bool `json:"extract_tables"`    // 是否提取表格，默认 true
	TableMaxRows    int  `json:"table_max_rows"`     // 单表最大行数，默认 100
	TableMaxCols    int  `json:"table_max_cols"`     // 单表最大列数，默认 20
	
	// 图片配置
	ExtractImages   bool  `json:"extract_images"`      // 是否提取图片，默认 true
	ImageMinSize    int   `json:"image_min_size"`       // 最小图片尺寸（像素），默认 50
	VLMEnabled      bool  `json:"vlm_enabled"`          // 是否启用 VLM 描述生成，默认 false（需配置）
	VLMBatchSize    int   `json:"vlm_batch_size"`        // VLM 批量处理大小，默认 5
	
	// OCR 配置
	OCRThreshold    float32 `json:"ocr_threshold"`       // 文本密度阈值，低于此值触发 OCR，默认 0.1
	OCREnabled      bool    `json:"ocr_enabled"`         // 是否启用 OCR 能力，默认 true
	
	// 标题检测配置
	DetectHeadings  bool `json:"detect_headings"`        // 是否检测标题层级，默认 true
	MinHeadingFontSize float32 `json:"min_heading_font_size"` // 最小标题字号比例，默认 1.2
}
