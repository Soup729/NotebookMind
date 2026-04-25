package service

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var evidenceIDPattern = regexp.MustCompile(`\[E(\d+)\]`)
var capitalizedPhrasePattern = regexp.MustCompile(`\b(?:[A-Z][A-Za-z0-9&.'-]*|AI|R&D|CEO|CFO|CTO|CSAT)(?:\s+(?:[A-Z][A-Za-z0-9&.'-]*|AI|R&D|CEO|CFO|CTO|CSAT)){1,}\b`)
var camelCaseEntityPattern = regexp.MustCompile(`\b[A-Z][a-z]+[A-Z][A-Za-z0-9]+\b`)

func BuildEvidencePackFromNotebookSources(sources []NotebookChatSource) EvidencePack {
	items := make([]EvidenceItem, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, src := range sources {
		content := strings.TrimSpace(src.Content)
		if content == "" {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d:%s", src.DocumentID, src.PageNumber, src.ChunkIndex, strings.Join(strings.Fields(content), " "))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		docName := strings.TrimSpace(src.DocumentName)
		if docName == "" {
			docName = "Unknown Document"
		}
		items = append(items, EvidenceItem{
			ID:           fmt.Sprintf("E%d", len(items)+1),
			DocumentID:   src.DocumentID,
			DocumentName: docName,
			PageNumber:   src.PageNumber,
			ChunkID:      fmt.Sprintf("%s:%d", src.DocumentID, src.ChunkIndex),
			ChunkType:    src.ChunkType,
			SectionPath:  src.SectionPath,
			BoundingBox:  src.BoundingBox,
			Content:      content,
		})
	}
	return EvidencePack{Items: items}
}

func RenderEvidenceCitations(answer string, pack EvidencePack) string {
	return evidenceIDPattern.ReplaceAllStringFunc(answer, func(match string) string {
		id := strings.Trim(match, "[]")
		item, ok := pack.SourceByID(id)
		if !ok {
			return match
		}
		return fmt.Sprintf("[Source: %s, Page %d]", item.DocumentName, item.PageNumber+1)
	})
}

func evidenceIDsInText(text string) []string {
	matches := evidenceIDPattern.FindAllStringSubmatch(text, -1)
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id := "E" + match[1]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.TrimPrefix(ids[i], "E"))
		right, _ := strconv.Atoi(strings.TrimPrefix(ids[j], "E"))
		return left < right
	})
	return ids
}

type CitationGuardOptions struct {
	RequireParagraphCitations bool
	ValidateNumbers           bool
	ValidateEntityPhrases     bool
	MinCitationCoverage       float64
}

type CitationGuardIssue struct {
	Type       string
	Paragraph  string
	EvidenceID string
	Detail     string
}

type CitationGuardResult struct {
	Passed           bool
	Issues           []CitationGuardIssue
	CitedParagraphs  int
	TotalParagraphs  int
	CitationCoverage float64
}

func ValidateCitationBoundAnswer(answer string, pack EvidencePack, options CitationGuardOptions) CitationGuardResult {
	paragraphs := answerParagraphs(answer)
	var issues []CitationGuardIssue
	citedParagraphs := 0

	for _, paragraph := range paragraphs {
		ids := evidenceIDsInText(paragraph)
		if len(ids) == 0 {
			if options.RequireParagraphCitations && looksFactualParagraph(paragraph) {
				issues = append(issues, CitationGuardIssue{Type: "missing_paragraph_citation", Paragraph: paragraph, Detail: "paragraph has no evidence id"})
			}
			continue
		}
		citedParagraphs++

		var evidenceText strings.Builder
		for _, id := range ids {
			item, ok := pack.SourceByID(id)
			if !ok {
				issues = append(issues, CitationGuardIssue{Type: "unknown_evidence_id", Paragraph: paragraph, EvidenceID: id, Detail: "evidence id not found"})
				continue
			}
			evidenceText.WriteString(item.Content)
			evidenceText.WriteString(" ")
		}

		claimText := evidenceIDPattern.ReplaceAllString(paragraph, "")
		if options.ValidateNumbers && evidenceText.Len() > 0 {
			for _, number := range extractTrustNumbers(claimText) {
				if !numberSupported(number, normalizeTrustNumberText(evidenceText.String())) {
					issues = append(issues, CitationGuardIssue{Type: "unsupported_number", Paragraph: paragraph, Detail: "number " + number + " not found in cited evidence"})
				}
			}
		}
		if options.ValidateEntityPhrases && evidenceText.Len() > 0 {
			normalizedEvidence := normalizeEntityText(evidenceText.String())
			for _, phrase := range extractEntityPhrases(claimText) {
				if !entityPhraseSupported(phrase, normalizedEvidence) {
					issues = append(issues, CitationGuardIssue{Type: "unsupported_entity", Paragraph: paragraph, Detail: "entity phrase " + phrase + " not found in cited evidence"})
				}
			}
		}
	}

	coverage := 1.0
	if len(paragraphs) > 0 {
		coverage = float64(citedParagraphs) / float64(len(paragraphs))
	}
	if options.MinCitationCoverage > 0 && coverage < options.MinCitationCoverage {
		issues = append(issues, CitationGuardIssue{Type: "weak_citation_coverage", Detail: fmt.Sprintf("coverage %.2f below %.2f", coverage, options.MinCitationCoverage)})
	}
	if trustCitationPattern.MatchString(answer) {
		issues = append(issues, CitationGuardIssue{Type: "raw_source_citation", Detail: "model emitted raw source citation instead of evidence id"})
	}

	return CitationGuardResult{
		Passed:           len(issues) == 0,
		Issues:           issues,
		CitedParagraphs:  citedParagraphs,
		TotalParagraphs:  len(paragraphs),
		CitationCoverage: coverage,
	}
}

