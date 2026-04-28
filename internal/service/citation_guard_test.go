package service

import (
	"strings"
	"testing"
)

func TestBuildEvidencePackFromNotebookSourcesAssignsStableIDs(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Annual.pdf", PageNumber: 0, ChunkIndex: 2, Content: "Revenue was $1.85B.", ChunkType: "table", SectionPath: []string{"Financials"}, BoundingBox: []float32{1, 2, 3, 4}},
		{DocumentID: "doc-2", DocumentName: "Risk.pdf", PageNumber: 3, ChunkIndex: 1, Content: "Cybersecurity risk was rated High."},
	}

	pack := BuildEvidencePackFromNotebookSources(sources)

	if len(pack.Items) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(pack.Items))
	}
	if pack.Items[0].ID != "E1" || pack.Items[1].ID != "E2" {
		t.Fatalf("unexpected evidence IDs: %#v", pack.Items)
	}
	if pack.Items[0].DocumentName != "Annual.pdf" || pack.Items[0].PageNumber != 0 {
		t.Fatalf("metadata not preserved: %#v", pack.Items[0])
	}
	if pack.Items[0].ChunkType != "table" || len(pack.Items[0].BoundingBox) != 4 {
		t.Fatalf("structured metadata not preserved: %#v", pack.Items[0])
	}
}

func TestRenderEvidenceCitationsConvertsIDsToCanonicalSources(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{
		{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."},
		{ID: "E2", DocumentName: "Risk.pdf", PageNumber: 3, Content: "Cybersecurity risk was rated High."},
	}}

	answer := "Revenue was $1.85B. [E1]\n\nRisk was High. [E2]"
	rendered := RenderEvidenceCitations(answer, pack)

	if !strings.Contains(rendered, "[Source: Annual.pdf, Page 1, E1]") {
		t.Fatalf("missing Annual citation: %s", rendered)
	}
	if !strings.Contains(rendered, "[Source: Risk.pdf, Page 4, E2]") {
		t.Fatalf("missing Risk citation: %s", rendered)
	}
	if strings.Contains(rendered, "[E1]") || strings.Contains(rendered, "[E2]") {
		t.Fatalf("evidence IDs should be rendered away: %s", rendered)
	}
}

func TestValidateCitationBoundAnswerRejectsMissingParagraphCitation(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("Revenue was $1.85B.", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0.8})

	if result.Passed {
		t.Fatalf("expected missing citation to fail")
	}
	if result.Issues[0].Type != "missing_paragraph_citation" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateCitationBoundAnswerRejectsUnknownEvidenceID(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("Revenue was $1.85B. [E9]", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0.8})

	if result.Passed {
		t.Fatalf("expected unknown evidence id to fail")
	}
	if result.Issues[0].Type != "unknown_evidence_id" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateCitationBoundAnswerRejectsUnsupportedNumber(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("Revenue was $2.10B. [E1]", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0.8})

	if result.Passed {
		t.Fatalf("expected unsupported number to fail")
	}
	if result.Issues[0].Type != "unsupported_number" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateCitationBoundAnswerPassesSupportedParagraph(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("Revenue was $1.85B. [E1]", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0.8})

	if !result.Passed {
		t.Fatalf("expected answer to pass: %#v", result.Issues)
	}
}

func TestValidateCitationBoundAnswerRejectsUnsupportedEntityPhrase(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{
		ID:           "E1",
		DocumentName: "Market.pdf",
		PageNumber:   0,
		Content:      "The market share chart lists Nexus Technologies at 23% and DataForge Systems at 28%.",
	}}}
	answer := "The top competitors are DataForge Systems, CloudNova, and QuantumEdge. [E1]"

	result := ValidateCitationBoundAnswer(answer, pack, CitationGuardOptions{
		RequireParagraphCitations: true,
		ValidateNumbers:           true,
		ValidateEntityPhrases:     true,
		MinCitationCoverage:       0.8,
	})

	if result.Passed {
		t.Fatalf("expected unsupported entity phrase to fail")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "unsupported_entity" && strings.Contains(issue.Detail, "CloudNova") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported CloudNova entity issue, got %#v", result.Issues)
	}
}

