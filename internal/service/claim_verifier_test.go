package service

import "testing"

func TestVerifyDraftAnswerRejectsMissingEvidence(t *testing.T) {
	result := VerifyDraftAnswer(DraftAnswer{
		Claims: []Claim{{Text: "Revenue was $1,850M.", Numbers: []string{"1850"}, Confidence: "high"}},
		Answer: "Revenue was $1,850M.",
	}, EvidencePack{Items: []EvidenceItem{{ID: "E1", Content: "Revenue was $1,850M."}}})
	if result.Passed {
		t.Fatalf("expected missing evidence ids to fail")
	}
}

func TestVerifyDraftAnswerRejectsNumberOnlyFoundInOtherEvidence(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{
		{ID: "E1", DocumentName: "A.pdf", PageNumber: 0, Content: "Headcount was 9,320."},
		{ID: "E2", DocumentName: "B.pdf", PageNumber: 0, Content: "Revenue was $1,850M."},
	}}
	result := VerifyDraftAnswer(DraftAnswer{
		Claims: []Claim{{Text: "Revenue was $1,850M.", EvidenceIDs: []string{"E1"}, Numbers: []string{"1850"}, Confidence: "high"}},
		Answer: "Revenue was $1,850M [Source: A.pdf, Page 1].",
	}, pack)
	if result.Passed {
		t.Fatalf("expected wrong evidence citation to fail")
	}
}

func TestVerifyDraftAnswerAcceptsCalculationWithCitedOperands(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{
		{ID: "E1", DocumentName: "Financials.pdf", PageNumber: 0, Content: "R&D investment was $266.4M. Revenue was $1,850M."},
	}}
	result := VerifyDraftAnswer(DraftAnswer{
		Claims: []Claim{{
			Text:        "R&D intensity was 14.4%.",
			EvidenceIDs: []string{"E1"},
			Numbers:     []string{"266.4", "1850"},
			Calculation: "266.4 / 1850 = 14.4%",
			Confidence:  "high",
		}},
		Answer: "R&D intensity was 14.4% [Source: Financials.pdf, Page 1].",
	}, pack)
	if !result.Passed {
		t.Fatalf("expected calculation with cited operands to pass: %#v", result.Issues)
	}
}

func TestVerifyDraftAnswerAcceptsCitedInsufficientAnswer(t *testing.T) {
	pack := EvidencePack{Items: []EvidenceItem{{ID: "E1", DocumentName: "Annual.pdf", PageNumber: 0, Content: "Company overview."}}}
	result := VerifyDraftAnswer(DraftAnswer{
		Claims: []Claim{{Text: "The retrieved evidence does not explicitly provide the requested value.", EvidenceIDs: []string{"E1"}, Confidence: "low"}},
		Answer: "The provided documents do not contain sufficient information to answer this question. [Source: Annual.pdf, Page 1]",
	}, pack)
	if !result.Passed {
		t.Fatalf("expected cited insufficient answer to pass: %#v", result.Issues)
	}
}
