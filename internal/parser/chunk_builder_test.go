package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChunkBuilderPersistsVisualRegionMetadata(t *testing.T) {
	root := t.TempDir()
	builder := NewChunkBuilder(&ParserConfig{
		ChunkSize:         1000,
		ChildChunkSize:    300,
		SaveVisualRegions: true,
		VisualStorageRoot: root,
	})

	result := &ParseResult{Blocks: []StructuredBlock{
		{
			ID:      "block-1",
			Type:    BlockTypeImage,
			Content: "Revenue chart",
			PageNum: 2,
			BBox:    BoundingBox{X0: 10, Y0: 20, X1: 200, Y1: 180},
			ImageData: &ImageData{
				PageIndex:  1,
				BBox:       BoundingBox{X0: 10, Y0: 20, X1: 200, Y1: 180},
				ImageBytes: []byte("fake image bytes"),
				MimeType:   "image/png",
				Caption:    "Revenue chart shows 2024 growth.",
				VisualType: "chart",
				VisualJSON: `{"visual_type":"chart","summary":"growth"}`,
			},
		},
	}}

	parents, children := builder.BuildChunks(result, "user-1", "doc-1")
	if len(parents) != 1 || len(children) != 1 {
		t.Fatalf("expected one parent and one child, got %d/%d", len(parents), len(children))
	}

	parent := parents[0]
	path, _ := parent.Metadata["visual_path"].(string)
	if path == "" {
		t.Fatalf("expected visual_path metadata, got %#v", parent.Metadata)
	}
	if !strings.Contains(parent.Content, "[Visual: chart]") || !strings.Contains(parent.Content, "VisualPath:") {
		t.Fatalf("expected visual evidence markers in content, got %q", parent.Content)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("expected visual region file to exist at %q: %v", path, err)
	}
	if children[0].Metadata["visual_type"] != "chart" {
		t.Fatalf("expected child visual metadata, got %#v", children[0].Metadata)
	}
}

func TestChunkBuilderPropagatesBoundingBoxToChildren(t *testing.T) {
	builder := NewChunkBuilder(&ParserConfig{
		ChunkSize:      1000,
		ChildChunkSize: 300,
	})

	result := &ParseResult{Blocks: []StructuredBlock{
		{
			ID:          "block-1",
			Type:        BlockTypeText,
			Content:     "First paragraph about retrieval evidence.",
			PageNum:     1,
			BBox:        BoundingBox{X0: 10, Y0: 20, X1: 300, Y1: 60},
			SectionPath: []string{"Section A"},
		},
		{
			ID:          "block-2",
			Type:        BlockTypeText,
			Content:     "Second paragraph that should expand the parent region.",
			PageNum:     1,
			BBox:        BoundingBox{X0: 12, Y0: 80, X1: 420, Y1: 130},
			SectionPath: []string{"Section A"},
		},
	}}

	parents, children := builder.BuildChunks(result, "user-1", "doc-1")
	if len(parents) != 1 || len(children) != 1 {
		t.Fatalf("expected one parent and one child, got %d/%d", len(parents), len(children))
	}

	want := BoundingBox{X0: 10, Y0: 20, X1: 420, Y1: 130}
	if parents[0].BBox != want {
		t.Fatalf("expected parent union bbox %#v, got %#v", want, parents[0].BBox)
	}
	if children[0].BBox != want {
		t.Fatalf("expected child to inherit parent bbox %#v, got %#v", want, children[0].BBox)
	}
	if len(children[0].SourceBlockIDs) != 2 {
		t.Fatalf("expected child source blocks to be propagated, got %#v", children[0].SourceBlockIDs)
	}
}
