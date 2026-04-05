package service

import "context"

// SSE Event Types
const (
	EventSource      = "source"   // Retrieved context sources
	EventToken       = "token"    // Streaming response token
	EventDone        = "done"     // Completion signal
	EventError       = "error"    // Error event
	EventSourcesOnly = "sources"   // Only sources, no streaming response (for quick mode)
)

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	Type    string      `json:"type,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Comment string      `json:"comment,omitempty"`
}

// SourceEvent is sent first with retrieved context
type SourceEvent struct {
	SessionID    string        `json:"session_id"`
	MessageID    string        `json:"message_id"`
	Sources      []ChatSource  `json:"sources"`
	PromptTokens int           `json:"prompt_tokens"`
}

// TokenEvent represents a streaming token
type TokenEvent struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Token     string `json:"token"`
}

// DoneEvent signals completion
type DoneEvent struct {
	SessionID         string `json:"session_id"`
	MessageID         string `json:"message_id"`
	Content           string `json:"content"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	TotalTokens       int    `json:"total_tokens"`
}

// ErrorEvent represents an error
type ErrorEvent struct {
	SessionID string `json:"session_id"`
	MessageID string `json:"message_id"`
	Error     string `json:"error"`
}

// StreamChatOptions contains options for streaming chat
type StreamChatOptions struct {
	UserID      string
	SessionID   string
	Question    string
	DocumentIDs []string
	OnSource    func(sources []ChatSource, promptTokens int) bool
	OnToken     func(token string) bool
	OnDone      func(content string, promptTokens, completionTokens, totalTokens int) bool
	OnError     func(err error) bool
}

// StreamChatService adds SSE streaming support
type StreamChatService interface {
	StreamSendMessage(ctx context.Context, opts StreamChatOptions) error
}