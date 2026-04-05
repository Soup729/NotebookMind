package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSSEEventTypes tests SSE event type constants
func TestSSEEventTypes(t *testing.T) {
	assert.Equal(t, "source", EventSource)
	assert.Equal(t, "token", EventToken)
	assert.Equal(t, "done", EventDone)
	assert.Equal(t, "error", EventError)
}

// TestSourceEvent tests SourceEvent structure
func TestSourceEvent(t *testing.T) {
	event := SourceEvent{
		SessionID:    "session-123",
		MessageID:    "msg-456",
		Sources:      []ChatSource{},
		PromptTokens: 100,
	}

	assert.Equal(t, "session-123", event.SessionID)
	assert.Equal(t, "msg-456", event.MessageID)
	assert.Equal(t, 100, event.PromptTokens)
}

// TestTokenEvent tests TokenEvent structure
func TestTokenEvent(t *testing.T) {
	event := TokenEvent{
		SessionID: "session-123",
		MessageID: "msg-456",
		Token:     "Hello",
	}

	assert.Equal(t, "session-123", event.SessionID)
	assert.Equal(t, "msg-456", event.MessageID)
	assert.Equal(t, "Hello", event.Token)
}

// TestDoneEvent tests DoneEvent structure
func TestDoneEvent(t *testing.T) {
	event := DoneEvent{
		SessionID:        "session-123",
		MessageID:        "msg-456",
		Content:          "Final response",
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	assert.Equal(t, "session-123", event.SessionID)
	assert.Equal(t, "msg-456", event.MessageID)
	assert.Equal(t, "Final response", event.Content)
	assert.Equal(t, 100, event.PromptTokens)
	assert.Equal(t, 50, event.CompletionTokens)
	assert.Equal(t, 150, event.TotalTokens)
}

// TestErrorEvent tests ErrorEvent structure
func TestErrorEvent(t *testing.T) {
	event := ErrorEvent{
		SessionID: "session-123",
		MessageID: "msg-456",
		Error:     "something went wrong",
	}

	assert.Equal(t, "session-123", event.SessionID)
	assert.Equal(t, "msg-456", event.MessageID)
	assert.Equal(t, "something went wrong", event.Error)
}

// TestStreamChatOptions tests StreamChatOptions structure
func TestStreamChatOptions(t *testing.T) {
	opts := StreamChatOptions{
		UserID:      "user-123",
		SessionID:   "session-456",
		Question:    "What is Go?",
		DocumentIDs: []string{"doc-1", "doc-2"},
	}

	assert.Equal(t, "user-123", opts.UserID)
	assert.Equal(t, "session-456", opts.SessionID)
	assert.Equal(t, "What is Go?", opts.Question)
	assert.Len(t, opts.DocumentIDs, 2)
	assert.Equal(t, "doc-1", opts.DocumentIDs[0])
	assert.Equal(t, "doc-2", opts.DocumentIDs[1])
	assert.Nil(t, opts.OnSource)
	assert.Nil(t, opts.OnToken)
	assert.Nil(t, opts.OnDone)
	assert.Nil(t, opts.OnError)
}

// TestBuildSessionTitle tests session title generation
func TestBuildSessionTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"What is Go?", "What is Go?"},
		{"This is a very long question that exceeds the maximum length of 48 characters and should be truncated", "This is a very long question that exceeds the maximum..."},
		{"", "New conversation"},
		{"   ", "New conversation"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := buildSessionTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDocumentsToSourcesWithMockDocs tests documentsToSources conversion
func TestDocumentsToSourcesWithMockDocs(t *testing.T) {
	// This test validates the helper function with mock data
	sources := documentsToSources(nil)
	assert.Empty(t, sources)
}