package service

import (
	"context"
	"strings"
	"testing"

	"NotebookAI/internal/models"
)

type fakeDocumentRepoForSources struct{}

func (f *fakeDocumentRepoForSources) Create(context.Context, *models.Document) error { return nil }
func (f *fakeDocumentRepoForSources) GetByID(context.Context, string, string) (*models.Document, error) {
	return nil, nil
}
func (f *fakeDocumentRepoForSources) GetByIDForWorker(context.Context, string) (*models.Document, error) {
	return nil, nil
}
func (f *fakeDocumentRepoForSources) GetNamesByIDs(_ context.Context, _ string, docIDs []string) map[string]string {
	names := make(map[string]string, len(docIDs))
	for _, id := range docIDs {
		names[id] = "Annual_Report_2024.pdf"
	}
	return names
}
func (f *fakeDocumentRepoForSources) ListByUser(context.Context, string) ([]models.Document, error) {
	return nil, nil
}
func (f *fakeDocumentRepoForSources) UpdateProcessingResult(context.Context, string, string, int, string) error {
	return nil
}
func (f *fakeDocumentRepoForSources) DeleteByID(context.Context, string, string) error { return nil }
func (f *fakeDocumentRepoForSources) CountByUser(context.Context, string) (int64, error) {
	return 0, nil
}
func (f *fakeDocumentRepoForSources) CountCompletedByUser(context.Context, string) (int64, error) {
	return 0, nil
}

func TestHybridResultsToNotebookSourcesPreservesStructuredMetadata(t *testing.T) {
	svc := &notebookChatService{docRepo: &fakeDocumentRepoForSources{}}
	results := []HybridResult{
		{
			DocumentID: "doc-1",
			Content:    "A cited paragraph.",
			Score:      0.87,
			Metadata: map[string]interface{}{
				"page_number":  2,
				"chunk_index":  7,
				"bbox":         []float32{10, 20, 110, 220},
				"section_path": []string{"Management Discussion", "Revenue"},
				"chunk_type":   "table",
			},
		},
	}

	sources := hybridResultsToNotebookSources(results, svc, context.Background(), "user-1")

	if len(sources) != 1 {
		t.Fatalf("expected one source, got %d", len(sources))
	}
	if sources[0].ChunkType != "table" {
		t.Fatalf("expected chunk type table, got %q", sources[0].ChunkType)
	}
	if len(sources[0].BoundingBox) != 4 || sources[0].BoundingBox[2] != 110 {
		t.Fatalf("expected bbox to be preserved, got %#v", sources[0].BoundingBox)
	}
	if len(sources[0].SectionPath) != 2 || sources[0].SectionPath[1] != "Revenue" {
		t.Fatalf("expected section path to be preserved, got %#v", sources[0].SectionPath)
	}
}

func TestNotebookPromptUsesCanonicalCitationTokensInRetrievedContext(t *testing.T) {
	svc := &notebookChatService{}
	prompt := svc.buildPrompt(nil, []NotebookChatSource{
		{
			DocumentName: "Financial_Statements_2024.pdf",
			PageNumber:   0,
			Content:      "R&D spending was $266.4 million.",
		},
	}, "What was R&D spending?", "")

	if !strings.Contains(prompt, "[E1] [Source: Financial_Statements_2024.pdf, Page 1]") {
		t.Fatalf("expected evidence id with canonical source in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not output [Source: ...] citations directly") {
		t.Fatalf("expected prompt to forbid raw source citations, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Source: Financial_Statements_2024.pdf (Page 1)") {
		t.Fatalf("prompt still contains parenthesized citation format:\n%s", prompt)
	}
}

func TestNotebookPromptIncludesSessionMemoryBeforeEvidence(t *testing.T) {
	svc := &notebookChatService{}
	prompt := svc.buildPrompt(nil, []NotebookChatSource{
		{
			DocumentName: "Annual_Report_2024.pdf",
			PageNumber:   0,
			Content:      "Revenue reached $1.85B.",
		},
	}, "Continue the analysis", "The user is preparing a board briefing.")

	memoryIndex := strings.Index(prompt, "## Conversation Memory")
	evidenceIndex := strings.Index(prompt, "## Evidence Blocks")
	if memoryIndex < 0 || evidenceIndex < 0 || memoryIndex > evidenceIndex {
		t.Fatalf("expected conversation memory before evidence blocks, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "The user is preparing a board briefing.") {
		t.Fatalf("expected memory content in prompt, got:\n%s", prompt)
	}
}
