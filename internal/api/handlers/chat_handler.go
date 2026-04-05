package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
	"enterprise-pdf-ai/internal/service"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	chatService service.ChatService
}

type createSessionRequest struct {
	Title string `json:"title" binding:"omitempty,max=255"`
}

type messageRequest struct {
	Question string `json:"question" binding:"required,min=1,max=4000"`
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

	reply, err := h.chatService.SendMessage(c.Request.Context(), userID, c.Param("id"), strings.TrimSpace(req.Question))
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
