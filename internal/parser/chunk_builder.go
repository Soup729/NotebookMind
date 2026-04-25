package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ChunkBuilder 负责从结构化块构建父子 chunk
type ChunkBuilder struct {
	config *ParserConfig
}

// NewChunkBuilder 创建 ChunkBuilder
func NewChunkBuilder(cfg *ParserConfig) *ChunkBuilder {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &ChunkBuilder{config: cfg}
}

// BuildChunks 将 ParseResult 构建为父子 chunk 对
// 返回: parentChunks（用于上下文拼接）, childChunks（用于向量召回）
func (b *ChunkBuilder) BuildChunks(result *ParseResult, userID, documentID string) ([]*Chunk, []*Chunk) {
	if result == nil || len(result.Blocks) == 0 {
		return nil, nil
	}

	parentChunks := make([]*Chunk, 0)
	childChunks := make([]*Chunk, 0)

	var currentParent strings.Builder
	var parentPageNum int
	var parentType BlockType = BlockTypeText
	var parentBBox BoundingBox
	var parentSectionPath []string
	var sourceBlockIDs []string
	parentIndex := 0
	childIndex := 0

	for _, block := range result.Blocks {
		// 标题块：结束当前父块，标题本身作为独立父块+子块
		if block.Type == BlockTypeHeading {
			if currentParent.Len() > b.config.ChunkSize/2 {
				content := strings.TrimSpace(currentParent.String())
				pChunk := b.createParentChunk(content, userID, documentID,
					parentIndex, parentPageNum, parentType, parentBBox, parentSectionPath, sourceBlockIDs)
				parentChunks = append(parentChunks, pChunk)
				children := b.createChildChunks(content, userID, documentID, pChunk.ID, childIndex, parentPageNum, parentType)
				childChunks = append(childChunks, children...)
				childIndex += len(children)
				parentIndex++
			}

			currentParent.Reset()
			sourceBlockIDs = nil

			// 标题作为独立父块
			titleChunk := &Chunk{
				ID:             uuid.NewString(),
				Content:        block.Content,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     parentIndex,
				ChunkType:      BlockTypeHeading,
				BBox:           block.BBox,
				SectionPath:    block.SectionPath,
				SourceBlockIDs: []string{block.ID},
				Metadata:       map[string]any{"heading_level": fmt.Sprintf("%d", block.HeadingLevel)},
			}
			parentChunks = append(parentChunks, titleChunk)

			// 子块用于召回
			childChunks = append(childChunks, &Chunk{
				ID:             uuid.NewString(),
				ParentID:       titleChunk.ID,
				Content:        block.Content,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     childIndex,
				ChunkType:      BlockTypeHeading,
				BBox:           block.BBox,
				SectionPath:    block.SectionPath,
				SourceBlockIDs: []string{block.ID},
			})
			childIndex++
			parentIndex++

			parentPageNum = block.PageNum
			parentBBox = block.BBox
			parentSectionPath = block.SectionPath
			continue
		}

		// 表格块：作为独立父块 + 子块
		if block.Type == BlockTypeTable {
			if currentParent.Len() > 0 {
				content := strings.TrimSpace(currentParent.String())
				pChunk := b.createParentChunk(content, userID, documentID,
					parentIndex, parentPageNum, parentType, parentBBox, parentSectionPath, sourceBlockIDs)
				parentChunks = append(parentChunks, pChunk)
				children := b.createChildChunks(content, userID, documentID, pChunk.ID, childIndex, parentPageNum, parentType)
				childChunks = append(childChunks, children...)
				childIndex += len(children)
				parentIndex++
				currentParent.Reset()
				sourceBlockIDs = nil
			}

			tableHTML := ""
			rowCount, colCount := 0, 0
			if block.TableData != nil {
				tableHTML = block.TableData.HTML
				rowCount = len(block.TableData.Rows)
				colCount = len(block.TableData.Headers)
			}

			tableParent := &Chunk{
				ID:             uuid.NewString(),
				Content:        block.Content,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     parentIndex,
				ChunkType:      BlockTypeTable,
				BBox:           block.BBox,
				TableHTML:      tableHTML,
				SourceBlockIDs: []string{block.ID},
				Metadata: map[string]any{
					"table_rows":    fmt.Sprintf("%d", rowCount),
					"table_columns": fmt.Sprintf("%d", colCount),
					"chunk_role":    "parent",
				},
			}
			parentChunks = append(parentChunks, tableParent)

			childChunks = append(childChunks, &Chunk{
				ID:             uuid.NewString(),
				ParentID:       tableParent.ID,
				Content:        block.Content,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     childIndex,
				ChunkType:      BlockTypeTable,
				BBox:           block.BBox,
				TableHTML:      tableHTML,
				SourceBlockIDs: []string{block.ID},
				Metadata:       map[string]any{"chunk_role": "child"},
			})
			childIndex++
			parentIndex++

			parentPageNum = block.PageNum
			continue
		}

		// 图片块：生成父+子块（使用 VLM 描述或占位文本）
		if block.Type == BlockTypeImage {
			imageContent, imageMetadata := b.buildVisualChunkContent(block, documentID)

			if strings.TrimSpace(imageContent) == "" {
				continue
			}

			imageParent := &Chunk{
				ID:             uuid.NewString(),
				Content:        imageContent,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     parentIndex,
				ChunkType:      BlockTypeImage,
				BBox:           block.BBox,
				SourceBlockIDs: []string{block.ID},
				Metadata:       imageMetadata.withRole("parent"),
			}
			parentChunks = append(parentChunks, imageParent)

			childChunks = append(childChunks, &Chunk{
				ID:             uuid.NewString(),
				ParentID:       imageParent.ID,
				Content:        imageContent,
				DocumentID:     documentID,
				UserID:         userID,
				PageNum:        block.PageNum,
				ChunkIndex:     childIndex,
				ChunkType:      BlockTypeImage,
				BBox:           block.BBox,
				SourceBlockIDs: []string{block.ID},
				Metadata:       imageMetadata.withRole("child"),
			})
			childIndex++
			parentIndex++
			continue
		}

		// 普通文本 / 列表等：累积到当前父块
		if currentParent.Len() == 0 {
			parentPageNum = block.PageNum
			parentBBox = block.BBox
			parentSectionPath = block.SectionPath
			parentType = block.Type
			sourceBlockIDs = make([]string, 0)
		}

		currentParent.WriteString(block.Content)
		currentParent.WriteString("\n\n")
		sourceBlockIDs = append(sourceBlockIDs, block.ID)

		// 超过 chunk size 时切分
		if currentParent.Len() >= b.config.ChunkSize {
			content := strings.TrimSpace(currentParent.String())
			pChunk := b.createParentChunk(content, userID, documentID,
				parentIndex, parentPageNum, parentType, parentBBox, parentSectionPath, sourceBlockIDs)
			parentChunks = append(parentChunks, pChunk)

			children := b.createChildChunks(content, userID, documentID, pChunk.ID, childIndex, parentPageNum, parentType)
			childChunks = append(childChunks, children...)
			childIndex += len(children)
			parentIndex++
			currentParent.Reset()
			sourceBlockIDs = nil
		}
	}

	// 处理剩余内容
	if currentParent.Len() > 0 {
		content := strings.TrimSpace(currentParent.String())
		pChunk := b.createParentChunk(content, userID, documentID,
			parentIndex, parentPageNum, parentType, parentBBox, parentSectionPath, sourceBlockIDs)
		parentChunks = append(parentChunks, pChunk)

		children := b.createChildChunks(content, userID, documentID, pChunk.ID, childIndex, parentPageNum, parentType)
		childChunks = append(childChunks, children...)
	}

	return parentChunks, childChunks
}