func TestValidateCitationBoundAnswerAcceptsSupportedEntityPhrases(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{
		ID:           "E1",
		DocumentName: "Market.pdf",
		PageNumber:   0,
		Content:      "The market share chart lists Nexus Technologies at 23% and DataForge Systems at 28%.",
	}}}

	result := ValidateCitationBoundAnswer("Nexus Technologies had 23% share, behind DataForge Systems. [E1]", pack, CitationGuardOptions{
		RequireParagraphCitations: true,
		ValidateNumbers:           true,
		ValidateEntityPhrases:     true,
		MinCitationCoverage:       0.8,
	})

	if !result.Passed {
		t.Fatalf("expected supported entity phrases to pass: %#v", result.Issues)
	}
}

func TestCitationGuardRendersValidAnswer(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("Revenue was $1.85B. [E1]", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0.8})
	if !result.Passed {
		t.Fatalf("expected valid answer: %#v", result.Issues)
	}
	rendered := RenderEvidenceCitations("Revenue was $1.85B. [E1]", pack)
	if rendered != "Revenue was $1.85B. [Source: Annual.pdf, Page 1, E1]" {
		t.Fatalf("unexpected rendered answer: %s", rendered)
	}
}

func TestRenderEvidenceCitationsCollapsesConsecutiveSameSourcePage(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}

	rendered := RenderEvidenceCitations("Revenue grew year over year. [E1]\n\nThe increase was driven by enterprise demand. [E1]", pack)

	if strings.Contains(rendered, "Revenue grew year over year. [Source: Annual.pdf, Page 1, E1]") {
		t.Fatalf("expected first duplicate citation to be collapsed: %s", rendered)
	}
	if !strings.Contains(rendered, "The increase was driven by enterprise demand. [Source: Annual.pdf, Page 1, E1]") {
		t.Fatalf("expected final paragraph in run to keep citation: %s", rendered)
	}
}

func TestRenderEvidenceCitationsKeepsDifferentConsecutiveSources(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{
		{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."},
		{ID: "E2", DocumentName: "Annual.pdf", PageNumber: 1, Content: "Margin was 18%."},
	}}

	rendered := RenderEvidenceCitations("Revenue grew. [E1]\n\nMargin improved. [E2]", pack)

	if !strings.Contains(rendered, "[Source: Annual.pdf, Page 1, E1]") || !strings.Contains(rendered, "[Source: Annual.pdf, Page 2, E2]") {
		t.Fatalf("different source pages should both remain: %s", rendered)
	}
}

func TestRenderEvidenceCitationsCollapsesConsecutiveSameDocumentPageWithDifferentEvidenceIDs(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{
		{ID: "E1", DocumentName: "Tutorial.pdf", PageNumber: 6, Content: "The tutorial initializes the DHT11 sensor."},
		{ID: "E2", DocumentName: "Tutorial.pdf", PageNumber: 6, Content: "The tutorial also initializes the LCD display."},
	}}

	rendered := RenderEvidenceCitations("The tutorial initializes the DHT11 sensor. [E1]\n\nIt also initializes the LCD display. [E2]", pack)

	if strings.Contains(rendered, "DHT11 sensor. [Source: Tutorial.pdf, Page 7, E1]") {
		t.Fatalf("expected earlier same-page citation to collapse even with different evidence id: %s", rendered)
	}
	if !strings.Contains(rendered, "LCD display. [Source: Tutorial.pdf, Page 7, E2]") {
		t.Fatalf("expected final same-page citation to remain: %s", rendered)
	}
}

func TestAnnotateSourcesWithCitationIDsMatchesEvidencePackIDs(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Annual.pdf", PageNumber: 0, ChunkIndex: 2, Content: "Revenue was $1.85B."},
		{DocumentID: "doc-1", DocumentName: "Annual.pdf", PageNumber: 1, ChunkIndex: 3, Content: "Operating margin was 18%."},
	}

	annotated := annotateSourcesWithCitationIDs(sources)

	if annotated[0].CitationID != "E1" || annotated[1].CitationID != "E2" {
		t.Fatalf("expected citation IDs to match evidence pack order, got %#v", annotated)
	}
	if sources[0].CitationID != "" {
		t.Fatalf("annotateSourcesWithCitationIDs should not mutate caller slice")
	}
}

func TestCitationGuardInsufficientAnswerDoesNotRequireCitation(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Revenue was $1.85B."}}}
	result := ValidateCitationBoundAnswer("The provided documents do not contain sufficient information to answer this question.", pack, CitationGuardOptions{RequireParagraphCitations: true, ValidateNumbers: true, MinCitationCoverage: 0})
	if !result.Passed {
		t.Fatalf("expected insufficient answer to pass without citation: %#v", result.Issues)
	}
}
