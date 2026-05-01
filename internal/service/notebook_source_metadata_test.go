package service

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/models"
)

func TestFormatEvidencePageUsesOneBasedDisplay(t *testing.T) {
	if got := formatEvidencePage(0); got != "Page 1" {
		t.Fatalf("expected Page 1 for internal page 0, got %q", got)
	}
	if got := formatEvidencePage(5); got != "Page 6" {
		t.Fatalf("expected Page 6 for internal page 5, got %q", got)
	}
}

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

func TestNotebookPromptIncludesStableGenerationContracts(t *testing.T) {
	svc := &notebookChatService{}
	prompt := svc.buildPrompt(nil, []SelectedDocumentContext{
		{DocumentID: "doc-1", DocumentName: "Tutorial.pdf", Summary: "Uses DHT11 and I2C LCD.", GuideStatus: models.GuideStatusCompleted},
	}, []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Tutorial.pdf", PageNumber: 0, Content: "The lab uses DHT11 and I2C LCD."},
	}, "请说明 DHT11 和 I2C LCD 的作用", "")

	for _, expected := range []string{
		"Answer in the same language as the user's question",
		"Keep technical terms",
		"Do NOT use external knowledge",
		"Do not output [Source: ...] citations directly",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected stable generation contract %q in prompt:\n%s", expected, prompt)
		}
	}
}

func TestNotebookPromptCoverageMatrixUsesGeneratedEvidenceIDs(t *testing.T) {
	svc := &notebookChatService{}
	sources := []NotebookChatSource{{
		DocumentID:   "doc-1",
		DocumentName: "Manual.pdf",
		PageNumber:   1,
		Content:      "Section 2 requires Serial Monitor output every 200ms.",
	}}

	prompt := svc.buildPrompt(nil, nil, sources, "哪些章节明确要求实时显示或实时数据输出？", "")

	coverageIndex := strings.Index(prompt, "## Coverage Matrix")
	evidenceIndex := strings.Index(prompt, "## Evidence Blocks")
	if coverageIndex < 0 || evidenceIndex < 0 || coverageIndex > evidenceIndex {
		t.Fatalf("expected coverage matrix before evidence blocks, got:\n%s", prompt)
	}
	coverageBlock := prompt[coverageIndex:evidenceIndex]
	if !strings.Contains(coverageBlock, "[E1]") {
		t.Fatalf("expected generated evidence ID in coverage block, got:\n%s", coverageBlock)
	}
	if sources[0].CitationID != "" {
		t.Fatalf("buildPrompt should not mutate caller sources, got citation ID %q", sources[0].CitationID)
	}
}

func TestNotebookPromptKeepsDesignGuideItemsOnlyInBackgroundHints(t *testing.T) {
	svc := &notebookChatService{}
	contexts := []SelectedDocumentContext{{
		DocumentID:   "doc-1",
		DocumentName: "Guide.pdf",
		Summary:      "The guide mentions fan and humidifier as possible extensions.",
		KeyPoints:    []string{"Use alarm device for warning"},
	}}
	sources := []NotebookChatSource{{
		DocumentID:   "doc-1",
		DocumentName: "Guide.pdf",
		PageNumber:   0,
		Content:      "Components: LED light. Requirements: output status to Serial Monitor.",
	}}

	prompt := svc.buildPrompt(nil, contexts, sources, "只使用文档中出现过的组件，设计一个监控系统方案", "")

	if !strings.Contains(prompt, "Core allowed items") || !strings.Contains(prompt, "LED light") {
		t.Fatalf("expected evidence-backed core inventory, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Background hints") || !strings.Contains(prompt, "fan and humidifier") {
		t.Fatalf("expected guide content under background hints, got:\n%s", prompt)
	}
	beforeBackground := strings.Split(prompt, "Background hints")[0]
	if strings.Contains(beforeBackground, "fan and humidifier") {
		t.Fatalf("guide-only content must not be emitted before background hints, got:\n%s", prompt)
	}
}

func TestNotebookPromptKeepsDesignFAQQuestionsInBackgroundHints(t *testing.T) {
	svc := &notebookChatService{}
	contexts := []SelectedDocumentContext{{
		DocumentID:   "doc-1",
		DocumentName: "Guide.pdf",
		FAQ: []DocumentGuideFAQ{{
			Question: "Can a fan be used?",
			Answer:   "Only as a possible extension.",
		}},
	}}
	sources := []NotebookChatSource{{
		DocumentID:   "doc-1",
		DocumentName: "Guide.pdf",
		PageNumber:   0,
		Content:      "Components: LED light. Requirements: output status to Serial Monitor.",
	}}

	prompt := svc.buildPrompt(nil, contexts, sources, "只使用文档中出现过的组件，设计一个监控系统方案", "")

	backgroundIndex := strings.Index(prompt, "Background hints")
	if backgroundIndex < 0 {
		t.Fatalf("expected background hints in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt[backgroundIndex:], "Can a fan be used?") {
		t.Fatalf("expected FAQ question under background hints, got:\n%s", prompt)
	}
	if strings.Contains(prompt[:backgroundIndex], "Can a fan be used?") {
		t.Fatalf("FAQ question must not appear in core inventory or selected context, got:\n%s", prompt)
	}
}

func TestClassifyNotebookAnswerModeAcrossDomains(t *testing.T) {
	tests := []struct {
		name     string
		question string
		want     NotebookAnswerMode
	}{
		{"coverage policy", "哪些章节涉及数据留存和审计日志？", NotebookAnswerModeCoverageListing},
		{"coverage english", "Which sections mention retention controls?", NotebookAnswerModeCoverageListing},
		{"constrained product", "只使用文档中出现过的功能，设计一个审批工作流", NotebookAnswerModeDesignSynthesis},
		{"constrained english", "Use only the documented modules to design a reporting workflow", NotebookAnswerModeDesignSynthesis},
		{"comparison section", "比较 Section 2 和 Appendix A 的数据保留要求", NotebookAnswerModeComparison},
		{"constraint check", "Policy 3.2 是否明确指定保留期限？如果没有怎么办？", NotebookAnswerModeConstraintCheck},
		{"overview", "这两个文档分别讲了什么内容？", NotebookAnswerModeOverview},
		{"exact fact", "R&D spending was多少？", NotebookAnswerModeExactFact},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyNotebookAnswerMode(tt.question); got != tt.want {
				t.Fatalf("classifyNotebookAnswerMode(%q)=%s, want %s", tt.question, got, tt.want)
			}
		})
	}
}

func TestClassifyNotebookAnswerModePrioritizesCoverageBeforeDesign(t *testing.T) {
	question := "只使用选中文档，列出哪些章节明确要求实时显示或实时数据输出？"
	if got := classifyNotebookAnswerMode(question); got != NotebookAnswerModeCoverageListing {
		t.Fatalf("expected coverage listing to win over constrained synthesis, got %s", got)
	}
}

func TestClassifyNotebookAnswerModePrioritizesConstraintBeforeDesign(t *testing.T) {
	question := "基于 Section 4，文档是否要求必须使用 LCD 显示距离？如果没有，输出方式是什么？"
	if got := classifyNotebookAnswerMode(question); got != NotebookAnswerModeConstraintCheck {
		t.Fatalf("expected constraint check to win over constrained synthesis, got %s", got)
	}
}

func TestConstrainedSynthesisRequiresStrongDesignSignal(t *testing.T) {
	weak := "这个系统基于哪些文档内容？"
	if got := classifyNotebookAnswerMode(weak); got == NotebookAnswerModeDesignSynthesis {
		t.Fatalf("weak system/based wording should not trigger design mode")
	}
	strong := "只使用文档中出现过的组件，设计一个监控系统方案"
	if got := classifyNotebookAnswerMode(strong); got != NotebookAnswerModeDesignSynthesis {
		t.Fatalf("expected strong design wording to trigger constrained synthesis, got %s", got)
	}
}

