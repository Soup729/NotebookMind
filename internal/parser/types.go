package parser

// BlockType represents the type of a document block/chunk
type BlockType string

const (
	BlockTypeText     BlockType = "text"      // 普通文本段落
	BlockTypeHeading  BlockType = "heading"   // 标题
	BlockTypeTable    BlockType = "table"     // 表格
	BlockTypeList     BlockType = "list"      // 列表
	BlockTypeImage    BlockType = "image"     // 图片
	BlockTypeCaption  BlockType = "caption"   // 图片/表格说明
	BlockTypeFooter   BlockType = "footer"   // 页脚
	BlockTypeHeader   BlockType = "header"   // 页眉
)

// HeadingLevel 标题层级 (H1=文档标题, H2=章节, H3=小节...)
type HeadingLevel int

const (
	HeadingH1 HeadingLevel = 1
	HeadingH2 HeadingLevel = 2
	HeadingH3 HeadingLevel = 3
	HeadingH4 HeadingLevel = 4
	HeadingH5 HeadingLevel = 5
	HeadingH6 HeadingLevel = 6
)

// BoundingBox 表示页面上的区域坐标 [x0, y0, x1, y1] (左上角到右下角)
type BoundingBox struct {
	X0 float32 `json:"x0"`
	Y0 float32 `json:"y0"`
	X1 float32 `json:"x1"`
	Y1 float32 `json:"y1"`
}

// TableData 表格结构化数据
type TableData struct {
	Headers []string   `json:"headers"`             // 表头
	Rows    [][]string `json:"rows"`                // 数据行
	HTML    string     `json:"html,omitempty"`      // HTML 格式（可选）
	Caption string     `json:"caption,omitempty"`   // 表格标题/说明
}

// ImageData 图片块信息
type ImageData struct {
	PageIndex  int       `json:"page_index"`
	BBox       BoundingBox `json:"bbox"`
	ImageBytes []byte    `json:"-"`                 // 原始图片二进制（不序列化）
	MimeType   string    `json:"mime_type"`         // image/jpeg, image/png
	Caption    string    `json:"caption,omitempty"` // VLM 生成的描述或原始 caption
	Width      int       `json:"width,omitempty"`
	Height     int       `json:"height,omitempty"`
}

// StructuredBlock 结构化文档块 —— 解析输出的最小单元
type StructuredBlock struct {
	ID          string        `json:"id"`                    // 唯一标识
	Type        BlockType     `json:"type"`                  // 块类型
	Content     string        `json:"content"`               // 文本内容
	PageNum     int           `json:"page_num"`              // 所在页码 (从 1 开始)
	BBox        BoundingBox   `json:"bbox,omitempty"`       // 页面坐标
	SectionPath []string      `json:"section_path,omitempty"` // 文档路径: ["第1章", "1.1节"]
	HeadingLevel HeadingLevel `json:"heading_level,omitempty"` // 仅 heading 类型有效
	TableData   *TableData    `json:"table_data,omitempty"`  // 仅 table 类型有效
	ImageData   *ImageData    `json:"image_data,omitempty"`  // 仅 image 类型有效
	Metadata    map[string]string `json:"metadata,omitempty"` // 额外元数据
}

// Chunk 用于向量检索的切片（由 StructuredBlock 聚合生成）
type Chunk struct {
	ID            string            `json:"id"`
	ParentID      string            `json:"parent_id,omitempty"` // 父 chunk ID
	Content       string            `json:"content"`
	DocumentID    string            `json:"document_id"`
	UserID        string            `json:"user_id"`
	PageNum       int               `json:"page_num"`
	ChunkIndex    int               `json:"chunk_index"`
	ChunkType     BlockType         `json:"chunk_type"`          // 主导块类型
	BBox          BoundingBox       `json:"bbox,omitempty"`
	SectionPath   []string          `json:"section_path,omitempty"`
	SourceBlockIDs []string         `json:"source_block_ids"`    // 来源 block ID 列表
	TableHTML     string            `json:"table_html,omitempty"`
	Metadata      map[string]any    `json:"metadata"`
}

// ParseResult 文档解析结果
type ParseResult struct {
	TotalPages   int               `json:"total_pages"`
	Blocks       []StructuredBlock `json:"blocks"`
	RawText      string            `json:"raw_text"`       // 全文纯文本（用于 Guide 生成）
	HasOCR       bool              `json:"has_ocr"`        // 是否使用了 OCR
	TableCount   int               `json:"table_count"`    // 提取到的表格数量
	ImageCount   int               `json:"image_count"`    // 提取到的图片数量
	ParseErrors  []string          `json:"parse_errors,omitempty"` // 解析过程中的非致命错误
}
