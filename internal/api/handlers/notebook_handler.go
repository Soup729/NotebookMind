package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	artifactService service.NotebookArtifactService
	exportService   service.NotebookExportService
	graphService    service.KnowledgeGraphService
	embedder        embeddings.Embedder
}

// NewNotebookHandler creates a new NotebookHandler
func NewNotebookHandler(
	notebookService service.NotebookService,
	chatService service.NotebookChatService,
	artifactService service.NotebookArtifactService,
	exportService service.NotebookExportService,
	graphService service.KnowledgeGraphService,
	embedder embeddings.Embedder,
) *NotebookHandler {
	return &NotebookHandler{
		notebookService: notebookService,
		chatService:     chatService,
		artifactService: artifactService,
		exportService:   exportService,
		graphService:    graphService,
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
	Question    string   `json:"question" binding:"required,min=1,max=4000"`
	DocumentIDs []string `json:"document_ids"`
}

type searchRequest struct {
	Query string `json:"query" binding:"required,min=1,max=1000"`
	TopK  int    `json:"top_k" binding:"omitempty,min=1,max=20"`
}

type generateArtifactRequest struct {
	Type string `json:"type" binding:"required,oneof=briefing comparison timeline topic_clusters study_pack"`
}

type createExportOutlineRequest struct {
	Format           string   `json:"format" binding:"required"`
	DocumentIDs      []string `json:"document_ids"`
	Language         string   `json:"language"`
	Style            string   `json:"style"`
	Length           string   `json:"length"`
	Requirements     string   `json:"requirements"`
	IncludeCitations bool     `json:"include_citations"`
}

type confirmExportRequest struct {
	Outline []service.ExportOutlineSection `json:"outline" binding:"required"`
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

	notebookID := c.Param("id")
	if h.graphService != nil {
		if err := h.graphService.DeleteNotebookGraph(c.Request.Context(), notebookID); err != nil {
			zap.L().Warn("cleanup notebook graph failed", zap.String("notebook_id", notebookID), zap.Error(err))
		}
	}
	if err := h.notebookService.DeleteNotebook(c.Request.Context(), userID, notebookID); err != nil {
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
	if h.graphService != nil {
		if err := h.graphService.DeleteDocumentGraph(c.Request.Context(), documentID); err != nil {
			zap.L().Warn("cleanup notebook graph document data failed",
				zap.String("notebook_id", notebookID),
				zap.String("document_id", documentID),
				zap.Error(err),
			)
		}
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

	c.JSON(http.StatusCreated, gin.H{"session": notebookSessionResponse(session)})
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
		items = append(items, notebookSessionResponse(&s))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// DeleteSession deletes a chat session from a notebook
func (h *NotebookHandler) DeleteSession(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	notebookID := c.Param("id")
	sessionID := c.Param("sessionId")

	// Verify notebook access
	if _, err := h.notebookService.GetNotebook(c.Request.Context(), userID, notebookID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify notebook"})
		return
	}

	if err := h.chatService.DeleteSession(c.Request.Context(), userID, sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *NotebookHandler) GetSessionMemory(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	session, ok := h.loadNotebookSession(c, userID)
	if !ok {
		return
	}
	memory, err := h.chatService.GetSessionMemory(c.Request.Context(), userID, session.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get session memory"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"memory": memory})
}

func (h *NotebookHandler) RefreshSessionMemory(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	session, ok := h.loadNotebookSession(c, userID)
	if !ok {
		return
	}
	memory, err := h.chatService.RefreshSessionMemory(c.Request.Context(), userID, session.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to refresh session memory"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"memory": memory})
}

func (h *NotebookHandler) ClearSessionMemory(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	session, ok := h.loadNotebookSession(c, userID)
	if !ok {
		return
	}
	if err := h.chatService.ClearSessionMemory(c.Request.Context(), userID, session.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear session memory"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *NotebookHandler) loadNotebookSession(c *gin.Context, userID string) (*models.ChatSession, bool) {
	notebookID := c.Param("id")
	sessionID := c.Param("sessionId")
	session, err := h.chatService.GetSession(c.Request.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load session"})
		return nil, false
	}
	if session.NotebookID != notebookID {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return nil, false
	}
	return session, true
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

	// 预检查：先验证会话是否存在，如果不存在立即返回明确错误而非 SSE 流中的隐式错误
	// 这样前端可以在发送前就知道会话已失效，避免长时间等待后才收到错误
	session, sessionErr := h.chatService.GetSession(c.Request.Context(), userID, sessionID)
	if sessionErr != nil {
		if errors.Is(sessionErr, repository.ErrNotFound) {
			c.JSON(http.StatusGone, gin.H{
				"error":     "session not found or expired",
				"code":      "SESSION_GONE",
				"hint":      "该对话已被删除或不存在，请创建新对话",
				"sessionId": sessionID,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load session"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Stream response — 传入预加载的 session 对象避免重复查询
	err := h.chatService.StreamChatWithSession(c.Request.Context(), userID, session, req.Question, req.DocumentIDs, func(reply *service.NotebookChatReply) bool {
		data, _ := json.Marshal(reply)
		_, err := c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
		if err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	})

	if err != nil {
		zap.L().Error("stream chat failed", zap.String("session_id", sessionID), zap.String("user_id", userID), zap.Error(err))
		// Send error event via SSE
		errData, _ := json.Marshal(gin.H{"error": "streaming failed", "detail": err.Error()})
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", errData))
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

func (h *NotebookHandler) GetKnowledgeGraph(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	if h.graphService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge graph service is not configured"})
		return
	}
	graph, err := h.graphService.GetNotebookGraph(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get knowledge graph"})
		return
	}
	c.JSON(http.StatusOK, graph)
}

func (h *NotebookHandler) ReindexKnowledgeGraph(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	if h.graphService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "knowledge graph service is not configured"})
		return
	}
	if err := h.graphService.ReindexNotebookGraph(c.Request.Context(), userID, c.Param("id")); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reindex knowledge graph", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

func (h *NotebookHandler) ListArtifacts(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	artifacts, err := h.artifactService.ListArtifacts(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list artifacts"})
		return
	}
	items := make([]gin.H, 0, len(artifacts))
	for _, artifact := range artifacts {
		items = append(items, artifactResponse(&artifact))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *NotebookHandler) GenerateArtifact(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	var req generateArtifactRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	artifact, err := h.artifactService.GenerateArtifact(c.Request.Context(), userID, c.Param("id"), req.Type)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"artifact": artifactResponse(artifact)})
}

func (h *NotebookHandler) GetArtifact(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	artifact, err := h.artifactService.GetArtifact(c.Request.Context(), userID, c.Param("id"), c.Param("artifactId"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get artifact"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"artifact": artifactResponse(artifact)})
}

func (h *NotebookHandler) DeleteArtifact(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	if err := h.artifactService.DeleteArtifact(c.Request.Context(), userID, c.Param("id"), c.Param("artifactId")); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete artifact"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *NotebookHandler) CreateExportOutline(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	var req createExportOutlineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	artifact, err := h.exportService.CreateOutline(c.Request.Context(), userID, c.Param("id"), service.ExportOutlineRequest{
		Format:           req.Format,
		DocumentIDs:      req.DocumentIDs,
		Language:         req.Language,
		Style:            req.Style,
		Length:           req.Length,
		Requirements:     req.Requirements,
		IncludeCitations: req.IncludeCitations,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notebook not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"artifact": artifactResponse(artifact)})
}

func (h *NotebookHandler) ConfirmExport(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	var req confirmExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	artifact, err := h.exportService.ConfirmOutline(c.Request.Context(), userID, c.Param("id"), c.Param("artifactId"), req.Outline)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"artifact": artifactResponse(artifact)})
}

func (h *NotebookHandler) DownloadExport(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}
	artifact, err := h.artifactService.GetArtifact(c.Request.Context(), userID, c.Param("id"), c.Param("artifactId"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get artifact"})
		return
	}
	if artifact.Status != models.ArtifactStatusCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "export is not completed"})
		return
	}
	if strings.TrimSpace(artifact.FilePath) == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "export file not found"})
		return
	}
	if _, err := os.Stat(artifact.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "export file not found"})
		return
	}
	fileName := artifact.FileName
	if strings.TrimSpace(fileName) == "" {
		fileName = "notebook-export.md"
	}
	c.FileAttachment(artifact.FilePath, fileName)
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

func notebookSessionResponse(session *models.ChatSession) gin.H {
	if session == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                   session.ID,
		"user_id":              session.UserID,
		"notebook_id":          session.NotebookID,
		"title":                session.Title,
		"last_message_at":      session.LastMessageAt,
		"memory_summary":       session.MemorySummary,
		"memory_json":          session.MemoryJSON,
		"memory_message_count": session.MemoryMessageCount,
		"memory_updated_at":    session.MemoryUpdatedAt,
		"created_at":           session.CreatedAt,
		"updated_at":           session.UpdatedAt,
	}
}

func artifactResponse(artifact *models.NotebookArtifact) gin.H {
	var content any
	if strings.TrimSpace(artifact.ContentJSON) != "" {
		_ = json.Unmarshal([]byte(artifact.ContentJSON), &content)
	}
	var sourceRefs any
	if strings.TrimSpace(artifact.SourceRefsJSON) != "" {
		_ = json.Unmarshal([]byte(artifact.SourceRefsJSON), &sourceRefs)
	}
	var request any
	if strings.TrimSpace(artifact.RequestJSON) != "" {
		_ = json.Unmarshal([]byte(artifact.RequestJSON), &request)
	}
	return gin.H{
		"id":           artifact.ID,
		"notebook_id":  artifact.NotebookID,
		"type":         artifact.Type,
		"title":        artifact.Title,
		"status":       artifact.Status,
		"content":      content,
		"source_refs":  sourceRefs,
		"request":      request,
		"file_name":    artifact.FileName,
		"mime_type":    artifact.MimeType,
		"task_id":      artifact.TaskID,
		"error_msg":    artifact.ErrorMsg,
		"version":      artifact.Version,
		"generated_at": artifact.GeneratedAt,
		"created_at":   artifact.CreatedAt,
		"updated_at":   artifact.UpdatedAt,
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