func TestConstrainedSynthesisIgnoresFactualProposalAndDesignSubstrings(t *testing.T) {
	tests := []string{
		"方案 A 的预算是多少？",
		"What does the proposal specify?",
		"The document has a designated reviewer; who approves it?",
		"Use only the selected document, what does the proposal specify?",
		"只使用文档回答，方案 A 的预算是多少？",
		"只使用文档中出现过的组件有哪些？",
	}
	for _, question := range tests {
		t.Run(question, func(t *testing.T) {
			if got := classifyNotebookAnswerMode(question); got == NotebookAnswerModeDesignSynthesis {
				t.Fatalf("factual proposal/design wording should not trigger design mode")
			}
		})
	}
}

func TestConstrainedSynthesisRecognizesEnglishOnlyUseDesignRequest(t *testing.T) {
	question := "Using only documented modules, design a reporting workflow"
	if got := classifyNotebookAnswerMode(question); got != NotebookAnswerModeDesignSynthesis {
		t.Fatalf("expected English only-use design request to trigger constrained synthesis, got %s", got)
	}
}

func TestDocumentAnchorResolverExtractsGenericAnchors(t *testing.T) {
	anchors := resolveDocumentAnchors("Compare Section 2.1, Table 4, Figure 3, Requirement R1 and Policy 3.2")
	labels := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		labels = append(labels, anchor.Label)
	}
	for _, expected := range []string{"Section 2.1", "Table 4", "Figure 3", "Requirement R1", "Policy 3.2"} {
		found := false
		for _, label := range labels {
			if label == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected anchor %q in %#v", expected, anchors)
		}
	}
}

func TestExtractComparisonSubjectsFromAnchors(t *testing.T) {
	subjects := extractComparisonSubjects("请比较 Lab Session 3 和 Lab Session 5 的输出内容与数据处理难点")
	labels := comparisonSubjectLabels(subjects)
	for _, expected := range []string{"Lab Session 3", "Lab Session 5"} {
		if !containsString(labels, expected) {
			t.Fatalf("expected subject %q in %#v", expected, labels)
		}
	}
}

func TestExtractComparisonSubjectsIncludesAppendixLetterAnchors(t *testing.T) {
	subjects := extractComparisonSubjects("比较 Section 2 和 Appendix A 的数据保留要求")
	labels := comparisonSubjectLabels(subjects)
	for _, expected := range []string{"Section 2", "Appendix A"} {
		if !containsString(labels, expected) {
			t.Fatalf("expected subject %q in %#v", expected, labels)
		}
	}
}

func TestExtractComparisonSubjectsRejectsDimensionListFragments(t *testing.T) {
	subjects := extractComparisonSubjects("请比较测量对象、主要传感器、数据处理难点和显示内容分别有什么不同？")
	labels := comparisonSubjectLabels(subjects)
	for _, forbidden := range []string{"测量对象、主要传感器、数据处理难点", "显示内容"} {
		if containsString(labels, forbidden) {
			t.Fatalf("did not expect dimension-list fragment %q in %#v", forbidden, labels)
		}
	}
	if len(subjects) != 0 {
		t.Fatalf("expected no subjects for dimension-only comparison question, got %#v", labels)
	}
}

func TestAnchorMatchesTextUsesNumberBoundaries(t *testing.T) {
	anchor := DocumentAnchor{Type: "section", Label: "Section 2", Number: "2", Aliases: []string{"Section 2", "Sec. 2", "§ 2"}}
	if !anchorMatchesText(anchor, "section 2 requires serial monitor output") {
		t.Fatal("expected exact Section 2 text to match")
	}
	if !anchorMatchesText(anchor, "sec. 2 requires serial monitor output") {
		t.Fatal("expected Sec. 2 alias to match")
	}
	if !anchorMatchesText(anchor, "§ 2 requires serial monitor output") {
		t.Fatal("expected § 2 alias to match")
	}
	for _, text := range []string{"section 20 requires serial monitor output", "section 2a requires serial monitor output"} {
		if anchorMatchesText(anchor, text) {
			t.Fatalf("did not expect Section 2 anchor to match %q", text)
		}
	}

	section2A := DocumentAnchor{Type: "section", Label: "Section 2A", Number: "2A", Aliases: []string{"Section 2A", "Sec. 2A", "§ 2A"}}
	if !anchorMatchesText(section2A, "section 2a requires serial monitor output") {
		t.Fatal("expected exact Section 2A text to match")
	}
}

func TestExtractComparisonDimensionsFromQuestion(t *testing.T) {
	dimensions := extractComparisonDimensions("请比较 Lab Session 3 和 Lab Session 5：测量对象、主要传感器、数据处理难点和显示内容分别有什么不同？")
	names := comparisonDimensionNames(dimensions)
	for _, expected := range []string{"测量对象", "主要传感器", "数据处理难点", "显示内容"} {
		if !containsString(names, expected) {
			t.Fatalf("expected dimension %q in %#v", expected, names)
		}
	}
}

func TestBuildComparisonMatrixKeepsEvidencePerSubject(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", Content: "Lab Session 3 uses DHT11 and LCD to display temperature and humidity.", CitationID: "E1"},
		{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", Content: "Lab Session 5 uses IR speed sensor and LCD to display RPM.", CitationID: "E2"},
	}
	matrix := buildComparisonMatrix(sources, "请比较 Lab Session 3 和 Lab Session 5 的主要传感器和显示内容")
	if len(matrix.Subjects) != 2 {
		t.Fatalf("expected two subjects, got %#v", matrix.Subjects)
	}
	cell := matrix.Cell("Lab Session 3", "主要传感器")
	if cell == nil || len(cell.Evidence) == 0 || cell.Evidence[0].CitationID != "E1" {
		t.Fatalf("expected Lab Session 3 sensor cell to use only E1, got %#v", cell)
	}
	wrong := matrix.Cell("Lab Session 3", "显示内容")
	if wrong == nil {
		t.Fatalf("expected Lab Session 3 display cell")
	}
	for _, evidence := range wrong.Evidence {
		if evidence.CitationID == "E2" {
			t.Fatalf("Lab Session 3 cell must not use Lab Session 5 evidence: %#v", wrong)
		}
	}
}

func TestBuildComparisonMatrixMarksMissingCellEvidence(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 2 describes retention period.", CitationID: "E1"},
	}
	matrix := buildComparisonMatrix(sources, "比较 Section 2 和 Section 3 的审计日志要求")
	cell := matrix.Cell("Section 3", "要求")
	if cell == nil || cell.Status != "missing" {
		t.Fatalf("expected missing cell for Section 3 requirement, got %#v", cell)
	}
}

func TestBuildComparisonMatrixRequiresSubtopicForGenericRequirementDimension(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 2 requires password rotation.", CitationID: "E1"},
	}
	matrix := buildComparisonMatrix(sources, "比较 Section 2 和 Section 3 的审计日志要求")
	cell := matrix.Cell("Section 2", "要求")
	if cell == nil || cell.Status != "missing" {
		t.Fatalf("expected Section 2 audit log requirement to stay missing, got %#v", cell)
	}
}

