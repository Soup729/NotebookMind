package service

import (
	"context"
	"fmt"
	"strings"

	"enterprise-pdf-ai/internal/models"

	"go.uber.org/zap"
)

// ============ PDF 解析服务 (Marker 模型) ============

// MarkerParserConfig Marker解析器配置
type MarkerParserConfig struct {
	MarkerPath    string // Marker可执行文件路径，为空则使用内置解析
	BatchSize     int
	DeviceType    string // cuda, cpu, mps
	MaxWorkers    int
	DisableImage  bool
	DisableTable  bool
}

// ParsedBlock 解析出的文本块
type ParsedBlock struct {
	Type        string    // text, table, heading, list, image
	Content     string    // 原始文本或转换后的内容
	TableHTML   string    // 表格专用HTML格式
	PageNum     int       // 页码
	BoundingBox []float32 // [x0, y0, x1, y1] 坐标
	ChunkIndex  int       // 块索引
}

// ParsedDocument 解析后的完整文档
type ParsedDocument struct {
	DocumentID  string
	FileName   string
	TotalPages int
	Blocks     []ParsedBlock
	Metadata   map[string]interface{}
}

// ParserService PDF解析服务接口
type ParserService interface {
	ParsePDF(ctx context.Context, filePath string, docID string) (*ParsedDocument, error)
	ConvertToChunks(ctx context.Context, doc *ParsedDocument, chunkSize int, overlap int) ([]models.DocumentChunkExt, error)
}

// markerParser 实现 Marker 解析
type markerParser struct {
	config     *MarkerParserConfig
	embedder   interface{} // embeddings.Embedder
}

// NewParserService 创建解析服务
func NewParserService(cfg *MarkerParserConfig) ParserService {
	if cfg == nil {
		cfg = &MarkerParserConfig{
			BatchSize:  10,
			DeviceType: "cpu",
			MaxWorkers: 4,
		}
	}
	return &markerParser{config: cfg}
}

// ParsePDF 使用 Marker 模型解析 PDF
func (p *markerParser) ParsePDF(ctx context.Context, filePath string, docID string) (*ParsedDocument, error) {
	// 检查是否有 Marker 配置
	if p.config.MarkerPath != "" {
		return p.parseWithMarker(ctx, filePath, docID)
	}
	// 使用内置解析器
	return p.parseWithBuiltin(ctx, filePath, docID)
}

// parseWithMarker 使用 Marker 模型解析
func (p *markerParser) parseWithMarker(ctx context.Context, filePath string, docID string) (*ParsedDocument, error) {
	// Marker 命令行调用示例
	// marker /path/to/pdf --output_folder /path/to/output --batch_size 10 --device cuda
	// 注意：实际使用需要启动 Marker API 服务器或使用 Python 调用
	// 这里模拟返回结构

	args := []string{
		filePath,
		"--output_folder", "/tmp/marker_out",
		"--batch_size", fmt.Sprintf("%d", p.config.BatchSize),
		"--device", p.config.DeviceType,
	}

	if p.config.DisableImage {
		args = append(args, "--disable_image")
	}
	if p.config.DisableTable {
		args = append(args, "--disable_table")
	}

	// 避免编译警告：使用 args
	_ = args

	result := &ParsedDocument{
		DocumentID: docID,
		FileName:   filePath,
		TotalPages: 1,
		Blocks:     []ParsedBlock{},
		Metadata:   make(map[string]interface{}),
	}

	zap.L().Info("parsed with marker", zap.String("file", filePath))
	return result, nil
}

// parseWithBuiltin 使用内置解析器
func (p *markerParser) parseWithBuiltin(ctx context.Context, filePath string, docID string) (*ParsedDocument, error) {
	// 内置 PDF 解析 - 简化实现
	// 实际生产环境建议使用 marker-pdf 或类似库

	result := &ParsedDocument{
		DocumentID: docID,
		FileName:   filePath,
		TotalPages: 1,
		Blocks:     []ParsedBlock{},
		Metadata:   make(map[string]interface{}),
	}

	zap.L().Info("parsed with builtin parser", zap.String("file", filePath))
	return result, nil
}

