package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"enterprise-pdf-ai/internal/api/middleware"
	"enterprise-pdf-ai/internal/configs"
	"enterprise-pdf-ai/internal/worker"
	"enterprise-pdf-ai/internal/worker/tasks"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type DocumentHandler struct {
	producer  worker.TaskProducer
	uploadCfg configs.UploadConfig
}

func NewDocumentHandler(producer worker.TaskProducer, uploadCfg configs.UploadConfig) *DocumentHandler {
	return &DocumentHandler{
		producer:  producer,
		uploadCfg: uploadCfg,
	}
}

func (h *DocumentHandler) UploadDocument(c *gin.Context) {
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

	c.JSON(http.StatusAccepted, gin.H{
		"task_id":     taskID,
		"document_id": documentID,
		"status":      "processing",
	})
}