func TestBuildComparisonMatrixDocumentNameFallbackDoesNotMatchContentMentions(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Alpha.pdf", Content: "Alpha.pdf mentions Beta.pdf in prose and requires password rotation.", CitationID: "E1"},
		{DocumentID: "doc-2", DocumentName: "Beta.pdf", Content: "Beta.pdf requires audit logging.", CitationID: "E2"},
	}
	matrix := buildComparisonMatrix(sources, "请比较这些文档的要求")
	cell := matrix.Cell("Beta.pdf", "要求")
	if cell == nil {
		t.Fatal("expected Beta.pdf requirement cell")
	}
	for _, evidence := range cell.Evidence {
		if evidence.CitationID == "E1" {
			t.Fatalf("Beta.pdf fallback subject must not use Alpha.pdf evidence: %#v", cell)
		}
	}
}

func TestBuildComparisonMatrixDocumentNameFallbackWithEmptyDocumentIDDoesNotMatchContentMentions(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentName: "Alpha.pdf", Content: "Alpha.pdf mentions Beta.pdf in prose and requires password rotation.", CitationID: "E1"},
		{DocumentName: "Beta.pdf", Content: "Beta.pdf requires audit logging.", CitationID: "E2"},
	}
	matrix := buildComparisonMatrix(sources, "请比较这些文档的要求")
	cell := matrix.Cell("Beta.pdf", "要求")
	if cell == nil {
		t.Fatal("expected Beta.pdf requirement cell")
	}
	for _, evidence := range cell.Evidence {
		if evidence.CitationID == "E1" {
			t.Fatalf("Beta.pdf fallback subject with empty document ID must not use Alpha.pdf evidence: %#v", cell)
		}
	}
}

func comparisonSubjectLabels(subjects []ComparisonSubject) []string {
	labels := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		labels = append(labels, subject.Label)
	}
	return labels
}

func comparisonDimensionNames(dimensions []ComparisonDimension) []string {
	names := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		names = append(names, dimension.Name)
	}
	return names
}

func TestNotebookRetrievalQueryPreservesQuestionAnchorsAndAddsGenericModeTerms(t *testing.T) {
	query := buildNotebookRetrievalQuery("哪些 Section 2.1 和 Table 4 涉及风险监控？")
	for _, expected := range []string{"哪些 Section 2.1", "Section 2.1", "Table 4", "coverage listing", "requirement instruction constraint"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected generic retrieval query to contain %q, got %q", expected, query)
		}
	}
}

func TestNotebookRetrievalQueryDoesNotHardCodeLabTermsForGenericConstrainedSynthesis(t *testing.T) {
	query := buildNotebookRetrievalQuery("只使用 Product Brief 中出现过的功能，设计合规报表工作流")
	for _, forbidden := range []string{"Lab Session 6", "DHT11", "HC-SR04", "servo motor"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("generic constrained synthesis query should not hard-code %q: %q", forbidden, query)
		}
	}
	for _, expected := range []string{"constrained synthesis", "allowed items", "features", "workflow"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected generic constrained query to contain %q, got %q", expected, query)
		}
	}
}

func TestNotebookRetrievalQueryAddsGenericRealtimeOutputCoverageTerms(t *testing.T) {
	query := buildNotebookRetrievalQuery("哪些章节明确要求实时显示或实时数据输出？")
	for _, expected := range []string{"coverage listing", "real-time", "output display", "refresh frequency", "trigger condition", "sensor value", "PWM output", "serial print"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected realtime output coverage term %q, got %q", expected, query)
		}
	}
}

func TestNotebookRetrievalQueryExpandsOutputFormatSubquestions(t *testing.T) {
	query := buildNotebookRetrievalQuery("在 Section 4 中，是否必须用 LCD 显示距离？如果没有，明确要求的距离输出方式是什么？")
	for _, expected := range []string{"Section 4", "output format", "serial output", "filtered distance", "loop cycle", "format string"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected output-format retrieval term %q, got %q", expected, query)
		}
	}
}

func TestCoverageRetrievalPlanUsesMultipleGenericRoutes(t *testing.T) {
	plan := buildNotebookRetrievalPlan("哪些章节明确要求实时显示或实时数据输出？", 12)
	if plan.Mode != NotebookAnswerModeCoverageListing {
		t.Fatalf("expected coverage mode, got %s", plan.Mode)
	}
	names := retrievalRouteNames(plan.Routes)
	for _, expected := range []string{"original_question", "requirement_constraint", "output_display", "format_frequency_trigger", "structure_items"} {
		if _, ok := names[expected]; !ok {
			t.Fatalf("expected route %q in %#v", expected, names)
		}
	}
	for _, route := range plan.Routes {
		if strings.Contains(route.Query, "DHT11") || strings.Contains(route.Query, "Lab Session 6") {
			t.Fatalf("planner route should not hard-code sample terms: %#v", route)
		}
	}
}

func TestCoverageRetrievalPlanIncludesStructureDiscoveryRoute(t *testing.T) {
	plan := buildNotebookRetrievalPlan("哪些章节明确要求实时显示或实时数据输出？", 12)
	names := retrievalRouteNames(plan.Routes)
	if _, ok := names["structure_discovery"]; !ok {
		t.Fatalf("expected structure_discovery route in %#v", names)
	}
}

func TestComparisonRetrievalPlanUsesSubjectScopedRoutes(t *testing.T) {
	plan := buildNotebookRetrievalPlan("请比较 Lab Session 3 和 Lab Session 5 的主要传感器和显示内容", 12)
	var hasSubject3 bool
	var hasSubject5 bool
	for _, route := range plan.Routes {
		if strings.Contains(route.Name, "subject") && strings.Contains(route.Query, "Lab Session 3") {
			hasSubject3 = true
		}
		if strings.Contains(route.Name, "subject") && strings.Contains(route.Query, "Lab Session 5") {
			hasSubject5 = true
		}
	}
	if !hasSubject3 || !hasSubject5 {
		t.Fatalf("expected subject-scoped comparison routes, got %#v", plan.Routes)
	}
}

func TestConstrainedSynthesisRetrievalPlanIncludesInventoryRoute(t *testing.T) {
	plan := buildNotebookRetrievalPlan("只使用文档中出现过的功能，设计一个审批工作流", 12)
	names := retrievalRouteNames(plan.Routes)
	if _, ok := names["allowed_items_inventory"]; !ok {
		t.Fatalf("expected allowed_items_inventory route in %#v", names)
	}
}

func retrievalRouteNames(routes []NotebookRetrievalRoute) map[string]struct{} {
	names := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		names[route.Name] = struct{}{}
	}
	return names
}

func TestPrioritizeNotebookSourcesPrefersExactAnchorEvidence(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", PageNumber: 0, Content: "Lab Session 5 uses an I2C-LCD to display RPM values."},
		{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 5, Content: "Lab Session 3 requires DHT11 temperature and humidity on a 16x2 I2C LCD and Serial Monitor output."},
		{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 0, Content: "Background: Arduino interrupts can improve real-time performance."},
	}

	ranked := prioritizeNotebookSourcesForQuestion(sources, "Lab Session 3 是否只需要串口输出？")

	if ranked[0].PageNumber != 5 || !strings.Contains(ranked[0].Content, "Lab Session 3") {
		t.Fatalf("expected exact Lab Session 3 requirement evidence first, got %#v", ranked[0])
	}
}

func TestMergeNotebookSourcesDeduplicatesAndCapsEvidence(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", PageNumber: 0, ChunkIndex: 1, Content: "same", Score: 0.5},
		{DocumentID: "doc-1", PageNumber: 0, ChunkIndex: 1, Content: "same", Score: 0.9},
		{DocumentID: "doc-2", PageNumber: 1, ChunkIndex: 2, Content: "other", Score: 0.7},
	}
	merged := mergeNotebookSources(sources, 1)
	if len(merged) != 1 {
		t.Fatalf("expected capped deduped result, got %#v", merged)
	}
	if merged[0].Score != 0.9 {
		t.Fatalf("expected best duplicate score to survive, got %#v", merged[0])
	}
}

