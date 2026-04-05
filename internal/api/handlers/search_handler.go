package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"enterprise-pdf-ai/internal/service"
	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	chatService service.ChatService
}

func NewSearchHandler(chatService service.ChatService) *SearchHandler {
	return &SearchHandler{chatService: chatService}
}

func (h *SearchHandler) Search(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}

	topK := 5
	if raw := strings.TrimSpace(c.Query("top_k")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 20 {
			topK = parsed
		}
	}

	results, err := h.chatService.Search(c.Request.Context(), userID, query, topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query": query,
		"items": results,
	})
}
