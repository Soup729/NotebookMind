package service

import (
	"testing"

	"NotebookAI/internal/parser"
)

func TestExtractAndAggregateGraphItems(t *testing.T) {
	chunks := []*parser.Chunk{
		{
			ID:          "chunk-1",
			DocumentID:  "doc-1",
			Content:     "Transformer uses Self-Attention to improve translation quality. The BLEU Score is reported in the table.",
			PageNum:     2,
			ChunkIndex:  0,
			ChunkType:   parser.BlockTypeText,
			SectionPath: []string{"Architecture", "Transformer"},
			Metadata:    map[string]any{"chunk_role": "parent"},
		},
		{
			ID:          "chunk-2",
			DocumentID:  "doc-1",
			Content:     "Self-Attention is compared with recurrent neural networks.",
			PageNum:     3,
			ChunkIndex:  1,
			ChunkType:   parser.BlockTypeText,
			SectionPath: []string{"Architecture", "Self-Attention"},
			Metadata:    map[string]any{"chunk_role": "parent"},
		},
	}

	items := extractGraphItems("nb-1", "doc-1", chunks)
	if len(items) == 0 {
		t.Fatal("expected graph items")
	}

	nodes, edges, stats := aggregateGraphItems(items, map[string]string{"doc-1": "paper.pdf"})
	if stats.Entities == 0 {
		t.Fatalf("expected entities, got %#v", nodes)
	}
	if stats.Relations == 0 {
		t.Fatalf("expected relations, got %#v", edges)
	}

	var foundTransformer bool
	for _, node := range nodes {
		if node.Label == "Transformer" {
			foundTransformer = true
			if node.Documents[0].Name != "paper.pdf" {
				t.Fatalf("expected document name to be preserved, got %#v", node.Documents)
			}
		}
	}
	if !foundTransformer {
		t.Fatalf("expected Transformer node, got %#v", nodes)
	}
}