func TestMergeNotebookSourcesPreservesRetrievalRoute(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", PageNumber: 0, ChunkIndex: 1, Content: "same", Score: 0.5, RetrievalRoute: "original_question"},
		{DocumentID: "doc-1", PageNumber: 0, ChunkIndex: 1, Content: "same", Score: 0.9, RetrievalRoute: "output_display"},
	}
	merged := mergeNotebookSources(sources, 10)
	if len(merged) != 1 {
		t.Fatalf("expected one deduped source, got %#v", merged)
	}
	if merged[0].RetrievalRoute != "output_display" {
		t.Fatalf("expected best route to survive, got %#v", merged[0])
	}
}

func TestStructureEvidenceConfigDefaults(t *testing.T) {
	cfg := defaultStructureEvidenceConfig()
	if !cfg.Enabled {
		t.Fatal("structure evidence should be enabled by default")
	}
	if cfg.MaxChunksPerDocument < 100 {
		t.Fatalf("expected broad document-scoped scan budget, got %d", cfg.MaxChunksPerDocument)
	}
	if cfg.AnchorContextWindow < 1 {
		t.Fatalf("expected nearby context window, got %d", cfg.AnchorContextWindow)
	}
	if cfg.MaxEvidence < 8 {
		t.Fatalf("expected evidence cap suitable for matrix prompts, got %d", cfg.MaxEvidence)
	}
}

func TestStructureEvidenceBucketKeys(t *testing.T) {
	cases := []struct {
		name     string
		evidence StructureEvidence
		expected string
	}{
		{name: "explicit bucket wins", evidence: StructureEvidence{BucketKey: "coverage:Section 2", Anchor: "Section 9"}, expected: "coverage:Section 2"},
		{name: "anchor route", evidence: StructureEvidence{Route: "anchor_first", Anchor: "Section 2"}, expected: "anchor:Section 2"},
		{name: "exact phrase route", evidence: StructureEvidence{Route: "exact_phrase", Anchor: "Table 4"}, expected: "anchor:Table 4"},
		{name: "coverage route", evidence: StructureEvidence{Route: "section_scan", Anchor: "Requirement R1"}, expected: "coverage:Requirement R1"},
		{name: "subject route", evidence: StructureEvidence{Route: "comparison_subject_1", Subject: "Product A"}, expected: "subject:Product A"},
		{name: "fallback route", evidence: StructureEvidence{Route: "original_question"}, expected: "route:original_question"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := structureEvidenceBucketKey(tc.evidence); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestMergeStructureEvidencePreservesMandatoryBucketsBeforeCap(t *testing.T) {
	evidence := []StructureEvidence{
		{Source: NotebookChatSource{Content: "semantic high", Score: 0.99, CitationID: "S1"}, Route: "original_question"},
		{Source: NotebookChatSource{Content: "Section 2 required output", Score: 0.1, CitationID: "A1"}, Route: "anchor_first", Anchor: "Section 2", Mandatory: true},
		{Source: NotebookChatSource{Content: "Section 4 required output", Score: 0.1, CitationID: "A2"}, Route: "section_scan", Anchor: "Section 4", CoverageStatus: CoverageExplicit, Mandatory: true},
	}
	merged := mergeStructureEvidence(evidence, 2)
	contents := structureEvidenceContents(merged)
	for _, expected := range []string{"Section 2 required output", "Section 4 required output"} {
		if !containsString(contents, expected) {
			t.Fatalf("expected mandatory evidence %q in %#v", expected, contents)
		}
	}
}

func TestMergeStructureEvidenceKeepsOnePerMandatoryBucketWhenOverCap(t *testing.T) {
	evidence := []StructureEvidence{
		{Source: NotebookChatSource{Content: "Section 2 title", Score: 0.4}, Route: "anchor_first", Anchor: "Section 2", Mandatory: true},
		{Source: NotebookChatSource{Content: "Section 2 detail", Score: 0.9}, Route: "anchor_first", Anchor: "Section 2", Mandatory: true},
		{Source: NotebookChatSource{Content: "Section 4 detail", Score: 0.8}, Route: "section_scan", Anchor: "Section 4", CoverageStatus: CoverageExplicit, Mandatory: true},
	}
	merged := mergeStructureEvidence(evidence, 2)
	if len(merged) != 2 {
		t.Fatalf("expected cap 2, got %#v", merged)
	}
	contents := structureEvidenceContents(merged)
	for _, expected := range []string{"Section 2 detail", "Section 4 detail"} {
		if !containsString(contents, expected) {
			t.Fatalf("expected mandatory evidence %q in %#v", expected, contents)
		}
	}
}

func TestBuildAnchorFirstEvidenceIncludesNearbyContext(t *testing.T) {
	chunks := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", ChunkIndex: 10, Content: "Section 2 - Output Controls", SectionPath: []string{"Section 2"}},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", ChunkIndex: 11, Content: "The system must print sensor value via serial output.", SectionPath: []string{"Section 2"}},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", ChunkIndex: 12, Content: "The output refresh interval is every 5 seconds.", SectionPath: []string{"Section 2"}},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", ChunkIndex: 20, Content: "Section 3 unrelated.", SectionPath: []string{"Section 3"}},
	}
	anchors := resolveDocumentAnchors("Section 2 的输出要求是什么？")
	evidence := buildAnchorFirstEvidenceFromSources(chunks, anchors, configs.StructureEvidenceConfig{Enabled: true, AnchorContextWindow: 2})
	contents := structureEvidenceContents(evidence)
	for _, expected := range []string{"The system must print sensor value via serial output.", "The output refresh interval is every 5 seconds."} {
		if !containsString(contents, expected) {
			t.Fatalf("expected nearby context %q in %#v", expected, contents)
		}
	}
	if containsString(contents, "Section 3 unrelated.") {
		t.Fatalf("should not include unrelated section context: %#v", contents)
	}
}

func TestExtractExactPhraseCandidatesClassifiesConfidence(t *testing.T) {
	candidates := extractExactPhraseCandidates(`Does Section 2 require "Status: approved" within 24 hours for ISO 27001?`)
	if !hasExactPhrase(candidates, "Status: approved", "high") {
		t.Fatalf("expected quoted template high confidence, got %#v", candidates)
	}
	if !hasExactPhrase(candidates, "24 hours", "high") {
		t.Fatalf("expected unit phrase high confidence, got %#v", candidates)
	}
	if !hasExactPhrase(candidates, "ISO 27001", "medium") {
		t.Fatalf("expected standard id medium confidence, got %#v", candidates)
	}
}

func TestNormalizeExactPhraseVariantsEscapesUnsafeText(t *testing.T) {
	variants := normalizeExactPhraseVariants("Metric: xx. xx% [A+B]")
	if !containsString(variants, "Metric: xx.xx% [A+B]") {
		t.Fatalf("expected punctuation-normalized variant, got %#v", variants)
	}
	for _, variant := range variants {
		if _, err := regexp.Compile(regexp.QuoteMeta(variant)); err != nil {
			t.Fatalf("variant should be regex safe after QuoteMeta: %q err=%v", variant, err)
		}
	}
}

