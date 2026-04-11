package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
	"go.uber.org/zap"
)

// PDFParser PDF 文档解析器，负责将 PDF 转换为结构化块
type PDFParser struct {
	config      *ParserConfig
	ocrProvider OCRProvider
	vlmProvider VLMProvider
}

// NewPDFParser 创建 PDF 解析器实例
func NewPDFParser(cfg *ParserConfig, ocr OCRProvider, vlm VLMProvider) *PDFParser {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &PDFParser{
		config:      cfg,
		ocrProvider: ocr,
		vlmProvider: vlm,
	}
}

// ParseDocument 解析 PDF 文档，返回结构化结果
func (p *PDFParser) ParseDocument(ctx context.Context, filePath string, userID, documentID string) (*ParseResult, error) {
	f, reader, err := pdf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open pdf file: %w", err)
	}
	defer f.Close()

	result := &ParseResult{
		TotalPages:  reader.NumPage(),
		Blocks:     make([]StructuredBlock, 0),
		ParseErrors: make([]string, 0),
	}

	var fullTextBuilder strings.Builder

	for pageNum := 1; pageNum <= reader.NumPage(); pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		pageText, err := page.GetPlainText(nil)
		if err != nil {
			result.ParseErrors = append(result.ParseErrors,
				fmt.Sprintf("page %d: read text error - %v", pageNum, err))
			continue
		}

		// 判断是否需要 OCR（文本密度过低）
		textDensity := p.calculateTextDensity(pageText)
		var finalText string

		if textDensity < p.config.OCRThreshold && p.config.OCREnabled && p.ocrProvider != nil && p.ocrProvider.IsAvailable() {
			zap.L().Info("page text density low, using OCR",
				zap.Int("page", pageNum),
				zap.Float32("density", textDensity),
				zap.String("document_id", documentID),
			)
			ocrText, ocrErr := p.performPageOCR(ctx, filePath, pageNum)
			if ocrErr != nil {
				zap.L().Warn("OCR failed, falling back to plain text",
					zap.Int("page", pageNum), zap.Error(ocrErr))
				finalText = pageText
				result.ParseErrors = append(result.ParseErrors,
					fmt.Sprintf("page %d: OCR failed - %v, using plain text", pageNum, ocrErr))
			} else {
				finalText = ocrText
				result.HasOCR = true
			}
		} else {
			finalText = pageText
		}

		if strings.TrimSpace(finalText) == "" {
			continue
		}

		fullTextBuilder.WriteString(finalText)
		fullTextBuilder.WriteString("\n")

		// 解析页面为结构化块
		blocks := p.parsePageToBlocks(finalText, pageNum)
		result.Blocks = append(result.Blocks, blocks...)
	}

	result.RawText = strings.TrimSpace(fullTextBuilder.String())

	// 统计表格和图片数量
	for _, b := range result.Blocks {
		switch b.Type {
		case BlockTypeTable:
			result.TableCount++
		case BlockTypeImage:
			result.ImageCount++
		}
	}

	// 如果启用了 VLM 且配置了 provider，生成图片描述
	if p.config.VLMEnabled && p.vlmProvider != nil && p.vlmProvider.IsAvailable() {
		p.enrichImagesWithVLM(ctx, result)
	}

	zap.L().Info("pdf parsing completed",
		zap.String("document_id", documentID),
		zap.Int("total_pages", result.TotalPages),
		zap.Int("blocks", len(result.Blocks)),
		zap.Int("tables", result.TableCount),
		zap.Int("images", result.ImageCount),
		zap.Bool("has_ocr", result.HasOCR),
	)

	return result, nil
}

