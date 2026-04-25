package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
)

type fakeChatRepoForRewrite struct {
	messages []models.ChatMessage
}

func (f *fakeChatRepoForRewrite) CreateSession(context.Context, *models.ChatSession) error {
	return nil
}
func (f *fakeChatRepoForRewrite) GetSession(context.Context, string, string) (*models.ChatSession, error) {
	return nil, nil
}
func (f *fakeChatRepoForRewrite) ListSessions(context.Context, string) ([]models.ChatSession, error) {
	return nil, nil
}
func (f *fakeChatRepoForRewrite) ListSessionsByNotebook(context.Context, string, string) ([]models.ChatSession, error) {
	return nil, nil
}
func (f *fakeChatRepoForRewrite) SaveMessage(context.Context, *models.ChatMessage) error { return nil }
func (f *fakeChatRepoForRewrite) ListSessionMessages(context.Context, string, string, int) ([]models.ChatMessage, error) {
	return f.messages, nil
}
func (f *fakeChatRepoForRewrite) UpdateSessionActivity(context.Context, string, string, string, time.Time) error {
	return nil
}
func (f *fakeChatRepoForRewrite) UpdateSessionMemory(context.Context, string, string, string, string, int, time.Time) error {
	return nil
}
func (f *fakeChatRepoForRewrite) ClearSessionMemory(context.Context, string, string) error {
	return nil
}
func (f *fakeChatRepoForRewrite) CountSessions(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeChatRepoForRewrite) CountMessages(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeChatRepoForRewrite) SumTokens(context.Context, string) (int64, error)     { return 0, nil }
func (f *fakeChatRepoForRewrite) DailyTokenUsage(context.Context, string, int) ([]repository.DailyUsageRow, error) {
	return nil, nil
}
func (f *fakeChatRepoForRewrite) DeleteSession(context.Context, string) error           { return nil }
func (f *fakeChatRepoForRewrite) DeleteMessagesBySession(context.Context, string) error { return nil }

func TestRewriteAddsConversationContextForFollowupEvenWhenIntentRoutingDisabled(t *testing.T) {
	repo := &fakeChatRepoForRewrite{messages: []models.ChatMessage{
		{Role: "user", Content: "What was the company's total R&D investment in 2024?"},
		{Role: "assistant", Content: "The company invested $266.4 million in R&D in 2024."},
	}}
	svc := NewIntentRewriteService(repo, nil, NewTokenizer(), &configs.IntentRewriteConfig{
		Enabled:         false,
		MaxContextTerms: 8,
	})

	result, err := svc.Rewrite(context.Background(), "user-1", "session-1", "What percentage of total revenue does that represent?")
	if err != nil {
		t.Fatalf("Rewrite returned error: %v", err)
	}

	if result.RewrittenQuery == result.OriginalQuery {
		t.Fatalf("expected follow-up query to include conversation context, got original query %q", result.RewrittenQuery)
	}
	if !strings.Contains(strings.ToLower(result.RewrittenQuery), "r&d") ||
		!strings.Contains(strings.ToLower(result.RewrittenQuery), "266.4") ||
		!strings.Contains(strings.ToLower(result.RewrittenQuery), "total revenue") {
		t.Fatalf("rewritten query did not preserve antecedent context: %q", result.RewrittenQuery)
	}
}

func TestRewriteFollowupPrefersStableFactAnchors(t *testing.T) {
	repo := &fakeChatRepoForRewrite{messages: []models.ChatMessage{
		{Role: "user", Content: "Which regions contributed most to revenue growth in 2024?"},
		{Role: "assistant", Content: "Asia-Pacific contributed 42% of incremental revenue growth, followed by Europe at 27%. Cloud Infrastructure and AI Platform were cited as the main product drivers."},
	}}
	svc := NewIntentRewriteService(repo, nil, NewTokenizer(), &configs.IntentRewriteConfig{
		Enabled:         false,
		MaxContextTerms: 12,
	})

	result, err := svc.Rewrite(context.Background(), "user-1", "session-1", "Among those top regions, which product categories drove that regional growth?")
	if err != nil {
		t.Fatalf("Rewrite returned error: %v", err)
	}

	rewritten := strings.ToLower(result.RewrittenQuery)
	for _, want := range []string{"asia-pacific", "42%", "europe", "cloud infrastructure", "ai platform", "product drivers"} {
		if !strings.Contains(rewritten, want) {
			t.Fatalf("rewritten query missing stable fact anchor %q:\n%s", want, result.RewrittenQuery)
		}
	}
}

func TestRewriteFollowupKeepsLateFactAnchorsFromLongPreviousAnswer(t *testing.T) {
	longPrefix := strings.Repeat("General narrative about revenue growth. ", 40)
	repo := &fakeChatRepoForRewrite{messages: []models.ChatMessage{
		{Role: "user", Content: "Which regions contributed most to revenue growth in 2024?"},
		{Role: "assistant", Content: longPrefix + "Asia-Pacific contributed 42% of incremental revenue growth, followed by Europe at 27%. Cloud Infrastructure and AI Platform were the main product drivers."},
	}}
	svc := NewIntentRewriteService(repo, nil, NewTokenizer(), &configs.IntentRewriteConfig{
		Enabled:         false,
		MaxContextTerms: 12,
	})

	result, err := svc.Rewrite(context.Background(), "user-1", "session-1", "Among those top regions, which product categories drove that regional growth?")
	if err != nil {
		t.Fatalf("Rewrite returned error: %v", err)
	}

	rewritten := strings.ToLower(result.RewrittenQuery)
	for _, want := range []string{"asia-pacific", "42%", "cloud infrastructure", "ai platform"} {
		if !strings.Contains(rewritten, want) {
			t.Fatalf("rewritten query lost late fact anchor %q:\n%s", want, result.RewrittenQuery)
		}
	}
}