func TestCoverageScanUsesBroadDocumentScopedCandidates(t *testing.T) {
	topK := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 1 controls an LED.", ChunkIndex: 1},
	}
	broad := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 1 controls an LED.", ChunkIndex: 1},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 2 must output status to the audit log.", ChunkIndex: 2},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 4 shall display the current state.", ChunkIndex: 4},
	}
	result := buildCoverageScanResult(topK, broad, "哪些章节明确要求输出或显示？", configs.StructureEvidenceConfig{Enabled: true, MaxChunksPerDocument: 500})
	anchors := coverageScanAnchorsByStatus(result, CoverageExplicit)
	for _, expected := range []string{"Section 2", "Section 4"} {
		if !containsString(anchors, expected) {
			t.Fatalf("expected broad scan explicit anchor %q in %#v", expected, anchors)
		}
	}
	if result.SourceKind != "repository_chunks" {
		t.Fatalf("expected broad source kind, got %q", result.SourceKind)
	}
}

func TestCoverageScanMarksControlOnlyAsExcluded(t *testing.T) {
	broad := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 1 turns on an LED after detection.", ChunkIndex: 1},
	}
	result := buildCoverageScanResult(nil, broad, "哪些章节明确要求数据输出？", configs.StructureEvidenceConfig{Enabled: true, MaxChunksPerDocument: 500})
	if len(result.Items) != 1 || result.Items[0].Status != CoverageExcluded {
		t.Fatalf("expected control-only excluded item, got %#v", result.Items)
	}
}

func TestCoverageMatrixUsesStructureEvidenceBuckets(t *testing.T) {
	items := []CoverageItem{{
		Anchor: "Section 2", Status: CoverageExplicit, IsExplicit: true,
		StructureEvidence: []StructureEvidence{{
			Source:    NotebookChatSource{CitationID: "E2", Content: "Section 2 must output status."},
			BucketKey: "coverage:Section 2", Mandatory: true,
		}},
	}}
	block := formatCoverageMatrixForPrompt(CoverageMatrix{Items: items})
	if !strings.Contains(block, "Section 2") || !strings.Contains(block, "[E2]") {
		t.Fatalf("expected bucketed coverage evidence in prompt, got:\n%s", block)
	}
}

func TestComparisonPromptRendersSubjectEvidenceBuckets(t *testing.T) {
	matrix := ComparisonMatrix{
		Subjects:   []ComparisonSubject{{Label: "Section 2"}, {Label: "Section 4"}},
		Dimensions: []ComparisonDimension{{Name: "要求"}},
		Cells: []ComparisonCell{
			{SubjectLabel: "Section 2", Dimension: "要求", Status: "supported", Evidence: []NotebookChatSource{{CitationID: "E2"}}},
			{SubjectLabel: "Section 4", Dimension: "要求", Status: "missing"},
		},
	}
	block := formatComparisonMatrixForPrompt(matrix)
	for _, expected := range []string{"## Comparison Evidence Matrix", "Subject: Section 2", "Evidence: [E2]", "Subject: Section 4"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected %q in comparison prompt:\n%s", expected, block)
		}
	}
}

func TestStructureFirstEvidenceDisabledReturnsOriginalSources(t *testing.T) {
	sources := []NotebookChatSource{{Content: "semantic only", Score: 0.9}}
	out := applyStructureFirstEvidenceForTest(sources, nil, "Section 2 的要求是什么？", configs.StructureEvidenceConfig{Enabled: false})
	if len(out) != 1 || out[0].Content != "semantic only" {
		t.Fatalf("disabled resolver should preserve original sources, got %#v", out)
	}
}

func TestStructureFirstEvidenceRunsBeforeFinalCap(t *testing.T) {
	retrieved := []NotebookChatSource{{Content: "semantic high", Score: 0.99}}
	broad := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 2 - Requirements", ChunkIndex: 2},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 2 must output status.", ChunkIndex: 3},
	}
	out := applyStructureFirstEvidenceForTest(retrieved, broad, "Section 2 的要求是什么？", configs.StructureEvidenceConfig{Enabled: true, MaxEvidence: 1, AnchorContextWindow: 2, MaxChunksPerDocument: 500})
	if len(out) != 1 || !strings.Contains(out[0].Content, "Section 2 must output status") {
		t.Fatalf("mandatory anchor evidence must win before cap, got %#v", out)
	}
}

func TestStructureEvidenceSourcesCarryBucketMetadataIntoCoverageMatrix(t *testing.T) {
	retrieved := []NotebookChatSource{{Content: "semantic high", Score: 0.99}}
	broad := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", Content: "Section 1 controls an LED. Section 2 - Requirements. Print the current light sensor value and PWM output via serial every 200ms.", ChunkIndex: 1},
	}
	out := applyStructureFirstEvidenceForTest(retrieved, broad, "哪些 Section 2 明确要求实时显示或实时数据输出？", configs.StructureEvidenceConfig{Enabled: true, MaxEvidence: 4, AnchorContextWindow: 2, MaxChunksPerDocument: 500})
	matrix := buildCoverageMatrix(annotateSourcesWithCitationIDs(out), "哪些 Section 2 明确要求实时显示或实时数据输出？")
	var section2 *CoverageItem
	for i := range matrix.Items {
		if matrix.Items[i].Anchor == "Section 2" {
			section2 = &matrix.Items[i]
			break
		}
	}
	if section2 == nil {
		t.Fatalf("expected Section 2 coverage item from structure bucket, got %#v", matrix.Items)
	}
	if section2.Status != CoverageExplicit {
		t.Fatalf("expected Section 2 explicit, got %#v", section2)
	}
	if len(section2.StructureEvidence) == 0 || section2.StructureEvidence[0].BucketKey != "coverage:Section 2" {
		t.Fatalf("expected structure bucket metadata to reach matrix, got %#v", section2.StructureEvidence)
	}
}

func TestCoverageRowsAssignsMultiAnchorChunkEvidenceToLaterAnchor(t *testing.T) {
	source := NotebookChatSource{
		DocumentID:   "doc-1",
		DocumentName: "Manual.pdf",
		Content:      "Section 1 controls an LED after motion detection. Section 2 - Photoresistor dimming. Print the current light sensor value and PWM output via serial every 200ms.",
	}
	items := buildCoverageRows([]NotebookChatSource{source}, "哪些章节明确要求实时数据输出？", discoverCoverageCandidates([]NotebookChatSource{source}, ""), true)
	var section1, section2 *CoverageItem
	for i := range items {
		switch items[i].Anchor {
		case "Section 1":
			section1 = &items[i]
		case "Section 2":
			section2 = &items[i]
		}
	}
	if section2 == nil {
		t.Fatalf("expected Section 2 candidate from multi-anchor chunk, got %#v", items)
	}
	if section2.Status != CoverageExplicit {
		t.Fatalf("expected Section 2 command-style output to be explicit, got %#v", section2)
	}
	if section1 != nil && section1.Status == CoverageExplicit {
		t.Fatalf("Section 1 must not inherit Section 2 output evidence: %#v", section1)
	}
}

func TestCommandStyleInstructionCountsAsCoverageRequirement(t *testing.T) {
	text := "Print the current light sensor value and PWM output via serial every 200ms."
	if !hasCoverageRequirementLanguage(text) {
		t.Fatalf("expected command-style instruction to count as explicit requirement: %q", text)
	}
}

func TestBuildCoverageItemsGroupsByStructureAnchor(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output every 200ms."},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 also displays PWM output values."},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 2, Content: "Section 3 turns on an LED after detection."},
	}
	items := buildCoverageItems(sources, "哪些章节明确要求实时显示或实时数据输出？")
	if len(items) < 2 {
		t.Fatalf("expected grouped coverage items, got %#v", items)
	}
	if items[0].Anchor != "Section 2" || !items[0].IsExplicit {
		t.Fatalf("expected Section 2 explicit first, got %#v", items[0])
	}
	if items[1].Anchor != "Section 3" || items[1].IsExplicit {
		t.Fatalf("expected Section 3 related but not explicit, got %#v", items[1])
	}
}

