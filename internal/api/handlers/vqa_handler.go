package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"NotebookAI/internal/service"
	"github.com/gin-gonic/gin"
)

// VQAHandler handles Visual Question Answering requests
type VQAHandler struct {
	llmService service.LLMService
}

// NewVQAHandler creates a new VQA handler
func NewVQAHandler(llmService service.LLMService) *VQAHandler {
	return &VQAHandler{llmService: llmService}
}

// AskImage asks a question about an uploaded image (multipart/form-data)
func (h *VQAHandler) AskImage(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	// Get question from form
	question := strings.TrimSpace(c.PostForm("question"))
	if question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}

	// Get image file
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required"})
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !isValidImageType(contentType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type, supported: JPEG, PNG, GIF, WebP"})
		return
	}

	// Read image data
	imageData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read image"})
		return
	}

	// Check image size (max 10MB)
	if len(imageData) > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image size exceeds 10MB limit"})
		return
	}

	_ = userID // Future: track per-user usage

	// Process VQA request
	ctx := context.Background()
	result, err := h.llmService.AnswerWithImage(ctx, question, imageData, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("VQA failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"answer":            result.Text,
		"prompt_tokens":     result.PromptTokens,
		"completion_tokens": result.CompletionTokens,
		"total_tokens":      result.TotalTokens,
	})
}

// AskImageURL asks a question about an image from a URL
func (h *VQAHandler) AskImageURL(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	var req struct {
		Question string `json:"question" binding:"required,min=1,max=2000"`
		ImageURL string `json:"image_url" binding:"required,url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_ = userID // Future: track per-user usage

	// Fetch image from URL
	resp, err := http.Get(req.ImageURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to fetch image from URL"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to download image"})
		return
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read image data"})
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if !isValidImageType(contentType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type from URL"})
		return
	}

	// Process VQA request
	ctx := context.Background()
	result, err := h.llmService.AnswerWithImage(ctx, req.Question, imageData, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("VQA failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"answer":            result.Text,
		"prompt_tokens":     result.PromptTokens,
		"completion_tokens": result.CompletionTokens,
		"total_tokens":      result.TotalTokens,
	})
}

// AskWithContext asks a question about an image with context from the knowledge base
func (h *VQAHandler) AskWithContext(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authenticated"})
		return
	}

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse multipart form"})
		return
	}

	// Get question from form
	question := strings.TrimSpace(c.PostForm("question"))
	if question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}

	// Get optional document_ids
	var documentIDs []string
	if idsStr := c.PostForm("document_ids"); idsStr != "" {
		if err := json.Unmarshal([]byte(idsStr), &documentIDs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document_ids format"})
			return
		}
	}

	// Get image file
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image file is required"})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !isValidImageType(contentType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type"})
		return
	}

	imageData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read image"})
		return
	}

	ctx := context.Background()

	// First, answer the image question
	vqaResult, err := h.llmService.AnswerWithImage(ctx, question, imageData, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("VQA failed: %v", err)})
		return
	}

	// Optionally enhance with knowledge base context if document_ids provided
	var enhancedAnswer string
	if len(documentIDs) > 0 {
		// Retrieve relevant context
		docs, err := h.llmService.RetrieveContext(ctx, question, 3, service.RetrievalOptions{
			UserID:      userID,
			DocumentIDs: documentIDs,
		})
		if err == nil && len(docs) > 0 {
			// Build context
			var contextBuilder strings.Builder
			contextBuilder.WriteString("Image analysis: " + vqaResult.Text + "\n\n")
			contextBuilder.WriteString("Additional context from documents:\n")
			for i, doc := range docs {
				contextBuilder.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.PageContent))
			}

			// Generate enhanced answer
			prompt := fmt.Sprintf(`Based on the following information, provide a comprehensive answer to the question.

%s

Question: %s

Provide a detailed answer that combines both the image analysis and the additional context.`, contextBuilder.String(), question)

			enhanced, err := h.llmService.GenerateAnswer(ctx, prompt)
			if err == nil {
				enhancedAnswer = enhanced.Text
			}
		}
	}

	answer := vqaResult.Text
	if enhancedAnswer != "" {
		answer = enhancedAnswer
	}

	c.JSON(http.StatusOK, gin.H{
		"answer":            answer,
		"image_answer":      vqaResult.Text, // Original VQA answer before enhancement
		"prompt_tokens":     vqaResult.PromptTokens,
		"completion_tokens": vqaResult.CompletionTokens,
		"total_tokens":      vqaResult.TotalTokens,
		"context_enhanced":   enhancedAnswer != "",
	})
}

// isValidImageType validates the image content type
func isValidImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
	}
	for _, t := range validTypes {
		if contentType == t {
			return true
		}
	}
	return false
}