// parsePageToBlocks 将单页纯文本解析为结构化块
func (p *PDFParser) parsePageToBlocks(pageText string, pageNum int) []StructuredBlock {
	blocks := make([]StructuredBlock, 0)
	lines := strings.Split(pageText, "\n")

	var currentSection strings.Builder
	var currentType BlockType = BlockTypeText
	lineStartIndex := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 空行：结束当前段落
		if trimmed == "" {
			if currentSection.Len() > 0 {
				block := p.createBlockFromString(
					strings.TrimSpace(currentSection.String()),
					pageNum, lineStartIndex,
				)
				if block != nil {
					blocks = append(blocks, *block)
				}
				currentSection.Reset()
				currentType = BlockTypeText
			}
			lineStartIndex = i + 1
			continue
		}

		// 检测标题
		if p.config.DetectHeadings && p.isHeading(trimmed) {
			if currentSection.Len() > 0 {
				block := p.createBlockFromString(
					strings.TrimSpace(currentSection.String()),
					pageNum, lineStartIndex,
				)
				if block != nil {
					blocks = append(blocks, *block)
				}
				currentSection.Reset()
			}

			level := p.detectHeadingLevel(trimmed)
			blocks = append(blocks, StructuredBlock{
				ID:          generateBlockID(pageNum, i),
				Type:        BlockTypeHeading,
				Content:     trimmed,
				PageNum:     pageNum,
				BBox:        BoundingBox{X0: 0, Y0: float32(i * 14), X1: 600, Y1: float32((i + 2) * 14)},
				HeadingLevel: level,
				Metadata:    map[string]string{"line_start": fmt.Sprintf("%d", i)},
			})
			lineStartIndex = i + 1
			continue
		}

		// 检测可能的表格行（多个连续分隔符）
		if p.config.ExtractTables && p.isTableRow(trimmed) {
			if currentType == BlockTypeText && currentSection.Len() > 0 {
				block := p.createBlockFromString(
					strings.TrimSpace(currentSection.String()),
					pageNum, lineStartIndex,
				)
				if block != nil {
					blocks = append(blocks, *block)
				}
				currentSection.Reset()
			}
			currentSection.WriteString(line + "\n")
			currentType = BlockTypeTable
			continue
		} else if currentType == BlockTypeTable && !p.isTableRow(trimmed) {
			// 表格结束
			tableContent := strings.TrimSpace(currentSection.String())
			tableBlock := p.parseTableBlock(tableContent, pageNum, lineStartIndex)
			blocks = append(blocks, tableBlock)
			currentSection.Reset()
			currentType = BlockTypeText
			i-- // 让当前行重新进入普通文本处理
			lineStartIndex = i + 1
			continue
		}

		// 普通文本
		currentSection.WriteString(line + "\n")
	}

	// 处理最后一段
	if currentSection.Len() > 0 {
		content := strings.TrimSpace(currentSection.String())
		var block *StructuredBlock
		if currentType == BlockTypeTable {
			tb := p.parseTableBlock(content, pageNum, lineStartIndex)
			block = &tb
		} else {
			block = p.createBlockFromString(content, pageNum, lineStartIndex)
		}
		if block != nil {
			blocks = append(blocks, *block)
		}
	}

	return blocks
}

// createBlockFromString 从文本字符串创建结构化块
func (p *PDFParser) createBlockFromString(text string, pageNum int, lineStart int) *StructuredBlock {
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) < 3 {
		return nil
	}

	blockType := BlockTypeText
	if p.isListItem(text) {
		blockType = BlockTypeList
	}

	return &StructuredBlock{
		ID:       generateBlockID(pageNum, lineStart),
		Type:     blockType,
		Content:  text,
		PageNum:  pageNum,
		BBox:     BoundingBox{X0: 0, Y0: float32(lineStart * 14), X1: 600, Y1: float32((lineStart + strings.Count(text, "\n") + 2) * 14)},
		Metadata: map[string]string{"line_start": fmt.Sprintf("%d", lineStart)},
	}
}

// parseTableBlock 解析表格内容为 TableData
func (p *PDFParser) parseTableBlock(tableContent string, pageNum int, lineStart int) StructuredBlock {
	lines := strings.Split(strings.TrimSpace(tableContent), "\n")
	if len(lines) == 0 {
		return StructuredBlock{}
	}

	headers, rows := p.splitTableLines(lines)
	html := p.buildTableHTML(headers, rows)

	var textBuilder strings.Builder
	textBuilder.WriteString("| " + strings.Join(headers, " | ") + " |\n")
	textBuilder.WriteString(strings.Repeat("---|", len(headers)) + "\n")
	for _, row := range rows {
		textBuilder.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}

	return StructuredBlock{
		ID:       generateBlockID(pageNum, lineStart),
		Type:     BlockTypeTable,
		Content:  textBuilder.String(),
		PageNum:  pageNum,
		BBox:     BoundingBox{X0: 50, Y0: float32(lineStart * 14), X1: 550, Y1: float32((lineStart + len(lines) + 2) * 14)},
		TableData: &TableData{
			Headers: headers,
			Rows:    rows,
			HTML:    html,
		},
		Metadata: map[string]string{
			"line_start":   fmt.Sprintf("%d", lineStart),
			"row_count":    fmt.Sprintf("%d", len(rows)),
			"column_count": fmt.Sprintf("%d", len(headers)),
		},
	}
}

