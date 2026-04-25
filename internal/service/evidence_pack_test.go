package service

import (
	"strings"
	"testing"
)

func TestBuildEvidencePackFormatsStableEvidenceIDs(t *testing.T) {
	plan := TrustPlan{Risks: []QueryRisk{QueryRiskTable}, NeedsFullTable: true}
	results := []HybridResult{
		{
			ChunkID:    "chunk-1",
			DocumentID: "doc-1",
			Content:    "R&D Expense Breakdown: Basic research $88.8M; Applied development $177.6M.",
			Metadata: map[string]interface{}{
				"page_number":  int64(0),
				"chunk_type":   "table",
				"section_path": []string{"Financial Tables", "R&D Expense Breakdown"},
			},
		},
	}
	sources := []NotebookChatSource{{DocumentID: "doc-1", DocumentName: "Financial_Statements_2024.pdf", PageNumber: 0}}

	pack := BuildEvidencePack(results, sources, plan)
	if len(pack.Items) != 1 {
		t.Fatalf("expected one evidence item, got %d", len(pack.Items))
	}
	if pack.Items[0].ID != "E1" {
		t.Fatalf("expected stable E1 id, got %q", pack.Items[0].ID)
	}
	formatted := pack.FormatForPrompt()
	if !strings.Contains(formatted, "[E1] [Source: Financial_Statements_2024.pdf, Page 1]") {
		t.Fatalf("formatted evidence missing canonical header:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Type: table") || !strings.Contains(formatted, "Section: Financial Tables > R&D Expense Breakdown") {
		t.Fatalf("formatted evidence missing metadata:\n%s", formatted)
	}
}

func TestBuildEvidencePackDeduplicatesByDocumentPageAndContent(t *testing.T) {
	results := []HybridResult{
		{ChunkID: "a", DocumentID: "doc", Content: "same content", Metadata: map[string]interface{}{"page_number": 0}},
		{ChunkID: "b", DocumentID: "doc", Content: "same content", Metadata: map[string]interface{}{"page_number": 0}},
	}
	sources := []NotebookChatSource{{DocumentID: "doc", DocumentName: "Annual_Report_2024.pdf", PageNumber: 0}}

	pack := BuildEvidencePack(results, sources, TrustPlan{})
	if len(pack.Items) != 1 {
		t.Fatalf("expected duplicate evidence to collapse, got %d", len(pack.Items))
	}
}