func TestDiscoverCoverageCandidatesFindsStructureItemsAcrossSources(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 0, Content: "Section 1 introduces motion detection."},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output every 200ms."},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 2, Content: "Section 4 requires Distance: xx.xx cm output."},
	}
	candidates := discoverCoverageCandidates(sources, "哪些章节明确要求实时显示或实时数据输出？")
	labels := coverageCandidateLabels(candidates)
	for _, expected := range []string{"Section 1", "Section 2", "Section 4"} {
		if !containsString(labels, expected) {
			t.Fatalf("expected candidate %q in %#v", expected, labels)
		}
	}
}

func TestDiscoverCoverageCandidatesUsesSectionPathFallback(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Policy.pdf", PageNumber: 3, SectionPath: []string{"Policy", "Audit Logging"}, Content: "The system must record changes."},
	}
	candidates := discoverCoverageCandidates(sources, "哪些章节涉及审计日志？")
	if len(candidates) != 1 || candidates[0].Anchor != "Audit Logging" {
		t.Fatalf("expected section path fallback candidate, got %#v", candidates)
	}
}

func TestExplicitCoverageSignalsRejectsControlOnlyEvidence(t *testing.T) {
	controlOnly := "Section 1 detects motion and turns on an LED for 3 seconds."
	if signals := explicitCoverageSignals(controlOnly, "哪些章节明确要求实时数据输出？"); len(signals) != 0 {
		t.Fatalf("control-only evidence should not be explicit output evidence, got %#v", signals)
	}
	outputEvidence := "Section 2 must print light sensor value and PWM output to the Serial Monitor every 200ms."
	signals := explicitCoverageSignals(outputEvidence, "哪些章节明确要求实时数据输出？")
	for _, expected := range []string{"serial monitor", "output", "every"} {
		if !containsString(signals, expected) {
			t.Fatalf("expected signal %q in %#v", expected, signals)
		}
	}
}

func TestCoverageAnchorForSourceExtractsGenericStructureAnchors(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		expected string
	}{
		{name: "lab session", content: "Lab Session 4 requires serial output.", expected: "Lab Session 4"},
		{name: "table", content: "Table 2 lists retention controls.", expected: "Table 2"},
		{name: "requirement", content: "Requirement 3 requires audit logging.", expected: "Requirement 3"},
		{name: "item", content: "Item 5 must show status.", expected: "Item 5"},
		{name: "clause", content: "Clause 7 requires notification.", expected: "Clause 7"},
		{name: "article", content: "Article 9 requires reporting.", expected: "Article 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := NotebookChatSource{DocumentName: "Manual.pdf", Content: tc.content}
			if got := coverageAnchorForSource(source); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestCoverageAnchorForSourceFallsBackToSectionPathPageAndDocument(t *testing.T) {
	if got := coverageAnchorForSource(NotebookChatSource{SectionPath: []string{"Chapter 1", "Outputs"}}); got != "Outputs" {
		t.Fatalf("expected section path fallback, got %q", got)
	}
	if got := coverageAnchorForSource(NotebookChatSource{PageNumber: 2}); got != "Page 3" {
		t.Fatalf("expected page fallback, got %q", got)
	}
	if got := coverageAnchorForSource(NotebookChatSource{PageNumber: -1, DocumentName: "Manual.pdf"}); got != "Manual.pdf" {
		t.Fatalf("expected document fallback, got %q", got)
	}
}

func TestBuildCoverageItemsKeepsThreeStrongestEvidenceChunks(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output.", Score: 0.1},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires LCD display.", Score: 0.9},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires output format.", Score: 0.8},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires refresh frequency.", Score: 0.7},
	}
	items := buildCoverageItems(sources, "哪些章节明确要求实时显示或实时数据输出？")
	if len(items) != 1 {
		t.Fatalf("expected one coverage item, got %#v", items)
	}
	if len(items[0].Evidence) != 3 {
		t.Fatalf("expected three strongest evidence chunks, got %#v", items[0].Evidence)
	}
	for _, evidence := range items[0].Evidence {
		if evidence.Score == 0.1 {
			t.Fatalf("lowest-scoring evidence should have been trimmed, got %#v", items[0].Evidence)
		}
	}
}

func TestBuildCoverageMatrixVerifiesEachCandidateSeparately(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 0, Content: "Section 1 turns on an LED after detection.", CitationID: "E1"},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output every 200ms.", CitationID: "E2"},
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 3, Content: "Section 4 must print Distance: xx.xx cm every loop cycle.", CitationID: "E4"},
	}
	matrix := buildCoverageMatrix(sources, "哪些章节明确要求实时显示或实时数据输出？")
	if len(matrix.Items) != 3 {
		t.Fatalf("expected three coverage rows, got %#v", matrix.Items)
	}
	if matrix.Items[0].Anchor != "Section 2" || !matrix.Items[0].IsExplicit {
		t.Fatalf("expected Section 2 explicit first, got %#v", matrix.Items[0])
	}
	if matrix.Items[1].Anchor != "Section 4" || !matrix.Items[1].IsExplicit {
		t.Fatalf("expected Section 4 explicit second, got %#v", matrix.Items[1])
	}
	if matrix.Items[2].Anchor != "Section 1" || matrix.Items[2].IsExplicit {
		t.Fatalf("expected Section 1 related but not explicit, got %#v", matrix.Items[2])
	}
}

func TestCoverageMatrixPreservesMissingFields(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output.", CitationID: "E2"},
	}
	matrix := buildCoverageMatrix(sources, "哪些章节明确要求实时显示或实时数据输出？请列出格式、频率和触发条件。")
	if len(matrix.Items) != 1 {
		t.Fatalf("expected one coverage row, got %#v", matrix.Items)
	}
	for _, expected := range []string{"format", "frequency", "trigger"} {
		if !containsString(matrix.Items[0].MissingFields, expected) {
			t.Fatalf("expected missing field %q in %#v", expected, matrix.Items[0].MissingFields)
		}
	}
}

func TestCoverageMatrixTreatsOutputShapeAsFormatEvidence(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "colon template", content: "Section 4 must print Distance: xx.xx cm every loop cycle."},
		{name: "equals template", content: "Section 4 must print Distance = xx.xx cm every loop cycle."},
		{name: "unit phrase", content: "Section 4 must print distance in cm every loop cycle."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sources := []NotebookChatSource{
				{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 3, Content: tc.content, CitationID: "E4"},
			}
			matrix := buildCoverageMatrix(sources, "Section 4 明确要求的输出格式是什么？")
			if len(matrix.Items) != 1 {
				t.Fatalf("expected one coverage row, got %#v", matrix.Items)
			}
			if containsString(matrix.Items[0].MissingFields, "format") {
				t.Fatalf("expected %q to count as format evidence, got %#v", tc.content, matrix.Items[0].MissingFields)
			}
		})
	}
}

func TestCoverageMatrixDoesNotTreatGenericColonProseAsFormat(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "note prose", content: "Note: Section 4 requires serial output."},
		{name: "section prose", content: "Section 4: requires serial output."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sources := []NotebookChatSource{
				{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 3, Content: tc.content, CitationID: "E4"},
			}
			matrix := buildCoverageMatrix(sources, "Section 4 明确要求的输出格式是什么？")
			if len(matrix.Items) != 1 {
				t.Fatalf("expected one coverage row, got %#v", matrix.Items)
			}
			if !containsString(matrix.Items[0].MissingFields, "format") {
				t.Fatalf("expected %q to keep format missing, got %#v", tc.content, matrix.Items[0].MissingFields)
			}
		})
	}
}