type visualMetadata map[string]any

func (m visualMetadata) withRole(role string) map[string]any {
	out := make(map[string]any, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	out["chunk_role"] = role
	return out
}

func (b *ChunkBuilder) buildVisualChunkContent(block StructuredBlock, documentID string) (string, visualMetadata) {
	metadata := visualMetadata{}
	imageContent := strings.TrimSpace(block.Content)
	visualType := "image"

	if block.ImageData != nil {
		if block.ImageData.VisualType != "" {
			visualType = block.ImageData.VisualType
		}
		if strings.TrimSpace(block.ImageData.Caption) != "" {
			imageContent = strings.TrimSpace(block.ImageData.Caption)
		}
		if b.config.SaveVisualRegions && block.ImageData.VisualPath == "" && len(block.ImageData.ImageBytes) > 0 {
			if path, err := b.persistVisualRegion(documentID, block.ID, block.PageNum, block.ImageData); err == nil {
				block.ImageData.VisualPath = path
			}
		}
		if block.ImageData.VisualPath != "" {
			metadata["visual_path"] = block.ImageData.VisualPath
		}
		if block.ImageData.VisualJSON != "" {
			metadata["visual_json"] = block.ImageData.VisualJSON
		}
	}

	if imageContent == "" {
		imageContent = fmt.Sprintf("Image evidence on page %d.", block.PageNum)
	}
	metadata["image_caption"] = imageContent
	metadata["visual_type"] = visualType

	var builder strings.Builder
	builder.WriteString("[Visual: ")
	builder.WriteString(visualType)
	builder.WriteString("]\n")
	builder.WriteString("Summary: ")
	builder.WriteString(imageContent)
	builder.WriteString("\n")
	if path, ok := metadata["visual_path"].(string); ok && path != "" {
		builder.WriteString("VisualPath: ")
		builder.WriteString(path)
		builder.WriteString("\n")
	}
	if raw, ok := metadata["visual_json"].(string); ok && raw != "" {
		builder.WriteString("VisualJSON: ")
		builder.WriteString(raw)
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String()), metadata
}

func (b *ChunkBuilder) persistVisualRegion(documentID, blockID string, pageNum int, img *ImageData) (string, error) {
	root := strings.TrimSpace(b.config.VisualStorageRoot)
	if root == "" {
		root = "storage/visual"
	}
	ext := ".bin"
	switch strings.ToLower(img.MimeType) {
	case "image/png":
		ext = ".png"
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	}
	if pageNum <= 0 {
		pageNum = img.PageIndex + 1
	}
	safeDocID := safePathSegment(documentID)
	safeBlockID := safePathSegment(blockID)
	dir := filepath.Join(root, safeDocID, fmt.Sprintf("page-%d", pageNum))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("%s%s", safeBlockID, ext))
	if err := os.WriteFile(path, img.ImageBytes, 0644); err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

var unsafePathSegment = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.NewString()
	}
	return unsafePathSegment.ReplaceAllString(value, "_")
}

