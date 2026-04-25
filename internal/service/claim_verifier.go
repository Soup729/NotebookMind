package service

import (
	"regexp"
	"strings"
)

type Claim struct {
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidence_ids"`
	Numbers     []string `json:"numbers,omitempty"`
	Calculation string   `json:"calculation,omitempty"`
	Confidence  string   `json:"confidence"`
}

type DraftAnswer struct {
	Claims []Claim `json:"claims"`
	Answer string  `json:"answer"`
}

type VerificationIssue struct {
	Type       string
	ClaimText  string
	EvidenceID string
	Detail     string
}

type VerificationResult struct {
	Passed bool
	Issues []VerificationIssue
}

var trustCitationPattern = regexp.MustCompile(`(?i)\[Source:\s*([^,\]]+),\s*Page\s+(\d+)\]`)

func VerifyDraftAnswer(draft DraftAnswer, pack EvidencePack) VerificationResult {
	var issues []VerificationIssue
	if strings.TrimSpace(draft.Answer) == "" {
		issues = append(issues, VerificationIssue{Type: "empty_answer", Detail: "draft answer is empty"})
	}
	for _, claim := range draft.Claims {
		if len(claim.EvidenceIDs) == 0 {
			issues = append(issues, VerificationIssue{Type: "missing_evidence", ClaimText: claim.Text, Detail: "claim has no evidence ids"})
			continue
		}
		evidenceText, missing := citedEvidenceText(claim.EvidenceIDs, pack)
		for _, id := range missing {
			issues = append(issues, VerificationIssue{Type: "unknown_evidence", ClaimText: claim.Text, EvidenceID: id, Detail: "evidence id not found"})
		}
		if len(missing) > 0 {
			continue
		}
		for _, number := range claim.Numbers {
			if !numberSupported(number, evidenceText) && !calculationSupported(number, claim.Calculation, evidenceText) {
				issues = append(issues, VerificationIssue{Type: "unsupported_number", ClaimText: claim.Text, Detail: "number " + number + " not found in cited evidence"})
			}
		}
	}
	for _, citation := range trustCitationPattern.FindAllStringSubmatch(draft.Answer, -1) {
		if len(citation) < 3 {
			continue
		}
		if !packHasCitation(pack, citation[1], citation[2]) {
			issues = append(issues, VerificationIssue{Type: "invalid_citation", Detail: citation[0]})
		}
	}
	return VerificationResult{Passed: len(issues) == 0, Issues: issues}
}

func citedEvidenceText(ids []string, pack EvidencePack) (string, []string) {
	var builder strings.Builder
	var missing []string
	for _, id := range ids {
		item, ok := pack.SourceByID(id)
		if !ok {
			missing = append(missing, id)
			continue
		}
		builder.WriteString(item.Content)
		builder.WriteString(" ")
	}
	return normalizeTrustNumberText(builder.String()), missing
}

func numberSupported(number string, evidence string) bool {
	n := normalizeTrustNumberText(number)
	if n == "" {
		return true
	}
	return strings.Contains(evidence, n)
}

func calculationSupported(number string, calculation string, evidence string) bool {
	if strings.TrimSpace(calculation) == "" {
		return false
	}
	for _, token := range extractTrustNumbers(calculation) {
		normalized := normalizeTrustNumberText(token)
		if normalized == normalizeTrustNumberText(number) {
			continue
		}
		if strings.Contains(evidence, normalized) {
			return true
		}
	}
	return false
}

func normalizeTrustNumberText(text string) string {
	replacer := strings.NewReplacer(",", "", "$", "", "€", "", "£", "", "¥", "", "%", "", " million", "m", " billion", "b")
	return strings.ToLower(replacer.Replace(strings.Join(strings.Fields(text), " ")))
}

func extractTrustNumbers(text string) []string {
	re := regexp.MustCompile(`\d+(?:,\d{3})*(?:\.\d+)?`)
	return re.FindAllString(text, -1)
}

func packHasCitation(pack EvidencePack, docName string, page string) bool {
	docName = strings.TrimSpace(strings.ToLower(docName))
	for _, item := range pack.Items {
		if strings.ToLower(item.DocumentName) == docName && page == int64ToString(item.PageNumber+1) {
			return true
		}
	}
	return false
}

func int64ToString(v int64) string {
	if v == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}