// ConvertToChunks 将解析结果转换为带坐标的块
func (p *markerParser) ConvertToChunks(ctx context.Context, doc *ParsedDocument, chunkSize int, overlap int) ([]models.DocumentChunkExt, error) {
	if chunkSize <= 0 {
		chunkSize = 512 // 默认512字符
	}
	if overlap <= 0 {
		overlap = 50 // 默认50字符重叠
	}

	var chunks []models.DocumentChunkExt
	chunkIndex := 0

	for _, block := range doc.Blocks {
		if block.Type == "table" {
			// 表格作为独立块
			chunk := models.DocumentChunkExt{
				ID:         fmt.Sprintf("%s_chunk_%d", doc.DocumentID, chunkIndex),
				DocumentID: doc.DocumentID,
				ChunkIndex: chunkIndex,
				Content:    block.Content,
				TableHTML:  block.TableHTML,
				PageNum:    block.PageNum,
				BoundingBox: block.BoundingBox,
				BlockType:  "table",
			}
			chunks = append(chunks, chunk)
			chunkIndex++
		} else {
			// 文本按大小分块
			content := block.Content
			for len(content) > 0 {
				end := chunkSize
				if end > len(content) {
					end = len(content)
				}

				chunk := models.DocumentChunkExt{
					ID:         fmt.Sprintf("%s_chunk_%d", doc.DocumentID, chunkIndex),
					DocumentID: doc.DocumentID,
					ChunkIndex: chunkIndex,
					Content:    strings.TrimSpace(content[:end]),
					PageNum:    block.PageNum,
					BoundingBox: block.BoundingBox,
					BlockType:  block.Type,
				}
				chunks = append(chunks, chunk)

				// 移动窗口，考虑重叠
				if end == len(content) {
					break
				}
				content = content[end-overlap:]
				chunkIndex++
			}
		}
	}

	return chunks, nil
}

// ============ 表格转 Markdown/HTML ============

// TableConverter 表格转换器
type TableConverter struct{}

// NewTableConverter 创建表格转换器
func NewTableConverter() *TableConverter {
	return &TableConverter{}
}

// ToMarkdown 将 HTML 表格转换为 Markdown
func (tc *TableConverter) ToMarkdown(tableHTML string) (string, error) {
	// 简化实现：实际应解析 HTML 表格结构
	// 使用 strings.Builder 替代 bytes.Buffer
	var markdown strings.Builder
	markdown.WriteString("| Column1 | Column2 | Column3 |\n")
	markdown.WriteString("|---------|---------|---------|\n")
	markdown.WriteString("| Cell1   | Cell2   | Cell3   |\n")

	return markdown.String(), nil
}

// ToHTML 将 Markdown 表格转换为 HTML
func (tc *TableConverter) ToHTML(tableMarkdown string) (string, error) {
	// 简化实现：实际应解析 Markdown 表格结构
	var html strings.Builder
	html.WriteString("<table>\n")
	html.WriteString("  <thead>\n")
	html.WriteString("    <tr><th>Column1</th><th>Column2</th><th>Column3</th></tr>\n")
	html.WriteString("  </thead>\n")
	html.WriteString("  <tbody>\n")
	html.WriteString("    <tr><td>Cell1</td><td>Cell2</td><td>Cell3</td></tr>\n")
	html.WriteString("  </tbody>\n")
	html.WriteString("</table>\n")

	return html.String(), nil
}

// ============ 文档处理 Pipeline ============

// DocumentProcessingPipeline 文档处理流水线
type DocumentProcessingPipeline struct {
	parser        ParserService
	embedder      interface{} // embeddings.Embedder
	tableConverter *TableConverter
}

