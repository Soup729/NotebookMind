package service

import (
	"testing"

	"NotebookAI/internal/models"
)

func TestBuildTrustPlanClassifiesHighRiskQueries(t *testing.T) {
	tests := []struct {
		name            string
		question        string
		history         []models.ChatMessage
		wantRisk        QueryRisk
		needsFullTable  bool
		needsParent     bool
		wantCalculation bool
	}{
		{
			name:            "table calculation",
			question:        "According to the R&D expenditure breakdown table, what is the ratio?",
			wantRisk:        QueryRiskTable,
			needsFullTable:  true,
			wantCalculation: true,
		},
		{
			name:        "procedure workflow",
			question:    "What is the vendor procurement and approval workflow?",
			wantRisk:    QueryRiskProcedure,
			needsParent: true,
		},
		{
			name:        "risk list",
			question:    "What were the top five risk factors, likelihood, and impact severity?",
			wantRisk:    QueryRiskRiskList,
			needsParent: true,
		},
		{
			name:            "followup calculation",
			question:        "What percentage of total revenue does that represent?",
			history:         []models.ChatMessage{{Role: "user", Content: "What was total R&D investment?"}},
			wantRisk:        QueryRiskFollowup,
			wantCalculation: true,
		},
		{
			name:     "chart",
			question: "Looking at the market share chart, who are the top competitors?",
			wantRisk: QueryRiskMultimodal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := BuildTrustPlan(tt.question, tt.history)
			if !plan.HasRisk(tt.wantRisk) {
				t.Fatalf("expected risk %q in %#v", tt.wantRisk, plan.Risks)
			}
			if tt.needsFullTable && !plan.NeedsFullTable {
				t.Fatalf("expected full table plan")
			}
			if tt.needsParent && !plan.NeedsParentBlock {
				t.Fatalf("expected parent block plan")
			}
			if tt.wantCalculation && !plan.HasRisk(QueryRiskCalculation) {
				t.Fatalf("expected calculation risk in %#v", plan.Risks)
			}
			if plan.TopK < 8 {
				t.Fatalf("expected high-risk topK >= 8, got %d", plan.TopK)
			}
		})
	}
}
