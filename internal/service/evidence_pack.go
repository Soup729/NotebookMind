package service

import (
	"crypto/sha1"
	"fmt"
	"strings"
)

type EvidenceItem struct {
	ID           string
	DocumentID   string
	DocumentName string
	PageNumber   int64
	ChunkID      string
	ChunkType    string
	SectionPath  []string
	BoundingBox  []float32
	Content      string
}

type EvidencePack struct {
	Items []EvidenceItem
}

func BuildEvidencePack(results []HybridResult, sources []NotebookChatSource, plan TrustPlan) EvidencePack {
	sourceByDoc := make(map[string]NotebookChatSource, len(sources))
	for _, source := range sources {
		if source.DocumentID != "" {
			sourceByDoc[source.DocumentID] = source
		}
	}

	items := make([]EvidenceItem, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		content := strings.TrimSpace(result.Content)
		if content == "" {
			continue
		}
		page := metadataInt64(result.Metadata, "page_number")
		key := evidenceDedupeKey(result.DocumentID, page, content)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		source := sourceByDoc[result.DocumentID]
		docName := source.DocumentName
		if docName == "" {
			docName = "Unknown Document"
		}
		item := EvidenceItem{
			ID:           fmt.Sprintf("E%d", len(items)+1),
			DocumentID:   result.DocumentID,
			DocumentName: docName,
			PageNumber:   page,
			ChunkID:      result.ChunkID,
			ChunkType:    metadataStringAny(result.Metadata, "chunk_type"),
			SectionPath:  metadataStringSlice(result.Metadata, "section_path"),
			BoundingBox:  metadataFloat32Slice(result.Metadata, "bbox"),
			Content:      content,
		}
		if item.ChunkType == "" {
			item.ChunkType = source.ChunkType
		}
		if len(item.SectionPath) == 0 {
			item.SectionPath = source.SectionPath
		}
		if len(item.BoundingBox) == 0 {
			item.BoundingBox = source.BoundingBox
		}
		items = append(items, item)
	}
	return EvidencePack{Items: items}
}

func (p EvidencePack) FormatForPrompt() string {
	var builder strings.Builder
	for _, item := range p.Items {
		builder.WriteString(fmt.Sprintf("[%s] [Source: %s, Page %d]\n", item.ID, item.DocumentName, item.PageNumber+1))
		if item.ChunkType != "" {
			builder.WriteString("Type: ")
			builder.WriteString(item.ChunkType)
			builder.WriteString("\n")
		}
		if len(item.SectionPath) > 0 {
			builder.WriteString("Section: ")
			builder.WriteString(strings.Join(item.SectionPath, " > "))
			builder.WriteString("\n")
		}
		builder.WriteString("Content:\n")
		builder.WriteString(item.Content)
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String())
}

func (p EvidencePack) SourceByID(id string) (EvidenceItem, bool) {
	for _, item := range p.Items {
		if item.ID == id {
			return item, true
		}
	}
	return EvidenceItem{}, false
}

func evidenceDedupeKey(documentID string, page int64, content string) string {
	sum := sha1.Sum([]byte(strings.Join(strings.Fields(content), " ")))
	return fmt.Sprintf("%s:%d:%x", documentID, page, sum)
}

func metadataInt64(metadata map[string]interface{}, key string) int64 {
	if metadata == nil {
		return 0
	}
	switch v := metadata[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}
