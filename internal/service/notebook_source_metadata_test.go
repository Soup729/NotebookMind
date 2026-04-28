package service

import (
	"context"
	"strings"
	"testing"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/models"
)

type fakeDocumentRepoForSources struct{}

type fakeTrustWorkflow struct{}

func (f *fakeTrustWorkflow) Run(context.Context, TrustWorkflowInput) (TrustWorkflowOutput, error) {
	return TrustWorkflowOutput{}, nil
}

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
	prompt := svc.buildPrompt(nil, nil, []NotebookChatSource{
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
	prompt := svc.buildPrompt(nil, nil, []NotebookChatSource{
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

func TestNotebookPromptIncludesSelectedDocumentGuidesBeforeEvidence(t *testing.T) {
	svc := &notebookChatService{}
	prompt := svc.buildPrompt(nil, []SelectedDocumentContext{
		{
			DocumentID:   "doc-1",
			DocumentName: "Tutorial 1_final.pdf",
			Summary:      "Covers Arduino Uno, DHT11 sensing, LCD display, and serial output.",
			KeyPoints:    []string{"Arduino Uno sensing workflow", "DHT11 humidity and temperature reading"},
			GuideStatus:  models.GuideStatusCompleted,
		},
		{
			DocumentID:   "doc-2",
			DocumentName: "Tutorial 2_final.pdf",
			Summary:      "Covers robot motor control and ultrasonic obstacle avoidance.",
			KeyPoints:    []string{"Motor driver control", "Ultrasonic distance measurement"},
			GuideStatus:  models.GuideStatusCompleted,
		},
	}, []NotebookChatSource{
		{
			DocumentID:   "doc-1",
			DocumentName: "Tutorial 1_final.pdf",
			PageNumber:   6,
			Content:      "The code initializes the DHT11 sensor and LCD.",
		},
	}, "这两个文档分别讲了什么内容？", "")

	guideIndex := strings.Index(prompt, "## Selected Document Context")
	evidenceIndex := strings.Index(prompt, "## Evidence Blocks")
	if guideIndex < 0 || evidenceIndex < 0 || guideIndex > evidenceIndex {
		t.Fatalf("expected selected document context before evidence, got:\n%s", prompt)
	}
	for _, expected := range []string{
		"Tutorial 1_final.pdf",
		"Tutorial 2_final.pdf",
		"Covers Arduino Uno",
		"Covers robot motor control",
		"must address each selected document",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
}

func TestNotebookPromptUsesSelectedGuidesEvenWhenEvidenceIsEmpty(t *testing.T) {
	svc := &notebookChatService{}
	prompt := svc.buildPrompt(nil, []SelectedDocumentContext{
		{
			DocumentID:   "doc-1",
			DocumentName: "Tutorial 2_final.pdf",
			Summary:      "Explains robot motor control and obstacle avoidance.",
			GuideStatus:  models.GuideStatusCompleted,
		},
	}, nil, "这个文档讲了什么？", "")

	if strings.Contains(prompt, "Answer questions using your general knowledge") {
		t.Fatalf("selected document guide context should not fall back to general knowledge prompt:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Tutorial 2_final.pdf") || !strings.Contains(prompt, "obstacle avoidance") {
		t.Fatalf("expected selected guide context in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "No exact retrieved evidence blocks were found") {
		t.Fatalf("expected empty evidence notice, got:\n%s", prompt)
	}
}

func TestGuideFirstOverviewBypassesEvidenceOnlyTrustWorkflow(t *testing.T) {
	svc := &notebookChatService{
		trustWorkflow: &fakeTrustWorkflow{},
		trustConfig: &configs.TrustWorkflowConfig{
			Enabled:      true,
			HighRiskOnly: true,
		},
	}
	input := TrustWorkflowInput{
		Question: "这两个文档分别讲了什么内容",
		DocumentContexts: []SelectedDocumentContext{
			{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", Summary: "Arduino sensing tutorial."},
			{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", Summary: "Robot control tutorial."},
		},
		Sources: []NotebookChatSource{{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 7, Content: "DHT11 sensor code."}},
	}

	if svc.shouldUseTrustWorkflow(input) {
		t.Fatal("guide-first multi-document overview should bypass evidence-only trust workflow")
	}
}

func TestCitationGuardKeepsGuideFirstOverviewInsteadOfFailingClosed(t *testing.T) {
	svc := &notebookChatService{
		citationGuard: &configs.CitationGuardConfig{
			Enabled:                   true,
			RequireParagraphCitations: true,
			ValidateNumbers:           true,
			ValidateEntityPhrases:     true,
			MinCitationCoverage:       0.8,
			FailClosedForHighRisk:     true,
		},
	}
	input := TrustWorkflowInput{
		Question: "这两个文档分别讲了什么内容",
		DocumentContexts: []SelectedDocumentContext{
			{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", Summary: "Arduino sensing tutorial."},
			{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", Summary: "Robot control tutorial."},
		},
		Sources: []NotebookChatSource{{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 7, Content: "DHT11 sensor code."}},
	}
	answer := "Tutorial 1 主要讲 Arduino 传感器训练。\n\nTutorial 2 主要讲机器人控制训练。"

	rendered := svc.applyCitationGuard(context.Background(), answer, input)

	if strings.Contains(rendered, "do not contain sufficient information") {
		t.Fatalf("guide overview should not be rewritten to insufficient answer: %s", rendered)
	}
	if !strings.Contains(rendered, "Tutorial 2 主要讲机器人控制训练") {
		t.Fatalf("expected original guide-grounded overview to remain, got: %s", rendered)
	}
}
