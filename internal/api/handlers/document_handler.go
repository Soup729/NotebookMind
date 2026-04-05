package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/models"
	"enterprise-pdf-ai/internal/repository"
	"enterprise-pdf-ai/internal/service"
	"enterprise-pdf-ai/internal/worker"
	"enterprise-pdf-ai/internal/worker/tasks"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type DocumentHandler struct {
	producer        worker.TaskProducer
	documentService service.DocumentService
	uploadCfg       configs.UploadConfig
}

func NewDocumentHandler(producer worker.TaskProducer, documentService service.DocumentService, uploadCfg configs.UploadConfig) *DocumentHandler {
	return &DocumentHandler{
		producer:        producer,
		documentService: documentService,
		uploadCfg:       uploadCfg,
	}
}

func (h *DocumentHandler) UploadDocument(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != strings.ToLower(h.uploadCfg.AllowedExtName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only .pdf files are supported"})
		return
	}

	maxFileSize := h.uploadCfg.MaxFileSizeMB * 1024 * 1024
	if fileHeader.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file size exceeds %d MB", h.uploadCfg.MaxFileSizeMB)})
		return
	}

	if err := os.MkdirAll(h.uploadCfg.LocalDir, 0o755); err != nil {
		zap.L().Error("create upload directory failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	documentID := uuid.NewString()
	storedFileName := fmt.Sprintf("%s_%d%s", documentID, time.Now().UnixNano(), ext)
	storedPath := filepath.Join(h.uploadCfg.LocalDir, storedFileName)

	if err := c.SaveUploadedFile(fileHeader, storedPath); err != nil {
		zap.L().Error("save uploaded file failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	document := &models.Document{
		ID:         documentID,
		UserID:     userID,
		FileName:   fileHeader.Filename,
		StoredPath: storedPath,
		Status:     models.DocumentStatusProcessing,
		FileSize:   fileHeader.Size,
	}
	if err := h.documentService.Create(c.Request.Context(), document); err != nil {
		zap.L().Error("persist document metadata failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist document"})
		return
	}

	taskPayload := tasks.ProcessDocumentPayload{
		TaskID:     uuid.NewString(),
		UserID:     userID,
		FilePath:   storedPath,
		FileName:   fileHeader.Filename,
		DocumentID: documentID,
	}
	taskID, err := h.producer.EnqueueProcessDocument(c.Request.Context(), taskPayload)
	if err != nil {
		zap.L().Error("enqueue process document task failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue task"})
		return
	}

	c.JSON(http.StatusAccepted, documentResponse(document, taskID))
}

func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	documents, err := h.documentService.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list documents"})
		return
	}

	items := make([]gin.H, 0, len(documents))
	for _, document := range documents {
		items = append(items, documentResponse(&document, ""))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *DocumentHandler) GetDocument(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	document, err := h.documentService.Get(c.Request.Context(), userID, c.Param("id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load document"})
		return
	}
	c.JSON(http.StatusOK, documentResponse(document, ""))
}

func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	if err := h.documentService.Delete(c.Request.Context(), userID, c.Param("id")); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete document"})
		return
	}
	c.Status(http.StatusNoContent)
}

func documentResponse(document *models.Document, taskID string) gin.H {
	response := gin.H{
		"id":            document.ID,
		"file_name":     document.FileName,
		"status":        document.Status,
		"error_message": document.ErrorMessage,
		"file_size":     document.FileSize,
		"chunk_count":   document.ChunkCount,
		"created_at":    document.CreatedAt,
		"updated_at":    document.UpdatedAt,
		"processed_at":  document.ProcessedAt,
	}
	if taskID != "" {
		response["task_id"] = taskID
	}
	return response
}
