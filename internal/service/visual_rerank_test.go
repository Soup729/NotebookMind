package service

import "testing"

func TestPrioritizeEvidenceTypesBoostsVisualMetadataForVisualQueries(t *testing.T) {
	results := []HybridResult{
		{ChunkID: "text", Score: 0.90, Content: "Narrative text", Metadata: map[string]interface{}{"chunk_type": "text"}},
		{ChunkID: "chart", Score: 0.88, Content: "Chart summary", Metadata: map[string]interface{}{"chunk_type": "image", "visual_type": "chart"}},
	}

	prioritized := prioritizeEvidenceTypes("What does the revenue chart show?", results)
	if prioritized[0].ChunkID != "chart" {
		t.Fatalf("expected chart visual evidence first, got %#v", prioritized)
	}
}

func TestExtractVisualMetadataFromContentFallback(t *testing.T) {
	source := NotebookChatSource{
		Content: "[Visual: chart]\nSummary: Revenue grew.\nVisualPath: storage/visual/doc-1/page-2/block-1.png\n",
	}

	path, visualType := source.VisualEvidence()
	if path != "storage/visual/doc-1/page-2/block-1.png" || visualType != "chart" {
		t.Fatalf("expected visual metadata from content, got path=%q type=%q", path, visualType)
	}
}
