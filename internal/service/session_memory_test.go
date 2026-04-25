package service

import (
	"strings"
	"testing"
	"time"

	"NotebookAI/internal/models"
)

func TestParseSessionMemoryNormalizesFields(t *testing.T) {
	raw := `{
		"goal":"Analyze annual revenue",
		"decisions":["Focus on margin"],
		"open_questions":["Compare market share"],
		"preferences":["Answer in Chinese"]
	}`

	memory := ParseSessionMemory(raw, "fallback")

	if memory.Goal != "Analyze annual revenue" {
		t.Fatalf("expected goal to be parsed, got %q", memory.Goal)
	}
	if len(memory.Decisions) != 1 || memory.Decisions[0] != "Focus on margin" {
		t.Fatalf("expected decisions to be parsed, got %#v", memory.Decisions)
	}
	if memory.Summary == "" {
		t.Fatalf("expected summary fallback to be populated")
	}
}

func TestSessionMemoryPromptStaysBelowEvidencePriority(t *testing.T) {
	updatedAt := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	memory := SessionMemory{
		Summary:       "The user is comparing revenue and risk disclosures.",
		Goal:          "Prepare a concise research briefing.",
		Decisions:     []string{"Use Chinese answers."},
		OpenQuestions: []string{"Check if revenue explanations align."},
		Preferences:   []string{"Conclusion first."},
		UpdatedAt:     &updatedAt,
	}

	prompt := memory.FormatForPrompt()

	if !strings.Contains(prompt, "Conversation Memory") || !strings.Contains(prompt, "never overrides document evidence") {
		t.Fatalf("expected prompt to label memory as subordinate context, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Prepare a concise research briefing.") {
		t.Fatalf("expected goal in prompt, got:\n%s", prompt)
	}
}

func TestShouldRefreshSessionMemoryUsesMessageDelta(t *testing.T) {
	session := &models.ChatSession{MemoryMessageCount: 4}
	if !ShouldRefreshSessionMemory(session, 10, 6) {
		t.Fatalf("expected refresh when delta reaches threshold")
	}
	if ShouldRefreshSessionMemory(session, 9, 6) {
		t.Fatalf("did not expect refresh below threshold")
	}

	empty := &models.ChatSession{}
	if !ShouldRefreshSessionMemory(empty, 2, 6) {
		t.Fatalf("expected refresh when memory is empty and messages exist")
	}
}
