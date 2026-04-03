package handlers

import (
	"net/http"
	"strings"

	"enterprise-pdf-ai/internal/api/middleware"
	"enterprise-pdf-ai/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ChatHandler struct {
	chatService service.ChatService
}

type chatRequest struct {
	Question  string `json:"question" binding:"required,min=1,max=4000"`
	SessionID string `json:"session_id" binding:"required,min=1,max=64"`
}

type sourceDocumentResponse struct {
	Content string  `json:"content"`
	Score   float32 `json:"score"`
}

func NewChatHandler(chatService service.ChatService) *ChatHandler {
	return &ChatHandler{chatService: chatService}
}

func (h *ChatHandler) Chat(c *gin.Context) {
	userIDRaw, ok := c.Get(middleware.ContextUserIDKey)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	userID, ok := userIDRaw.(string)
	if !ok || strings.TrimSpace(userID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user context"})
		return
	}

	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	answer, docs, err := h.chatService.Ask(c.Request.Context(), userID, strings.TrimSpace(req.SessionID), strings.TrimSpace(req.Question))
	if err != nil {
		zap.L().Error("chat failed",
			zap.String("user_id", userID),
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to answer question"})
		return
	}

	sources := make([]sourceDocumentResponse, 0, len(docs))
	for _, doc := range docs {
		sources = append(sources, sourceDocumentResponse{
			Content: doc.PageContent,
			Score:   doc.Score,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":       strings.TrimSpace(req.SessionID),
		"answer":           answer,
		"source_documents": sources,
	})
}