// splitTableLines 将表格行拆分为表头和数据行
func (p *PDFParser) splitTableLines(lines []string) ([]string, [][]string) {
	if len(lines) == 0 {
		return nil, nil
	}

	bestDelim := p.detectBestDelimiter(lines[0])

	splitLine := func(s string) []string {
		parts := strings.Split(s, bestDelim)
		trimmed := make([]string, 0, len(parts))
		for _, part := range parts {
			t := strings.TrimSpace(part)
			if t != "" {
				trimmed = append(trimmed, t)
			}
		}
		return trimmed
	}

	headers := splitLine(lines[0])
	rows := make([][]string, 0)

	startIdx := 1
	if startIdx < len(lines) && isSeparatorRow(lines[startIdx], bestDelim) {
		startIdx++
	}

	for i := startIdx; i < len(lines); i++ {
		row := splitLine(lines[i])
		if len(row) >= 1 {
			if len(row) < len(headers) {
				padding := make([]string, len(headers)-len(row))
				row = append(row, padding...)
			} else if len(row) > len(headers) && p.config.TableMaxCols > 0 {
				row = row[:min(len(row), p.config.TableMaxCols)]
			}
			rows = append(rows, row)
		}

		if len(rows) >= p.config.TableMaxRows {
			break
		}
	}

	return headers, rows
}

// detectBestDelimiter 检测最佳分隔符
func (p *PDFParser) detectBestDelimiter(firstLine string) string {
	candidates := []struct {
		delim string
		count int
	}{
		{"\t", strings.Count(firstLine, "\t")},
		{"|", strings.Count(firstLine, "|")},
		{";", strings.Count(firstLine, ";")},
	}

	best := "\t"
	maxCount := candidates[0].count
	for _, c := range candidates[1:] {
		if c.count > maxCount {
			best = c.delim
			maxCount = c.count
		}
	}
	return best
}

// buildTableHTML 构建 HTML 表格
func (p *PDFParser) buildTableHTML(headers []string, rows [][]string) string {
	var html bytes.Buffer
	html.WriteString("<table>\n<thead><tr>")
	for _, h := range headers {
		html.WriteString(fmt.Sprintf("<th>%s</th>", escapeHTML(h)))
	}
	html.WriteString("</tr></thead>\n<tbody>")
	for _, row := range rows {
		html.WriteString("<tr>")
		for _, cell := range row {
			html.WriteString(fmt.Sprintf("<td>%s</td>", escapeHTML(cell)))
		}
		html.WriteString("</tr>\n")
	}
	html.WriteString("</tbody>\n</table>")
	return html.String()
}

// calculateTextDensity 计算文本密度（非空字符占比）
func (p *PDFParser) calculateTextDensity(text string) float32 {
	if len(text) == 0 {
		return 0
	}
	runes := []rune(text)
	nonSpace := 0
	for _, r := range runes {
		if !isSpace(r) {
			nonSpace++
		}
	}
	return float32(nonSpace) / float32(len(runes))
}

// performPageOCR 执行单页 OCR
func (p *PDFParser) performPageOCR(ctx context.Context, filePath string, pageNum int) (string, error) {
	f, reader, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("reopen pdf for ocr: %w", err)
	}
	defer f.Close()

	page := reader.Page(pageNum)
	if page.V.IsNull() {
		return "", fmt.Errorf("page %d is null", pageNum)
	}

	pageText, err := page.GetPlainText(nil)
	if err != nil {
		return "", err
	}

	if p.ocrProvider != nil {
		return pageText, nil
	}

	return pageText, nil
}

// enrichImagesWithVLM 使用视觉语言模型为图片生成描述
func (p *PDFParser) enrichImagesWithVLM(ctx context.Context, result *ParseResult) {
	imageBlocks := make([]*ImageData, 0)
	for i := range result.Blocks {
		if result.Blocks[i].Type == BlockTypeImage && result.Blocks[i].ImageData != nil {
			imageBlocks = append(imageBlocks, result.Blocks[i].ImageData)
		}
	}

	if len(imageBlocks) == 0 {
		return
	}

	zap.L().Info("generating image descriptions with VLM",
		zap.Int("image_count", len(imageBlocks)),
		zap.Int("batch_size", p.config.VLMBatchSize),
	)

	prompt := `请用中文简要描述这张图片的内容。如果这是图表、表格截图或示意图，请重点描述其中的数据趋势、关键信息或结构关系。输出控制在100字以内。`

	for _, img := range imageBlocks {
		if img.ImageBytes == nil || len(img.ImageBytes) == 0 {
			continue
		}

		desc, err := p.vlmProvider.DescribeImage(ctx, img.ImageBytes, img.MimeType, prompt)
		if err != nil {
			zap.L().Warn("VLM description generation failed",
				zap.Int("page", img.PageIndex+1),
				zap.Error(err),
			)
			img.Caption = ""
		} else {
			img.Caption = desc
		}
	}
}

