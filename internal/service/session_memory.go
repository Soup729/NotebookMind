package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
	"github.com/tmc/langchaingo/llms"
	"go.uber.org/zap"
)

const defaultMemoryRefreshDelta = 6

type SessionMemory struct {
	Summary       string     `json:"summary"`
	Goal          string     `json:"goal,omitempty"`
	Decisions     []string   `json:"decisions,omitempty"`
	OpenQuestions []string   `json:"open_questions,omitempty"`
	Preferences   []string   `json:"preferences,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type SessionMemoryService interface {
	GetMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error)
	RefreshMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error)
	ClearMemory(ctx context.Context, userID, sessionID string) error
	MemoryPrompt(ctx context.Context, userID, sessionID string) string
	MaybeRefreshAsync(userID, sessionID string)
}

type sessionMemoryService struct {
	repo     repository.ChatRepository
	llm      llms.Model
	generate func(ctx context.Context, prompt string) (string, error)
}

func NewSessionMemoryService(repo repository.ChatRepository, llm llms.Model) SessionMemoryService {
	svc := &sessionMemoryService{repo: repo, llm: llm}
	svc.generate = func(ctx context.Context, prompt string) (string, error) {
		if llm == nil {
			return "", errors.New("session memory llm is not configured")
		}
		return llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	}
	return svc
}

func ParseSessionMemory(raw, fallbackSummary string) SessionMemory {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var memory SessionMemory
	if err := json.Unmarshal([]byte(text), &memory); err != nil {
		memory.Summary = strings.TrimSpace(fallbackSummary)
	} else if strings.TrimSpace(memory.Summary) == "" {
		memory.Summary = strings.TrimSpace(fallbackSummary)
	}
	memory.Summary = strings.TrimSpace(memory.Summary)
	memory.Goal = strings.TrimSpace(memory.Goal)
	memory.Decisions = compactStrings(memory.Decisions, 8)
	memory.OpenQuestions = compactStrings(memory.OpenQuestions, 8)
	memory.Preferences = compactStrings(memory.Preferences, 6)
	return memory
}

func (m SessionMemory) FormatForPrompt() string {
	if strings.TrimSpace(m.Summary) == "" && strings.TrimSpace(m.Goal) == "" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("## Conversation Memory\n")
	builder.WriteString("This memory summarizes prior turns in this session and never overrides document evidence.\n")
	if m.Goal != "" {
		builder.WriteString("- Goal: ")
		builder.WriteString(m.Goal)
		builder.WriteString("\n")
	}
	if m.Summary != "" {
		builder.WriteString("- Summary: ")
		builder.WriteString(m.Summary)
		builder.WriteString("\n")
	}
	writeMemoryList(&builder, "Decisions", m.Decisions)
	writeMemoryList(&builder, "Open questions", m.OpenQuestions)
	writeMemoryList(&builder, "Preferences", m.Preferences)
	return strings.TrimSpace(builder.String())
}

func ShouldRefreshSessionMemory(session *models.ChatSession, messageCount, delta int) bool {
	if session == nil || messageCount <= 0 {
		return false
	}
	if delta <= 0 {
		delta = defaultMemoryRefreshDelta
	}
	if strings.TrimSpace(session.MemoryJSON) == "" && strings.TrimSpace(session.MemorySummary) == "" {
		return session.MemoryMessageCount == 0 || messageCount-session.MemoryMessageCount >= delta
	}
	return messageCount-session.MemoryMessageCount >= delta
}

func (s *sessionMemoryService) GetMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error) {
	session, err := s.repo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	memory := ParseSessionMemory(session.MemoryJSON, session.MemorySummary)
	memory.UpdatedAt = session.MemoryUpdatedAt
	return &memory, nil
}

func (s *sessionMemoryService) MemoryPrompt(ctx context.Context, userID, sessionID string) string {
	memory, err := s.GetMemory(ctx, userID, sessionID)
	if err != nil {
		return ""
	}
	return memory.FormatForPrompt()
}

func (s *sessionMemoryService) RefreshMemory(ctx context.Context, userID, sessionID string) (*SessionMemory, error) {
	session, err := s.repo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	messages, err := s.repo.ListSessionMessages(ctx, userID, sessionID, 0)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		empty := SessionMemory{}
		return &empty, nil
	}
	prompt := buildSessionMemoryPrompt(session, messages)
	raw, err := s.generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate session memory: %w", err)
	}
	fallback := fallbackSessionSummary(messages)
	memory := ParseSessionMemory(raw, fallback)
	now := time.Now()
	memory.UpdatedAt = &now
	rawJSON, _ := json.Marshal(memory)
	if err := s.repo.UpdateSessionMemory(ctx, userID, sessionID, memory.Summary, string(rawJSON), len(messages), now); err != nil {
		return nil, err
	}
	return &memory, nil
}

func (s *sessionMemoryService) ClearMemory(ctx context.Context, userID, sessionID string) error {
	return s.repo.ClearSessionMemory(ctx, userID, sessionID)
}

func (s *sessionMemoryService) MaybeRefreshAsync(userID, sessionID string) {
	go func() {
		ctx := context.Background()
		session, err := s.repo.GetSession(ctx, userID, sessionID)
		if err != nil {
			return
		}
		messages, err := s.repo.ListSessionMessages(ctx, userID, sessionID, 0)
		if err != nil {
			return
		}
		if !ShouldRefreshSessionMemory(session, len(messages), defaultMemoryRefreshDelta) {
			return
		}
		if _, err := s.RefreshMemory(ctx, userID, sessionID); err != nil {
			zap.L().Warn("session memory refresh failed", zap.String("session_id", sessionID), zap.Error(err))
		}
	}()
}

func buildSessionMemoryPrompt(session *models.ChatSession, messages []models.ChatMessage) string {
	var builder strings.Builder
	builder.WriteString("Summarize this notebook chat session into strict JSON only.\n")
	builder.WriteString("Do not infer sensitive personal traits. Only preserve explicit research context, decisions, open questions, and answer preferences.\n")
	builder.WriteString("Schema: {\"summary\":\"...\",\"goal\":\"...\",\"decisions\":[\"...\"],\"open_questions\":[\"...\"],\"preferences\":[\"...\"]}\n\n")
	if strings.TrimSpace(session.MemoryJSON) != "" || strings.TrimSpace(session.MemorySummary) != "" {
		builder.WriteString("Existing memory:\n")
		builder.WriteString(ParseSessionMemory(session.MemoryJSON, session.MemorySummary).FormatForPrompt())
		builder.WriteString("\n\n")
	}
	builder.WriteString("Messages:\n")
	start := 0
	if len(messages) > 24 {
		start = len(messages) - 24
	}
	for _, msg := range messages[start:] {
		builder.WriteString(strings.ToUpper(msg.Role))
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(msg.Content))
		builder.WriteString("\n")
	}
	return builder.String()
}

func fallbackSessionSummary(messages []models.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	start := 0
	if len(messages) > 4 {
		start = len(messages) - 4
	}
	var parts []string
	for _, msg := range messages[start:] {
		content := strings.Join(strings.Fields(msg.Content), " ")
		if len([]rune(content)) > 120 {
			content = string([]rune(content)[:120])
		}
		if content != "" {
			parts = append(parts, msg.Role+": "+content)
		}
	}
	return strings.Join(parts, " | ")
}

func compactStrings(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func writeMemoryList(builder *strings.Builder, label string, values []string) {
	if len(values) == 0 {
		return
	}
	builder.WriteString("- ")
	builder.WriteString(label)
	builder.WriteString(": ")
	builder.WriteString(strings.Join(values, "; "))
	builder.WriteString("\n")
}