// createParentChunk 创建父 chunk
func (b *ChunkBuilder) createParentChunk(content, userID, documentID string, index int, pageNum int, chunkType BlockType, bbox BoundingBox, sectionPath []string, sourceBlocks []string) *Chunk {
	return &Chunk{
		ID:             uuid.NewString(),
		Content:        content,
		DocumentID:     documentID,
		UserID:         userID,
		PageNum:        pageNum,
		ChunkIndex:     index,
		ChunkType:      chunkType,
		BBox:           bbox,
		SectionPath:    sectionPath,
		SourceBlockIDs: sourceBlocks,
		Metadata:       map[string]any{"chunk_role": "parent"},
	}
}

// createChildChunks 从父内容创建子 chunks
func (b *ChunkBuilder) createChildChunks(parentContent, userID, documentID, parentID string, startIdx int, pageNum int, chunkType BlockType) []*Chunk {
	runes := []rune(parentContent)
	totalRunes := len(runes)

	if totalRunes <= b.config.ChildChunkSize {
		return []*Chunk{{
			ID:         uuid.NewString(),
			ParentID:   parentID,
			Content:    parentContent,
			DocumentID: documentID,
			UserID:     userID,
			PageNum:    pageNum,
			ChunkIndex: startIdx,
			ChunkType:  chunkType,
			Metadata:   map[string]any{"chunk_role": "child"},
		}}
	}

	children := make([]*Chunk, 0)
	chunkSize := b.config.ChildChunkSize
	overlap := chunkSize / 6

	for offset := 0; offset < totalRunes; {
		end := offset + chunkSize
		if end > totalRunes {
			end = totalRunes
		}

		content := string(runes[offset:end])
		content = strings.TrimSpace(content)
		if content != "" {
			children = append(children, &Chunk{
				ID:         uuid.NewString(),
				ParentID:   parentID,
				Content:    content,
				DocumentID: documentID,
				UserID:     userID,
				PageNum:    pageNum,
				ChunkIndex: startIdx + len(children),
				ChunkType:  chunkType,
				Metadata:   map[string]any{"chunk_role": "child"},
			})
		}

		offset += (chunkSize - overlap)
		if offset < 0 || offset >= totalRunes {
			break
		}
	}

	return children
}

// ToMetadata 将 parent chunk 转为标准化的 metadata map（供向量库使用）
func (c *Chunk) ToMetadata() map[string]any {
	metadata := map[string]any{
		"user_id":          c.UserID,
		"document_id":      c.DocumentID,
		"page_num":         c.PageNum,
		"chunk_index":      c.ChunkIndex,
		"chunk_type":       string(c.ChunkType),
		"source_block_ids": c.SourceBlockIDs,
	}
	if c.ParentID != "" {
		metadata["parent_id"] = c.ParentID
		metadata["chunk_role"] = "child"
	} else {
		metadata["chunk_role"] = "parent"
	}
	if len(c.SectionPath) > 0 {
		metadata["section_path"] = c.SectionPath
	}
	if c.TableHTML != "" {
		metadata["table_html"] = c.TableHTML
	}
	if c.BBox != (BoundingBox{}) {
		metadata["bbox"] = []float32{c.BBox.X0, c.BBox.Y0, c.BBox.X1, c.BBox.Y1}
	}
	for k, v := range c.Metadata {
		if _, exists := metadata[k]; !exists {
			metadata[k] = v
		}
	}
	return metadata
}
