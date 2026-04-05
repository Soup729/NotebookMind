package handlers

import (
	"net/http"

	"NotebookAI/internal/models"
	"NotebookAI/internal/service"

	"github.com/gin-gonic/gin"
)

// NoteHandler 笔记处理器
type NoteHandler struct {
	noteService service.NoteService
}

// NewNoteHandler 创建笔记处理器
func NewNoteHandler(noteService service.NoteService) *NoteHandler {
	return &NoteHandler{noteService: noteService}
}

// CreateNoteRequest 创建笔记请求
type CreateNoteRequest struct {
	NotebookID string            `json:"notebook_id"`
	SessionID  string            `json:"session_id"`
	Title      string            `json:"title" binding:"required"`
	Content    string            `json:"content" binding:"required"`
	Type       string            `json:"type"`
	IsPinned   bool              `json:"is_pinned"`
	Tags       []string          `json:"tags"`
	Metadata   map[string]string `json:"metadata"`
}

// UpdateNoteRequest 更新笔记请求
type UpdateNoteRequest struct {
	Title      *string   `json:"title"`
	Content    *string   `json:"content"`
	IsPinned   *bool     `json:"is_pinned"`
	Tags       *[]string `json:"tags"`
	NotebookID *string   `json:"notebook_id"`
}

// ListNotesRequest 列出笔记请求
type ListNotesRequest struct {
	NotebookID string `form:"notebook_id"`
	SessionID  string `form:"session_id"`
	Type       string `form:"type"`
	Tag        string `form:"tag"`
	PinnedOnly bool   `form:"pinned_only"`
	Page       int    `form:"page,default=1"`
	PageSize   int    `form:"page_size,default=20"`
}

// Create 创建笔记
func (h *NoteHandler) Create(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceReq := &service.CreateNoteRequest{
		NotebookID: req.NotebookID,
		SessionID:  req.SessionID,
		Title:      req.Title,
		Content:    req.Content,
		Type:       req.Type,
		IsPinned:   req.IsPinned,
		Tags:       req.Tags,
		Metadata:   req.Metadata,
	}

	note, err := h.noteService.Create(c.Request.Context(), userID, serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create note"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"note": note.ToResponse()})
}

// Update 更新笔记
func (h *NoteHandler) Update(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note id is required"})
		return
	}

	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceReq := &service.UpdateNoteRequest{
		Title:      req.Title,
		Content:    req.Content,
		IsPinned:   req.IsPinned,
		Tags:       req.Tags,
		NotebookID: req.NotebookID,
	}

	note, err := h.noteService.Update(c.Request.Context(), userID, noteID, serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update note"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"note": note.ToResponse()})
}

// Delete 删除笔记
func (h *NoteHandler) Delete(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note id is required"})
		return
	}

	err := h.noteService.Delete(c.Request.Context(), userID, noteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete note"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "note deleted"})
}

// Get 获取单个笔记
func (h *NoteHandler) Get(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note id is required"})
		return
	}

	note, err := h.noteService.Get(c.Request.Context(), userID, noteID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"note": note.ToResponse()})
}

// List 列出笔记
func (h *NoteHandler) List(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req ListNotesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceReq := &service.ListNotesRequest{
		NotebookID: req.NotebookID,
		SessionID:  req.SessionID,
		Type:       req.Type,
		Tag:        req.Tag,
		PinnedOnly: req.PinnedOnly,
		Page:       req.Page,
		PageSize:   req.PageSize,
	}

	resp, err := h.noteService.List(c.Request.Context(), userID, serviceReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notes"})
		return
	}

	// 转换响应
	notes := make([]*models.NoteResponse, len(resp.Items))
	for i, note := range resp.Items {
		notes[i] = note.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"items":       notes,
		"total_count": resp.TotalCount,
		"page":        resp.Page,
		"page_size":   resp.PageSize,
	})
}

// Pin 钉住/取消钉住笔记
func (h *NoteHandler) Pin(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	if noteID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note id is required"})
		return
	}

	err := h.noteService.Pin(c.Request.Context(), userID, noteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pin note"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "note pin toggled"})
}

// AddTag 添加标签
func (h *NoteHandler) AddTag(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	var req struct {
		Tag string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.noteService.AddTag(c.Request.Context(), userID, noteID, req.Tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tag added"})
}

// RemoveTag 移除标签
func (h *NoteHandler) RemoveTag(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	noteID := c.Param("id")
	var req struct {
		Tag string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.noteService.RemoveTag(c.Request.Context(), userID, noteID, req.Tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove tag"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "tag removed"})
}

// SearchByTag 按标签搜索笔记
func (h *NoteHandler) SearchByTag(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	notes, err := h.noteService.SearchByTag(c.Request.Context(), userID, tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search notes"})
		return
	}

	responses := make([]*models.NoteResponse, len(notes))
	for i, note := range notes {
		responses[i] = note.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{"items": responses})
}
