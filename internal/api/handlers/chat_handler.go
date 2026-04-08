package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ChatHandler struct {
	chatService service.ChatService
}

type createSessionRequest struct {
	Title string `json:"title" binding:"omitempty,max=255"`
}

type messageRequest struct {
	Question    string   `json:"question" binding:"required,min=1,max=4000"`
	DocumentIDs []string `json:"document_ids"`
}

func NewChatHandler(chatService service.ChatService) *ChatHandler {
	return &ChatHandler{chatService: chatService}
}

func (h *ChatHandler) CreateSession(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !isEmptyBodyError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.chatService.CreateSession(c.Request.Context(), userID, strings.TrimSpace(req.Title))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"session": session})
}

func (h *ChatHandler) ListSessions(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	sessions, err := h.chatService.ListSessions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": sessions})
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	messages, err := h.chatService.ListMessages(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load messages"})
		return
	}

	items := make([]gin.H, 0, len(messages))
	for _, message := range messages {
		items = append(items, chatMessageResponse(message))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req messageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reply, err := h.chatService.SendMessage(c.Request.Context(), userID, c.Param("id"), strings.TrimSpace(req.Question), req.DocumentIDs)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session": reply.Session,
		"message": gin.H{
			"id":                reply.Message.ID,
			"role":              reply.Message.Role,
			"content":           reply.Message.Content,
			"prompt_tokens":     reply.Message.PromptTokens,
			"completion_tokens": reply.Message.CompletionTokens,
			"total_tokens":      reply.Message.TotalTokens,
			"created_at":        reply.Message.CreatedAt,
			"sources":           reply.Sources,
		},
	})
}

func chatMessageResponse(message models.ChatMessage) gin.H {
	sources := make([]service.ChatSource, 0)
	if strings.TrimSpace(message.SourcesJSON) != "" {
		_ = json.Unmarshal([]byte(message.SourcesJSON), &sources)
	}

	return gin.H{
		"id":                message.ID,
		"session_id":        message.SessionID,
		"role":              message.Role,
		"content":           message.Content,
		"prompt_tokens":     message.PromptTokens,
		"completion_tokens": message.CompletionTokens,
		"total_tokens":      message.TotalTokens,
		"created_at":        message.CreatedAt,
		"sources":           sources,
	}
}

func isEmptyBodyError(err error) bool {
	return strings.Contains(err.Error(), "EOF")
}

// StreamSendMessage handles SSE streaming chat
func (h *ChatHandler) StreamSendMessage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	sessionID := c.Param("id")

	var req messageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	sessionIDFinal := sessionID

	// Stream response using callbacks
	err := h.chatService.StreamSendMessage(c.Request.Context(), service.StreamChatOptions{
		UserID:      userID,
		SessionID:   sessionID,
		Question:    strings.TrimSpace(req.Question),
		DocumentIDs: req.DocumentIDs,
		OnSource: func(sources []service.ChatSource, promptTokens int) bool {
			event := service.SourceEvent{
				SessionID:    sessionIDFinal,
				MessageID:    "", // Will be set after generation
				Sources:      sources,
				PromptTokens: promptTokens,
			}
			data, _ := json.Marshal(event)
			_, err := c.Writer.WriteString(fmt.Sprintf("event: source\ndata: %s\n\n", data))
			if err != nil {
				return false
			}
			c.Writer.Flush()
			return true
		},
		OnToken: func(token string) bool {
			event := service.TokenEvent{
				SessionID: sessionIDFinal,
				MessageID: "", // Will be set after generation
				Token:     token,
			}
			data, _ := json.Marshal(event)
			_, err := c.Writer.WriteString(fmt.Sprintf("event: token\ndata: %s\n\n", data))
			if err != nil {
				return false
			}
			c.Writer.Flush()
			return true
		},
		OnDone: func(content string, promptTokens, completionTokens, totalTokens int) bool {
			event := service.DoneEvent{
				SessionID:        sessionIDFinal,
				MessageID:        "", // Will be set after generation
				Content:          content,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      totalTokens,
			}
			data, _ := json.Marshal(event)
			_, err := c.Writer.WriteString(fmt.Sprintf("event: done\ndata: %s\n\n", data))
			if err != nil {
				return false
			}
			c.Writer.Flush()
			return true
		},
		OnError: func(err error) bool {
			event := service.ErrorEvent{
				SessionID: sessionIDFinal,
				MessageID: "",
				Error:     err.Error(),
			}
			data, _ := json.Marshal(event)
			_, writeErr := c.Writer.WriteString(fmt.Sprintf("event: error\ndata: %s\n\n", data))
			if writeErr != nil {
				return false
			}
			c.Writer.Flush()
			return true
		},
	})

	if err != nil {
		zap.L().Error("stream chat failed", zap.Error(err))
		// Error already sent via callback
		return
	}

	// Send completion marker
	c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

// GetRecommendations returns recommended follow-up questions
func (h *ChatHandler) GetRecommendations(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	sessionID := c.Param("id")

	questions, err := h.chatService.GenerateRecommendedQuestions(c.Request.Context(), userID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate recommendations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"questions":  questions,
	})
}

// GetReflection returns an AI reflection on a specific message
func (h *ChatHandler) GetReflection(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	sessionID := c.Param("id")
	messageID := c.Param("messageId")

	reflection, err := h.chatService.GenerateReflection(c.Request.Context(), userID, sessionID, messageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate reflection: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionID,
		"message_id": messageID,
		"reflection": reflection,
	})
}