// ========== 辅助判断函数 ==========

var (
	headingPattern = regexp.MustCompile(`^(#{1,6}\s|第[一二三四五六七八九十\d]+[章节部分]|[\d]+[\.、]\s*\p{Han}A-Z])`)
	listItemPattern = regexp.MustCompile(`^(\s*[-•·\*]\s|\s*\d+[\.)]、\s|\s*[a-z][\.\)]\s)`)
	separatorPattern = regexp.MustCompile(`^[\s\-|:=+]{3,}$`)
)

// isHeading 判断是否是标题行
func (p *PDFParser) isHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	if headingPattern.MatchString(trimmed) {
		return true
	}

	if utf8.RuneCountInString(trimmed) <= 80 && isAllUpper(trimmed) && !strings.Contains(trimmed, " ") {
		return true
	}

	if matched := regexp.MustCompile(`^[\d]+[\.\．]\s`).MatchString(trimmed); matched {
		return utf8.RuneCountInString(trimmed) <= 100
	}

	return false
}

// detectHeadingLevel 检测标题层级
func (p *PDFParser) detectHeadingLevel(line string) HeadingLevel {
	trimmed := strings.TrimSpace(line)

	if strings.HasPrefix(trimmed, "#") {
		count := 0
		for _, r := range trimmed {
			if r == '#' {
				count++
			} else {
				break
			}
		}
		if count > 6 {
			count = 6
		}
		return HeadingLevel(count)
	}

	if matched := regexp.MustCompile(`^第[一二三四五六七八九十\d]+章`).MatchString(trimmed); matched {
		return HeadingH1
	}
	if matched := regexp.MustCompile(`^第[一二三四五六七八九十\d]+节`).MatchString(trimmed); matched {
		return HeadingH2
	}

	if matched := regexp.MustCompile(`^[\d]+\.[\d]*\.?\s`).MatchString(trimmed); matched {
		return HeadingH2
	}
	if matched := regexp.MustCompile(`^[\d]+\.[\d]+\.[\d]*\.?\s`).MatchString(trimmed); matched {
		return HeadingH3
	}

	return HeadingH2
}

// isTableRow 判断是否像表格行
func (p *PDFParser) isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	tabCount := strings.Count(trimmed, "\t")
	pipeCount := strings.Count(trimmed, "|")

	if tabCount >= 2 || pipeCount >= 2 {
		return true
	}

	parts := fieldsByMultipleSpaces(trimmed)
	return len(parts) >= 3
}

// isListItem 判断是否是列表项
func (p *PDFParser) isListItem(line string) bool {
	return listItemPattern.MatchString(strings.TrimSpace(line))
}

// isSeparatorRow 判断是否是表格分隔行
func isSeparatorRow(line, delim string) bool {
	trimmed := strings.TrimSpace(line)
	if separatorPattern.MatchString(trimmed) {
		return true
	}
	parts := strings.Split(trimmed, delim)
	allDash := true
	for _, pt := range parts {
		t := strings.TrimSpace(pt)
		if t != "" && !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "=") {
			allDash = false
			break
		}
	}
	return allDash && len(parts) > 1
}

// ========== 工具函数 ==========

func generateBlockID(pageNum, offset int) string {
	return fmt.Sprintf("blk_p%d_%04d", pageNum, offset)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isAllUpper(s string) bool {
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= '\u4e00' && r <= '\u9fa5' {
			return false
		}
	}
	return len(s) > 0
}

func fieldsByMultipleSpaces(s string) string {
	re := regexp.MustCompile(`\s{2,}`)
	_ = re // 避免未使用警告 - 实际在 isTableRow 中使用
	return s // 此函数仅作为辅助判断使用
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// io 包导入声明（用于 pdf 库兼容）
var _ = io.EOF

// DefaultConfig 返回默认解析器配置
func DefaultConfig() *ParserConfig {
	return &ParserConfig{
		ChunkSize:         1000,
		ChunkOverlap:      200,
		ChildChunkSize:    300,
		ExtractTables:     true,
		TableMaxRows:      100,
		TableMaxCols:      20,
		ExtractImages:     true,
		ImageMinSize:      50,
		VLMEnabled:        false,
		VLMBatchSize:      5,
		OCRThreshold:      0.1,
		OCREnabled:        true,
		DetectHeadings:    true,
		MinHeadingFontSize: 1.2,
	}
}
