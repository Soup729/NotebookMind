package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tmc/langchaingo/embeddings"
	"go.uber.org/zap"
)

// NotebookHandler handles notebook-related HTTP requests
type NotebookHandler struct {
	notebookService service.NotebookService
	chatService     service.NotebookChatService
	embedder        embeddings.Embedder
}

// NewNotebookHandler creates a new NotebookHandler
func NewNotebookHandler(
	notebookService service.NotebookService,
	chatService service.NotebookChatService,
	embedder embeddings.Embedder,
) *NotebookHandler {
	return &NotebookHandler{
		notebookService: notebookService,
		chatService:     chatService,
		embedder:        embedder,
	}
}

// Request/Response DTOs

type createNotebookRequest struct {
	Title       string `json:"title" binding:"omitempty,max=255"`
	Description string `json:"description" binding:"omitempty,max=1000"`
}

type updateNotebookRequest struct {
	Title       string `json:"title" binding:"omitempty,max=255"`
	Description string `json:"description" binding:"omitempty,max=1000"`
	Status      string `json:"status" binding:"omitempty,oneof=active archived"`
}

type addDocumentRequest struct {
	DocumentID string `json:"document_id" binding:"required,uuid"`
}

type notebookCreateSessionRequest struct {
	Title string `json:"title" binding:"omitempty,max=255"`
}

type chatMessageRequest struct {
	Question string `json:"question" binding:"required,min=1,max=4000"`
}

type searchRequest struct {
	Query string `json:"query" binding:"required,min=1,max=1000"`
	TopK  int    `json:"top_k" binding:"omitempty,min=1,max=20"`
}

// Notebook Handlers

func (h *NotebookHandler) CreateNotebook(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req createNotebookRequest
	if err := c.ShouldBindJSON(&req); err != nil && !isNotebookEmptyBodyError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	notebook, err := h.notebookService.CreateNotebook(c.Request.Context(), userID, req.Title, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create notebook"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"notebook": notebookResponse(notebook)})
}

func (h *NotebookHandler) GetNotebook(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebook, err := h.notebookService.GetNotebook(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notebook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notebook": notebookResponse(notebook)})
}

func (h *NotebookHandler) ListNotebooks(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebooks, err := h.notebookService.ListNotebooks(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notebooks"})
		return
	}

	items := make([]gin.H, 0, len(notebooks))
	for _, nb := range notebooks {
		items = append(items, notebookResponse(&nb))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *NotebookHandler) UpdateNotebook(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebook, err := h.notebookService.GetNotebook(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notebook"})
		return
	}

	var req updateNotebookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		notebook.Title = strings.TrimSpace(req.Title)
	}
	if req.Description != "" {
		notebook.Description = strings.TrimSpace(req.Description)
	}
	if req.Status != "" {
		notebook.Status = req.Status
	}

	if err := h.notebookService.UpdateNotebook(c.Request.Context(), notebook); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notebook"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notebook": notebookResponse(notebook)})
}

func (h *NotebookHandler) DeleteNotebook(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	if err := h.notebookService.DeleteNotebook(c.Request.Context(), userID, c.Param("id")); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete notebook"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Document Management

func (h *NotebookHandler) AddDocument(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")

	// Verify notebook access
	if _, err := h.notebookService.GetNotebook(c.Request.Context(), userID, notebookID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notebook"})
		return
	}

	var req addDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.notebookService.AddDocumentToNotebook(c.Request.Context(), notebookID, req.DocumentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add document"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "document added to notebook"})
}

func (h *NotebookHandler) RemoveDocument(c *gin.Context) {
	_, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")
	documentID := c.Param("documentId")

	if err := h.notebookService.RemoveDocumentFromNotebook(c.Request.Context(), notebookID, documentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook or document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove document"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *NotebookHandler) ListDocuments(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")

	// Verify notebook access
	if _, err := h.notebookService.GetNotebook(c.Request.Context(), userID, notebookID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notebook"})
		return
	}

	documents, err := h.notebookService.ListNotebookDocuments(c.Request.Context(), notebookID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list documents"})
		return
	}

	items := make([]gin.H, 0, len(documents))
	for _, doc := range documents {
		items = append(items, documentResponse(&doc, ""))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Document Guide

func (h *NotebookHandler) GetDocumentGuide(c *gin.Context) {
	_, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	documentID := c.Param("documentId")

	guide, err := h.notebookService.GetDocumentGuide(c.Request.Context(), documentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "guide not found or not yet generated"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get guide"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"guide": guideResponse(guide)})
}

// Chat Session Management

func (h *NotebookHandler) CreateSession(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")

	// Verify notebook access
	if _, err := h.notebookService.GetNotebook(c.Request.Context(), userID, notebookID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notebook"})
		return
	}

	var req notebookCreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !isNotebookEmptyBodyError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := h.chatService.CreateSession(c.Request.Context(), userID, notebookID, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"session": session})
}

func (h *NotebookHandler) ListSessions(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")

	sessions, err := h.chatService.ListSessions(c.Request.Context(), userID, notebookID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	items := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, gin.H{
			"id":              s.ID,
			"title":           s.Title,
			"last_message_at": s.LastMessageAt,
			"created_at":      s.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// StreamChat performs streaming chat with SSE
func (h *NotebookHandler) StreamChat(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	sessionID := c.Param("sessionId")

	var req chatMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Stream response
	err := h.chatService.StreamChat(c.Request.Context(), userID, sessionID, req.Question, func(reply *service.NotebookChatReply) bool {
		data, _ := json.Marshal(reply)
		_, err := c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
		if err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	})

	if err != nil {
		zap.L().Error("stream chat failed", zap.Error(err))
		// Send error event
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", `{"error":"streaming failed"}`))
		c.Writer.Flush()
		return
	}

	// Send completion event
	c.Writer.WriteString("data: [DONE]\n\n")
	c.Writer.Flush()
}

// Search within notebook
func (h *NotebookHandler) SearchNotebook(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")

	var req searchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	// Embed query
	queryVector, err := h.embedder.EmbedQuery(c.Request.Context(), req.Query)
	if err != nil {
		zap.L().Error("embed query failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to embed query"})
		return
	}

	results, err := h.chatService.SearchNotebook(c.Request.Context(), userID, notebookID, req.Query, topK)
	if err != nil {
		zap.L().Error("search notebook failed", zap.Error(err), zap.String("notebookID", notebookID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":  req.Query,
		"top_k":  topK,
		"items":  results,
		"vector": queryVector, // Optional: return for debugging
	})
}

// Helper functions

func notebookResponse(nb *models.Notebook) gin.H {
	return gin.H{
		"id":           nb.ID,
		"title":        nb.Title,
		"description":  nb.Description,
		"status":       nb.Status,
		"document_cnt": nb.DocumentCnt,
		"created_at":   nb.CreatedAt,
		"updated_at":   nb.UpdatedAt,
	}
}

func guideResponse(guide *models.DocumentGuide) gin.H {
	return gin.H{
		"id":           guide.ID,
		"document_id":  guide.DocumentID,
		"summary":      guide.Summary,
		"faq_json":     guide.FaqJSON,
		"key_points":   guide.KeyPoints,
		"status":       guide.Status,
		"error_msg":    guide.ErrorMsg,
		"generated_at": guide.GeneratedAt,
		"created_at":   guide.CreatedAt,
	}
}

func isNotebookEmptyBodyError(err error) bool {
	if err == io.EOF {
		return true
	}
	return strings.Contains(err.Error(), "EOF")
}
