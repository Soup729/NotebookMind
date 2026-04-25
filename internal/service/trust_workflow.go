package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"NotebookAI/internal/models"
	"github.com/tmc/langchaingo/llms"
)

type TrustWorkflow interface {
	Run(ctx context.Context, input TrustWorkflowInput) (TrustWorkflowOutput, error)
}

type TrustWorkflowInput struct {
	Question      string
	UserID        string
	SessionID     string
	NotebookID    string
	DocumentIDs   []string
	History       []models.ChatMessage
	SearchResults []HybridResult
	Sources       []NotebookChatSource
}

type TrustWorkflowOutput struct {
	Answer       string
	Sources      []NotebookChatSource
	Plan         TrustPlan
	EvidencePack EvidencePack
	Verification VerificationResult
	Repaired     bool
}

type trustWorkflow struct {
	hybridSearch      HybridSearchService
	llm               llms.Model
	maxRepairAttempts int
	generate          func(ctx context.Context, prompt string) (string, error)
}

func NewTrustWorkflow(hybridSearch HybridSearchService, llm llms.Model, maxRepairAttempts int) TrustWorkflow {
	if maxRepairAttempts < 0 {
		maxRepairAttempts = 0
	}
	w := &trustWorkflow{
		hybridSearch:      hybridSearch,
		llm:               llm,
		maxRepairAttempts: maxRepairAttempts,
	}
	w.generate = func(ctx context.Context, prompt string) (string, error) {
		return llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	}
	return w
}

func (w *trustWorkflow) Run(ctx context.Context, input TrustWorkflowInput) (TrustWorkflowOutput, error) {
	plan := BuildTrustPlan(input.Question, input.History)
	results := input.SearchResults
	if len(results) == 0 && w.hybridSearch != nil {
		var err error
		results, err = w.hybridSearch.SearchWithOptions(ctx, HybridSearchOptions{
			Query:       plan.StandaloneQuery,
			UserID:      input.UserID,
			SessionID:   input.SessionID,
			NotebookID:  input.NotebookID,
			DocumentIDs: input.DocumentIDs,
			TopK:        plan.TopK,
		})
		if err != nil {
			return TrustWorkflowOutput{}, fmt.Errorf("trust workflow search: %w", err)
		}
	}

	pack := BuildEvidencePack(results, input.Sources, plan)
	if len(pack.Items) == 0 {
		answer := "The provided documents do not contain sufficient information to answer this question."
		return TrustWorkflowOutput{Answer: answer, Sources: input.Sources, Plan: plan, EvidencePack: pack, Verification: VerificationResult{Passed: true}}, nil
	}

	draft, err := w.generateDraft(ctx, buildReasoningPrompt(plan, pack))
	if err != nil {
		return TrustWorkflowOutput{}, err
	}
	verification := VerifyDraftAnswer(draft, pack)
	if verification.Passed {
		return TrustWorkflowOutput{Answer: strings.TrimSpace(draft.Answer), Sources: input.Sources, Plan: plan, EvidencePack: pack, Verification: verification}, nil
	}

	for attempt := 0; attempt < w.maxRepairAttempts; attempt++ {
		repairPrompt := buildRepairPrompt(plan, pack, draft, verification)
		draft, err = w.generateDraft(ctx, repairPrompt)
		if err != nil {
			return TrustWorkflowOutput{}, err
		}
		verification = VerifyDraftAnswer(draft, pack)
		if verification.Passed {
			return TrustWorkflowOutput{Answer: strings.TrimSpace(draft.Answer), Sources: input.Sources, Plan: plan, EvidencePack: pack, Verification: verification, Repaired: true}, nil
		}
	}

	fallback := citedInsufficientAnswer(pack)
	return TrustWorkflowOutput{Answer: fallback, Sources: input.Sources, Plan: plan, EvidencePack: pack, Verification: VerificationResult{Passed: true}, Repaired: true}, nil
}

func (w *trustWorkflow) generateDraft(ctx context.Context, prompt string) (DraftAnswer, error) {
	if w.generate == nil {
		return DraftAnswer{}, fmt.Errorf("trust workflow generator is nil")
	}
	raw, err := w.generate(ctx, prompt)
	if err != nil {
		return DraftAnswer{}, err
	}
	return parseDraftAnswer(raw)
}

func buildReasoningPrompt(plan TrustPlan, pack EvidencePack) string {
	return fmt.Sprintf(`You are a grounded answer planner for an enterprise document QA system.

Return strict JSON only. Do not use markdown fences.

JSON schema:
{
  "claims": [
    {
      "text": "one factual claim",
      "evidence_ids": ["E1"],
      "numbers": ["optional numeric strings copied from evidence"],
      "calculation": "optional calculation using evidence numbers",
      "confidence": "high|medium|low"
    }
  ],
  "answer": "final answer with [Source: DocumentName, Page X] citations"
}

Rules:
- Every factual claim must list evidence_ids from the Evidence section.
- Do not invent missing table rows, workflow thresholds, risk ratings, chart values, names, or dates.
- For calculations, include the source operands in numbers and calculation.
- If evidence is insufficient, say so in the answer and cite the closest evidence item.
- Final answer must use [Source: DocumentName, Page X] citations.

Question:
%s

Evidence:
%s
`, plan.OriginalQuestion, pack.FormatForPrompt())
}

func buildRepairPrompt(plan TrustPlan, pack EvidencePack, draft DraftAnswer, verification VerificationResult) string {
	issueLines := make([]string, 0, len(verification.Issues))
	for _, issue := range verification.Issues {
		issueLines = append(issueLines, fmt.Sprintf("- %s: %s %s", issue.Type, issue.ClaimText, issue.Detail))
	}
	draftJSON, _ := json.Marshal(draft)
	return fmt.Sprintf(`Repair the draft answer so every claim is supported.

Return strict JSON only using the same schema.

Question:
%s

Evidence:
%s

Verifier issues:
%s

Previous draft:
%s
`, plan.OriginalQuestion, pack.FormatForPrompt(), strings.Join(issueLines, "\n"), string(draftJSON))
}

func parseDraftAnswer(raw string) (DraftAnswer, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	var draft DraftAnswer
	if err := json.Unmarshal([]byte(cleaned), &draft); err != nil {
		return DraftAnswer{}, fmt.Errorf("parse draft answer: %w", err)
	}
	return draft, nil
}

func citedInsufficientAnswer(pack EvidencePack) string {
	if len(pack.Items) == 0 {
		return "The provided documents do not contain sufficient information to answer this question."
	}
	item := pack.Items[0]
	return fmt.Sprintf("The provided documents do not contain sufficient information to answer this question. [Source: %s, Page %d]", item.DocumentName, item.PageNumber+1)
}
