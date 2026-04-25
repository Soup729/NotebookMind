package service

import (
	"strings"

	"NotebookAI/internal/models"
)

type QueryRisk string

const (
	QueryRiskTable       QueryRisk = "table"
	QueryRiskProcedure   QueryRisk = "procedure"
	QueryRiskRiskList    QueryRisk = "risk_list"
	QueryRiskComparison  QueryRisk = "comparison"
	QueryRiskCalculation QueryRisk = "calculation"
	QueryRiskMultimodal  QueryRisk = "multimodal"
	QueryRiskFollowup    QueryRisk = "followup"
)

type TrustPlan struct {
	OriginalQuestion string
	StandaloneQuery  string
	Intent           QueryIntent
	Risks            []QueryRisk
	NeedsParentBlock bool
	NeedsFullTable   bool
	NeedsRepair      bool
	TopK             int
}

func BuildTrustPlan(question string, history []models.ChatMessage) TrustPlan {
	plan := TrustPlan{
		OriginalQuestion: question,
		StandaloneQuery:  strings.TrimSpace(question),
		Intent:           IntentUnknown,
		TopK:             5,
	}
	normalized := strings.ToLower(question)

	if hasAny(normalized, "table", "breakdown", "quarterly", "margin", "product line", "expenditure") {
		plan.addRisk(QueryRiskTable)
		plan.NeedsFullTable = true
		plan.NeedsParentBlock = true
	}
	if hasAny(normalized, "workflow", "procedure", "process", "approval", "procurement", "lifecycle", "步骤", "流程") {
		plan.addRisk(QueryRiskProcedure)
		plan.NeedsParentBlock = true
	}
	if hasAny(normalized, "risk factor", "risk factors", "likelihood", "impact severity", "risk assessment") {
		plan.addRisk(QueryRiskRiskList)
		plan.NeedsParentBlock = true
	}
	if hasAny(normalized, "compare", "comparing", "difference", "versus", "vs", "between", "对比", "比较") {
		plan.addRisk(QueryRiskComparison)
	}
	if hasAny(normalized, "percentage", "ratio", "average", "per employee", "growth rate", "calculate", "margin", "%", "多少") {
		plan.addRisk(QueryRiskCalculation)
	}
	if hasAny(normalized, "chart", "graph", "pie chart", "bar chart", "org chart", "image", "shown") {
		plan.addRisk(QueryRiskMultimodal)
		plan.NeedsParentBlock = true
	}
	if len(history) > 0 && hasAny(normalized, "that", "this", "those", "these", "it", "they", "among those", "represent") {
		plan.addRisk(QueryRiskFollowup)
	}

	if plan.HasHighRisk() {
		plan.TopK = 8
		plan.NeedsRepair = true
	}
	return plan
}

func (p TrustPlan) HasRisk(risk QueryRisk) bool {
	for _, r := range p.Risks {
		if r == risk {
			return true
		}
	}
	return false
}

func (p TrustPlan) HasHighRisk() bool {
	return len(p.Risks) > 0
}

func (p *TrustPlan) addRisk(risk QueryRisk) {
	if p.HasRisk(risk) {
		return
	}
	p.Risks = append(p.Risks, risk)
}

func hasAny(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