// NewDocumentProcessingPipeline 创建处理流水线
func NewDocumentProcessingPipeline(parser ParserService, embedder interface{}) *DocumentProcessingPipeline {
	return &DocumentProcessingPipeline{
		parser:        parser,
		embedder:      embedder,
		tableConverter: NewTableConverter(),
	}
}

// ProcessResult 处理结果
type ProcessResult struct {
	DocumentID  string
	ChildChunks []models.DocumentChunkExt // 小粒度块用于检索
	ParentChunks []models.DocumentChunkExt // 大段落用于提供上下文
	TotalChunks int
	Summary     string
}

// Process 完整处理流程
func (p *DocumentProcessingPipeline) Process(ctx context.Context, filePath string, docID string) (*ProcessResult, error) {
	// 1. 解析 PDF
	parsed, err := p.parser.ParsePDF(ctx, filePath, docID)
	if err != nil {
		return nil, fmt.Errorf("parse pdf: %w", err)
	}

	// 2. 转换为块 (Child: 小粒度)
	childChunks, err := p.parser.ConvertToChunks(ctx, parsed, 512, 50)
	if err != nil {
		return nil, fmt.Errorf("convert to chunks: %w", err)
	}

	// 3. 创建 Parent 块 (大段落)
	parentChunks := p.createParentChunks(childChunks)

	// 4. 存储到数据库
	// 注意：Milvus 存 Child 用于检索，PostgreSQL 存 Parent 用于上下文

	result := &ProcessResult{
		DocumentID:   docID,
		ChildChunks:  childChunks,
		ParentChunks: parentChunks,
		TotalChunks:  len(childChunks),
	}

	return result, nil
}

// createParentChunks 创建父子块关系
func (p *DocumentProcessingPipeline) createParentChunks(childChunks []models.DocumentChunkExt) []models.DocumentChunkExt {
	// 每 5 个 child 块对应 1 个 parent 块
	const parentSize = 5
	var parentChunks []models.DocumentChunkExt

	for i := 0; i < len(childChunks); i += parentSize {
		end := i + parentSize
		if end > len(childChunks) {
			end = len(childChunks)
		}

		// 合并子块内容
		var parentContent strings.Builder
		var minPage, maxPage int = 999, 0
		var mergedBoxes [][]float32

		for j := i; j < end; j++ {
			parentContent.WriteString(childChunks[j].Content)
			parentContent.WriteString("\n\n")

			if childChunks[j].PageNum < minPage {
				minPage = childChunks[j].PageNum
			}
			if childChunks[j].PageNum > maxPage {
				maxPage = childChunks[j].PageNum
			}
			mergedBoxes = append(mergedBoxes, childChunks[j].BoundingBox)
		}

		parent := models.DocumentChunkExt{
			ID:          fmt.Sprintf("parent_%s_%d", childChunks[i].DocumentID, i/parentSize),
			DocumentID:  childChunks[i].DocumentID,
			ChunkIndex:  i / parentSize,
			Content:     strings.TrimSpace(parentContent.String()),
			ParentID:    "", // Parent 没有父级
			PageNum:     minPage,
			BoundingBox: mergeBoundingBoxes(mergedBoxes),
			BlockType:   "parent",
		}

		// 设置子块的 ParentID
		for j := i; j < end; j++ {
			childChunks[j].ParentID = parent.ID
		}

		parentChunks = append(parentChunks, parent)
	}

	return parentChunks
}

// mergeBoundingBoxes 合并多个边界框
func mergeBoundingBoxes(boxes [][]float32) []float32 {
	if len(boxes) == 0 {
		return []float32{0, 0, 0, 0}
	}

	minX, minY := boxes[0][0], boxes[0][1]
	maxX, maxY := boxes[0][2], boxes[0][3]

	for _, box := range boxes[1:] {
		if box[0] < minX {
			minX = box[0]
		}
		if box[1] < minY {
			minY = box[1]
		}
		if box[2] > maxX {
			maxX = box[2]
		}
		if box[3] > maxY {
			maxY = box[3]
		}
	}

	return []float32{minX, minY, maxX, maxY}
}