func extractEntityPhrases(text string) []string {
	seen := map[string]struct{}{}
	var phrases []string
	add := func(raw string) {
		phrase := strings.Trim(raw, " \t\r\n.,;:!?()[]{}\"'")
		if phrase == "" || isIgnoredEntityPhrase(phrase) {
			return
		}
		key := strings.ToLower(phrase)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		phrases = append(phrases, phrase)
	}
	for _, match := range capitalizedPhrasePattern.FindAllString(text, -1) {
		add(match)
	}
	for _, match := range camelCaseEntityPattern.FindAllString(text, -1) {
		add(match)
	}
	return phrases
}

func isIgnoredEntityPhrase(phrase string) bool {
	lower := strings.ToLower(strings.TrimSpace(phrase))
	if lower == "" {
		return true
	}
	ignoredExact := map[string]struct{}{
		"source": {}, "page": {}, "content": {}, "the provided": {}, "provided documents": {},
		"the company": {}, "chief executive officer": {}, "fiscal year": {},
	}
	if _, ok := ignoredExact[lower]; ok {
		return true
	}
	ignoredPrefixes := []string{
		"the ", "this ", "these ", "those ", "according to ", "based on ",
		"source ", "page ", "content ",
	}
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(lower, prefix) && len(strings.Fields(lower)) <= 3 {
			return true
		}
	}
	return false
}

func entityPhraseSupported(phrase string, normalizedEvidence string) bool {
	normalizedPhrase := normalizeEntityText(phrase)
	if normalizedPhrase == "" {
		return true
	}
	if strings.Contains(normalizedEvidence, normalizedPhrase) {
		return true
	}
	words := significantEntityWords(normalizedPhrase)
	if len(words) == 0 {
		return true
	}
	matched := 0
	for _, word := range words {
		if strings.Contains(normalizedEvidence, word) {
			matched++
		}
	}
	return matched == len(words) && len(words) >= 2
}

func normalizeEntityText(text string) string {
	text = strings.ToLower(text)
	var builder strings.Builder
	lastSpace := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '&' {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func significantEntityWords(normalizedPhrase string) []string {
	stop := map[string]struct{}{
		"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "of": {}, "for": {}, "to": {}, "in": {}, "on": {}, "with": {},
		"company": {}, "companies": {}, "business": {}, "segment": {}, "segments": {},
	}
	var words []string
	for _, word := range strings.Fields(normalizedPhrase) {
		if len(word) < 2 {
			continue
		}
		if _, ok := stop[word]; ok {
			continue
		}
		words = append(words, word)
	}
	return words
}

func answerParagraphs(answer string) []string {
	lines := strings.Split(strings.ReplaceAll(answer, "\r\n", "\n"), "\n")
	paragraphs := make([]string, 0, len(lines))
	var current strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(current.String()))
				current.Reset()
			}
			continue
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(trimmed)
	}
	if current.Len() > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(current.String()))
	}
	return paragraphs
}

func looksFactualParagraph(paragraph string) bool {
	text := strings.TrimSpace(paragraph)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "provided documents do not contain sufficient information") {
		return false
	}
	return strings.ContainsAny(text, "0123456789") ||
		strings.Contains(lower, " is ") ||
		strings.Contains(lower, " are ") ||
		strings.Contains(lower, " was ") ||
		strings.Contains(lower, " were ") ||
		strings.Contains(lower, " increased") ||
		strings.Contains(lower, " decreased") ||
		strings.Contains(lower, " compared")
}