func TestFormatCoverageMatrixForPromptIncludesMissingFields(t *testing.T) {
	matrix := CoverageMatrix{Items: []CoverageItem{
		{
			Anchor: "Section 2", DocumentName: "Manual.pdf", IsExplicit: true,
			Signals:       []string{"serial monitor"},
			MissingFields: []string{"format", "frequency"},
			Evidence:      []NotebookChatSource{{CitationID: "E2", Content: "Section 2 requires Serial Monitor output."}},
		},
	}}
	block := formatCoverageMatrixForPrompt(matrix)
	for _, expected := range []string{"## Coverage Matrix", "Explicit items", "Section 2", "missing=format, frequency", "[E2]"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected coverage matrix prompt to contain %q, got:\n%s", expected, block)
		}
	}
}

func TestNotebookPromptUsesCoverageMatrixForCoverageQuestions(t *testing.T) {
	svc := &notebookChatService{}
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Manual.pdf", PageNumber: 1, Content: "Section 2 requires Serial Monitor output.", CitationID: ""},
	}
	prompt := svc.buildPrompt(nil, nil, sources, "哪些章节明确要求实时显示或实时数据输出？", "")
	if !strings.Contains(prompt, "## Coverage Matrix") {
		t.Fatalf("expected coverage matrix in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Use Coverage Matrix Explicit items as the main answer set") {
		t.Fatalf("expected coverage matrix instruction in prompt, got:\n%s", prompt)
	}
}

func TestFormatComparisonMatrixForPromptBindsCellsToEvidence(t *testing.T) {
	matrix := ComparisonMatrix{
		Subjects:   []ComparisonSubject{{Label: "Lab Session 3"}, {Label: "Lab Session 5"}},
		Dimensions: []ComparisonDimension{{Name: "主要传感器"}, {Name: "显示内容"}},
		Cells: []ComparisonCell{
			{SubjectLabel: "Lab Session 3", Dimension: "主要传感器", Status: "supported", Evidence: []NotebookChatSource{{CitationID: "E1"}}},
			{SubjectLabel: "Lab Session 5", Dimension: "主要传感器", Status: "supported", Evidence: []NotebookChatSource{{CitationID: "E2"}}},
			{SubjectLabel: "Lab Session 3", Dimension: "显示内容", Status: "missing"},
		},
	}
	block := formatComparisonMatrixForPrompt(matrix)
	for _, expected := range []string{"## Comparison Matrix", "Do not use evidence across subjects", "Lab Session 3", "主要传感器 evidence=[E1]", "显示内容 status=missing"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected comparison matrix block to contain %q, got:\n%s", expected, block)
		}
	}
}

func TestNotebookPromptUsesComparisonMatrixForComparisonQuestions(t *testing.T) {
	svc := &notebookChatService{}
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", Content: "Lab Session 3 uses DHT11.", CitationID: ""},
		{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", Content: "Lab Session 5 uses IR speed sensor.", CitationID: ""},
	}
	prompt := svc.buildPrompt(nil, nil, sources, "请比较 Lab Session 3 和 Lab Session 5 的主要传感器", "")
	if !strings.Contains(prompt, "## Comparison Matrix") {
		t.Fatalf("expected comparison matrix in prompt, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not use evidence across subjects") {
		t.Fatalf("expected anti-cross-evidence instruction, got:\n%s", prompt)
	}
}

func TestFormatCoverageItemsForPromptSeparatesExplicitAndRelated(t *testing.T) {
	items := []CoverageItem{
		{
			Anchor: "Section 2", DocumentName: "Manual.pdf", IsExplicit: true,
			Signals:  []string{"serial monitor", "every"},
			Evidence: []NotebookChatSource{{CitationID: "E1", Content: "Section 2 requires Serial Monitor output every 200ms."}},
		},
		{
			Anchor: "Section 1", DocumentName: "Manual.pdf", IsExplicit: false,
			Evidence: []NotebookChatSource{{CitationID: "E2", Content: "Section 1 turns on an LED."}},
		},
	}
	block := formatCoverageItemsForPrompt(items)
	for _, expected := range []string{"## Coverage Items", "Explicit items", "Section 2", "signals=serial monitor, every", "[E1]", "Related but not explicit", "Section 1"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected coverage prompt block to contain %q, got:\n%s", expected, block)
		}
	}
}

func TestFilterSelectedNotebookDocumentsKeepsOnlyIntersection(t *testing.T) {
	filtered, ok := filterSelectedNotebookDocuments([]string{"doc-1", "doc-2"}, []string{"doc-2", "doc-3"})
	if !ok || len(filtered) != 1 || filtered[0] != "doc-2" {
		t.Fatalf("expected only selected notebook document, got filtered=%#v ok=%v", filtered, ok)
	}
}

func TestFilterSelectedNotebookDocumentsRejectsEmptyIntersection(t *testing.T) {
	filtered, ok := filterSelectedNotebookDocuments([]string{"doc-1"}, []string{"missing"})
	if ok || len(filtered) != 0 {
		t.Fatalf("expected invalid selection to reject full-notebook fallback, got filtered=%#v ok=%v", filtered, ok)
	}
}

func TestPromptForConstrainedSynthesisForbidsUnlistedDevicesAndImplicitActuators(t *testing.T) {
	prompt := formatAnswerModeInstructions(NotebookAnswerModeDesignSynthesis)
	for _, expected := range []string{"不得引入文档外实体", "unlisted devices", "隐含执行器", "必须先列出 Allowed items", "不要写“可使用其他设备”"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected constrained synthesis prompt to contain %q, got:\n%s", expected, prompt)
		}
	}
	for _, forbidden := range []string{"风扇", "加湿器", "报警器", "Lab Session 6"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("constrained synthesis prompt should not contain sample-specific term %q:\n%s", forbidden, prompt)
		}
	}
}

func TestFormatAllowedInventoryForConstrainedSynthesisUsesOnlyEvidenceAndGuideItems(t *testing.T) {
	contexts := []SelectedDocumentContext{{
		DocumentName: "Brief.pdf",
		Summary:      "Includes approval workflow, audit log, reviewer role.",
		KeyPoints:    []string{"Export reports", "Email notification"},
	}}
	sources := []NotebookChatSource{{Content: "Components: dashboard, audit trail, CSV export. Requirements: reviewer approves requests."}}
	block := formatAllowedInventoryForPrompt(contexts, sources, NotebookAnswerModeDesignSynthesis)
	for _, expected := range []string{"Core allowed items", "Background hints", "approval workflow", "audit trail", "CSV export", "reviewer"} {
		if !strings.Contains(block, expected) {
			t.Fatalf("expected inventory block to contain %q, got:\n%s", expected, block)
		}
	}
}

func TestFormatAllowedInventoryKeepsGuideItemsAsBackgroundOnly(t *testing.T) {
	contexts := []SelectedDocumentContext{{
		DocumentName: "Guide.pdf",
		Summary:      "The guide mentions fan and humidifier as possible extensions.",
		KeyPoints:    []string{"Use alarm device for warning"},
	}}
	sources := []NotebookChatSource{{
		Content: "Components: LED light, LCD screen, servo motor. Requirements: output status to Serial Monitor.",
	}}
	block := formatAllowedInventoryForPrompt(contexts, sources, NotebookAnswerModeDesignSynthesis)
	if !strings.Contains(block, "Core allowed items") || !strings.Contains(block, "LED light") {
		t.Fatalf("expected evidence-backed core inventory, got:\n%s", block)
	}
	if !strings.Contains(block, "Background hints") || !strings.Contains(block, "fan and humidifier") {
		t.Fatalf("expected guide content only as background hints, got:\n%s", block)
	}
	coreSection := strings.Split(block, "Background hints")[0]
	if strings.Contains(coreSection, "humidifier") || strings.Contains(coreSection, "alarm device") {
		t.Fatalf("guide-only items must not enter core inventory, got:\n%s", block)
	}
}

