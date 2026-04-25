package service

import (
	"context"
	"strings"
	"testing"

	"NotebookAI/internal/models"
)

func TestParseDraftAnswerStripsMarkdownFence(t *testing.T) {
	raw := "```json\n{\"claims\":[{\"text\":\"Total revenue was $1,850M.\",\"evidence_ids\":[\"E1\"],\"numbers\":[\"1850\"],\"confidence\":\"high\"}],\"answer\":\"Total revenue was $1,850M [Source: Financials.pdf, Page 1].\"}\n```"
	draft, err := parseDraftAnswer(raw)
	if err != nil {
		t.Fatalf("parseDraftAnswer returned error: %v", err)
	}
	if len(draft.Claims) != 1 || draft.Claims[0].EvidenceIDs[0] != "E1" {
		t.Fatalf("unexpected draft: %#v", draft)
	}
}

func TestBuildReasoningPromptRequiresStrictJSONAndEvidenceIDs(t *testing.T) {
	prompt := buildReasoningPrompt(TrustPlan{OriginalQuestion: "What is revenue?"}, EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Financials.pdf", PageNumber: 0, Content: "Revenue was $1,850M."}}})
	if !strings.Contains(prompt, "Return strict JSON only") || !strings.Contains(prompt, "evidence_ids") || !strings.Contains(prompt, "[E1]") {
		t.Fatalf("prompt missing strict JSON/evidence requirements:\n%s", prompt)
	}
}

func TestRunTrustWorkflowRepairsUnsupportedDraft(t *testing.T) {
	calls := 0
	workflow := &trustWorkflow{
		generate: func(_ context.Context, prompt string) (string, error) {
			calls++
			if calls == 1 {
				return `{"claims":[{"text":"Approval threshold is $500K.","evidence_ids":["E1"],"numbers":["500"],"confidence":"high"}],"answer":"Approval threshold is $500K [Source: Policy.pdf, Page 1]."}`, nil
			}
			return `{"claims":[{"text":"The retrieved evidence does not explicitly provide the approval threshold.","evidence_ids":["E1"],"confidence":"low"}],"answer":"The provided documents do not contain sufficient information to answer this question. [Source: Policy.pdf, Page 1]"}`, nil
		},
		maxRepairAttempts: 1,
	}
	output, err := workflow.Run(context.Background(), TrustWorkflowInput{
		Question: "What is the approval threshold?",
		History:  nil,
		SearchResults: []HybridResult{{
			ChunkID:    "chunk-1",
			DocumentID: "doc-1",
			Content:    "Procurement policy requires finance review.",
			Metadata:   map[string]interface{}{"page_number": 0, "chunk_type": "procedure"},
		}},
		Sources: []NotebookChatSource{{DocumentID: "doc-1", DocumentName: "Policy.pdf", PageNumber: 0}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one repair call, got %d calls", calls)
	}
	if !strings.Contains(output.Answer, "sufficient information") {
		t.Fatalf("expected repaired insufficient answer, got %q", output.Answer)
	}
	if !output.Verification.Passed {
		t.Fatalf("expected repaired answer to pass verification: %#v", output.Verification.Issues)
	}
}

func TestTrustPlanHighRiskOnly(t *testing.T) {
	plan := BuildTrustPlan("Summarize the annual report", []models.ChatMessage{})
	if plan.HasHighRisk() {
		t.Fatalf("plain summary should not be high-risk: %#v", plan.Risks)
	}
}