func TestTrimRunesDoesNotSplitChineseText(t *testing.T) {
	value := trimRunes("温度湿度传感器", 4)
	if value != "温度湿度" {
		t.Fatalf("expected rune-safe truncation, got %q", value)
	}
}

func TestInventoryCandidateSentencesSplitsEnglishPeriods(t *testing.T) {
	content := "Components: LED light. Requirements: output status to Serial Monitor. Notes: optional appendix."
	candidates := inventoryCandidateSentences(content)
	for _, expected := range []string{"Components: LED light", "Requirements: output status to Serial Monitor"} {
		if !containsString(candidates, expected) {
			t.Fatalf("expected candidate %q in %#v", expected, candidates)
		}
	}
	for _, candidate := range candidates {
		if strings.Contains(candidate, "LED light") && strings.Contains(candidate, "Serial Monitor") {
			t.Fatalf("expected English period split to keep candidates separate, got %#v", candidates)
		}
	}
}

func TestPromptForConstraintCheckRequiresAnsweringOutputFormatAndFrequency(t *testing.T) {
	prompt := formatAnswerModeInstructions(NotebookAnswerModeConstraintCheck)
	for _, expected := range []string{"输出方式", "格式", "频率", "触发条件", "逐项回答用户问题"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected constraint prompt to contain %q, got:\n%s", expected, prompt)
		}
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

func TestCitationGuardKeepsOpenSynthesisInsteadOfFailingClosed(t *testing.T) {
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
		Question: "根据这两个实验，LCD 的共同作用是什么？",
		DocumentContexts: []SelectedDocumentContext{
			{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", Summary: "Uses LCD to show temperature and humidity."},
			{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", Summary: "Uses LCD to show RPM."},
		},
		Sources: []NotebookChatSource{
			{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 5, Content: "LCD displays temperature and humidity."},
			{DocumentID: "doc-2", DocumentName: "Tutorial 2.pdf", PageNumber: 0, Content: "I2C-LCD displays RPM."},
		},
	}
	answer := "文档明确说明：Tutorial 1 使用 LCD 显示温度和湿度；Tutorial 2 使用 LCD 显示 RPM。[E1][E2]\n\n根据文档推断：LCD 在两个实验中都承担实时反馈实验状态或测量结果的作用。"

	rendered := svc.applyCitationGuard(context.Background(), answer, input)

	if strings.Contains(rendered, "do not contain sufficient information") {
		t.Fatalf("open synthesis should not be rewritten to insufficient answer: %s", rendered)
	}
	if !strings.Contains(rendered, "根据文档推断") {
		t.Fatalf("expected inference wording to remain, got: %s", rendered)
	}
}

func TestCitationGuardDoesNotRewriteSupportedConstraintAnswerToInsufficientWhenCitationMissing(t *testing.T) {
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
		Question: "Lab Session 3 是否只需要串口输出？",
		Sources: []NotebookChatSource{
			{DocumentID: "doc-1", DocumentName: "Tutorial 1.pdf", PageNumber: 5, Content: "Lab Session 3 requires DHT11 data to be shown on a 16x2 I2C LCD and printed to Serial Monitor."},
		},
	}
	answer := "这个说法不对。Lab Session 3 不只要求串口输出，还要求在 16x2 I2C LCD 上显示温度和湿度，同时通过 Serial Monitor 输出。"

	rendered := svc.applyCitationGuard(context.Background(), answer, input)

	if strings.Contains(rendered, "do not contain sufficient information") {
		t.Fatalf("supported constraint answer should not be rewritten to insufficient: %s", rendered)
	}
	if !strings.Contains(rendered, "16x2 I2C LCD") {
		t.Fatalf("expected original supported answer to remain, got: %s", rendered)
	}
}

func TestEvidencePackHasQuestionRelevantEvidenceMatchesAnchorsAndKeywords(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "Section 4 requires serial output. Distance: xx.xx cm is printed every loop cycle."}}}
	if !evidencePackHasQuestionRelevantEvidence("在 Section 4 中明确要求的距离输出方式是什么？", pack) {
		t.Fatal("expected evidence to be relevant to output-format question")
	}
}

func TestEvidencePackRelevanceRequiresSubjectMatchEvenWithAnchorForOutputQuestion(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "Section 4 requires Serial Monitor output."}}}
	if evidencePackHasQuestionRelevantEvidence("在 Section 4 中明确要求的距离输出方式是什么？", pack) {
		t.Fatal("expected anchor-only generic output evidence to be insufficient for distance output-format question")
	}
}

func TestEvidencePackRelevanceAllowsAnchorWithSubjectForOutputQuestion(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "Section 4 prints Distance: xx.xx cm to Serial Monitor."}}}
	if !evidencePackHasQuestionRelevantEvidence("在 Section 4 中明确要求的距离输出方式是什么？", pack) {
		t.Fatal("expected anchor plus distance output evidence to be relevant")
	}
}

func TestRelevantQuestionTermsExtractsChineseDomainTerms(t *testing.T) {
	terms := relevantQuestionTerms("文档是否要求必须使用LCD显示距离并通过串口输出格式？")
	for _, expected := range []string{"lcd", "显示", "距离", "串口", "输出", "格式"} {
		if !containsString(terms, expected) {
			t.Fatalf("expected term %q in %#v", expected, terms)
		}
	}
}

func TestEvidencePackRelevanceHandlesUnspacedChineseQuestion(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "The system prints Distance: xx.xx cm to the Serial Monitor every loop cycle."}}}
	if !evidencePackHasQuestionRelevantEvidence("文档是否要求显示距离并说明输出格式？", pack) {
		t.Fatal("expected Chinese output-format question to match relevant evidence")
	}
}

func TestEvidencePackRelevanceRejectsGenericOutputSignalWithoutSubject(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "The dashboard display shows approval status output for reviewers."}}}
	if evidencePackHasQuestionRelevantEvidence("文档是否要求显示距离并说明输出格式？", pack) {
		t.Fatal("expected generic output/display evidence without the question subject to be rejected")
	}
}

func TestCapNotebookSourcesKeepsRequestedTopK(t *testing.T) {
	sources := []NotebookChatSource{
		{DocumentID: "doc-1", Score: 0.9},
		{DocumentID: "doc-2", Score: 0.8},
	}
	capped := capNotebookSources(sources, 1)
	if len(capped) != 1 || capped[0].DocumentID != "doc-1" {
		t.Fatalf("expected top 1 source, got %#v", capped)
	}
}

func TestRepairPromptForbidsInsufficientWhenRelevantEvidenceExists(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "The document requires LCD display and Serial Monitor output."}}}
	prompt := buildCitationRepairPrompt("是否只需要 Serial Monitor？", "old answer", pack, CitationGuardResult{Issues: []CitationGuardIssue{{Type: "missing_paragraph_citation", Detail: "missing"}}})
	if !strings.Contains(prompt, "Do NOT replace a supported answer") {
		t.Fatalf("expected repair prompt to preserve supported answers, got:\n%s", prompt)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func structureEvidenceContents(values []StructureEvidence) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Source.Content)
	}
	return out
}

func hasExactPhrase(candidates []ExactPhraseCandidate, phrase, confidence string) bool {
	for _, candidate := range candidates {
		if candidate.Phrase == phrase && candidate.Confidence == confidence {
			return true
		}
	}
	return false
}

func coverageCandidateLabels(candidates []CoverageCandidate) []string {
	labels := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		labels = append(labels, candidate.Anchor)
	}
	return labels
}
