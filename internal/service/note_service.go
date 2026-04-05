package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ============ 研究笔记服务 ============

// NoteService 研究笔记接口
type NoteService interface {
	// Create 创建笔记
	Create(ctx context.Context, userID string, req *CreateNoteRequest) (*models.Note, error)
	// Update 更新笔记
	Update(ctx context.Context, userID, noteID string, req *UpdateNoteRequest) (*models.Note, error)
	// Delete 删除笔记
	Delete(ctx context.Context, userID, noteID string) error
	// Get 获取单个笔记
	Get(ctx context.Context, userID, noteID string) (*models.Note, error)
	// List 列出用户笔记
	List(ctx context.Context, userID string, req *ListNotesRequest) (*ListNotesResponse, error)
	// Pin 钉住/取消钉住
	Pin(ctx context.Context, userID, noteID string) error
	// AddTag 添加标签
	AddTag(ctx context.Context, userID, noteID, tag string) error
	// RemoveTag 移除标签
	RemoveTag(ctx context.Context, userID, noteID, tag string) error
	// SearchByTag 按标签搜索
	SearchByTag(ctx context.Context, userID, tag string) ([]*models.Note, error)
}

// CreateNoteRequest 创建笔记请求
type CreateNoteRequest struct {
	NotebookID string            `json:"notebook_id"`
	SessionID  string            `json:"session_id"`
	Title      string            `json:"title" binding:"required"`
	Content    string            `json:"content" binding:"required"`
	Type       string            `json:"type"` // ai_response, original_text, summary, custom
	IsPinned   bool              `json:"is_pinned"`
	Tags       []string          `json:"tags"`
	Metadata   map[string]string `json:"metadata"`
}

// UpdateNoteRequest 更新笔记请求
type UpdateNoteRequest struct {
	Title     *string   `json:"title"`
	Content   *string   `json:"content"`
	IsPinned  *bool     `json:"is_pinned"`
	Tags      *[]string `json:"tags"`
	NotebookID *string  `json:"notebook_id"`
}

// ListNotesRequest 列出笔记请求
type ListNotesRequest struct {
	NotebookID string `json:"notebook_id"`
	SessionID  string `json:"session_id"`
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	PinnedOnly bool   `json:"pinned_only"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

// ListNotesResponse 列出笔记响应
type ListNotesResponse struct {
	Items      []*models.Note `json:"items"`
	TotalCount int            `json:"total_count"`
	Page       int            `json:"page"`
	PageSize   int            `json:"page_size"`
}

// noteService 实现
type noteService struct {
	noteRepo repository.NoteRepository
}

// NewNoteService 创建笔记服务
func NewNoteService(noteRepo repository.NoteRepository) NoteService {
	return &noteService{noteRepo: noteRepo}
}

// Create 创建笔记
func (s *noteService) Create(ctx context.Context, userID string, req *CreateNoteRequest) (*models.Note, error) {
	noteType := req.Type
	if noteType == "" {
		noteType = "custom"
	}

	// 序列化标签
	var tagsJSON string
	if len(req.Tags) > 0 {
		tagsBytes, _ := json.Marshal(req.Tags)
		tagsJSON = string(tagsBytes)
	}

	// 序列化元数据
	var metadataJSON string
	if len(req.Metadata) > 0 {
		metadataBytes, _ := json.Marshal(req.Metadata)
		metadataJSON = string(metadataBytes)
	}

	note := &models.Note{
		ID:         uuid.NewString(),
		UserID:     userID,
		NotebookID: req.NotebookID,
		SessionID:  req.SessionID,
		Title:      req.Title,
		Content:    req.Content,
		Type:       noteType,
		IsPinned:   req.IsPinned,
		Tags:       tagsJSON,
		Metadata:   metadataJSON,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := s.noteRepo.Create(ctx, note); err != nil {
		return nil, fmt.Errorf("create note: %w", err)
	}

	zap.L().Info("note created", zap.String("note_id", note.ID), zap.String("user_id", userID))
	return note, nil
}

// Update 更新笔记
func (s *noteService) Update(ctx context.Context, userID, noteID string, req *UpdateNoteRequest) (*models.Note, error) {
	note, err := s.noteRepo.GetByID(ctx, userID, noteID)
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}

	if req.Title != nil {
		note.Title = *req.Title
	}
	if req.Content != nil {
		note.Content = *req.Content
	}
	if req.IsPinned != nil {
		note.IsPinned = *req.IsPinned
	}
	if req.Tags != nil {
		tagsBytes, _ := json.Marshal(req.Tags)
		note.Tags = string(tagsBytes)
	}
	if req.NotebookID != nil {
		note.NotebookID = *req.NotebookID
	}
	note.UpdatedAt = time.Now()

	if err := s.noteRepo.Update(ctx, note); err != nil {
		return nil, fmt.Errorf("update note: %w", err)
	}

	return note, nil
}

// Delete 删除笔记
func (s *noteService) Delete(ctx context.Context, userID, noteID string) error {
	return s.noteRepo.Delete(ctx, userID, noteID)
}

// Get 获取笔记
func (s *noteService) Get(ctx context.Context, userID, noteID string) (*models.Note, error) {
	return s.noteRepo.GetByID(ctx, userID, noteID)
}

// List 列出笔记
func (s *noteService) List(ctx context.Context, userID string, req *ListNotesRequest) (*ListNotesResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}

	notes, total, err := s.noteRepo.List(ctx, userID, &repository.ListNotesFilter{
		NotebookID: req.NotebookID,
		SessionID:  req.SessionID,
		Type:       req.Type,
		Tag:        req.Tag,
		PinnedOnly: req.PinnedOnly,
	}, req.Page, req.PageSize)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}

	return &ListNotesResponse{
		Items:      notes,
		TotalCount: total,
		Page:       req.Page,
		PageSize:   req.PageSize,
	}, nil
}

// Pin 钉住笔记
func (s *noteService) Pin(ctx context.Context, userID, noteID string) error {
	note, err := s.noteRepo.GetByID(ctx, userID, noteID)
	if err != nil {
		return err
	}
	note.IsPinned = !note.IsPinned // 切换状态
	note.UpdatedAt = time.Now()
	return s.noteRepo.Update(ctx, note)
}

// AddTag 添加标签
func (s *noteService) AddTag(ctx context.Context, userID, noteID, tag string) error {
	note, err := s.noteRepo.GetByID(ctx, userID, noteID)
	if err != nil {
		return err
	}

	var tags []string
	if note.Tags != "" {
		json.Unmarshal([]byte(note.Tags), &tags)
	}

	// 检查是否已存在
	for _, t := range tags {
		if t == tag {
			return nil
		}
	}

	tags = append(tags, tag)
	tagsBytes, _ := json.Marshal(tags)
	note.Tags = string(tagsBytes)
	note.UpdatedAt = time.Now()

	return s.noteRepo.Update(ctx, note)
}

// RemoveTag 移除标签
func (s *noteService) RemoveTag(ctx context.Context, userID, noteID, tag string) error {
	note, err := s.noteRepo.GetByID(ctx, userID, noteID)
	if err != nil {
		return err
	}

	var tags []string
	if note.Tags != "" {
		json.Unmarshal([]byte(note.Tags), &tags)
	}

	// 过滤掉目标标签
	newTags := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}

	tagsBytes, _ := json.Marshal(newTags)
	note.Tags = string(tagsBytes)
	note.UpdatedAt = time.Now()

	return s.noteRepo.Update(ctx, note)
}

// SearchByTag 按标签搜索
func (s *noteService) SearchByTag(ctx context.Context, userID, tag string) ([]*models.Note, error) {
	notes, _, err := s.noteRepo.List(ctx, userID, &repository.ListNotesFilter{
		Tag: tag,
	}, 1, 100)
	if err != nil {
		return nil, err
	}

	// 进一步过滤（因为 List 可能用 LIKE 匹配）
	var result []*models.Note
	for _, note := range notes {
		var tags []string
		if note.Tags != "" {
			json.Unmarshal([]byte(note.Tags), &tags)
		}
		for _, t := range tags {
			if strings.TrimSpace(t) == tag {
				result = append(result, note)
				break
			}
		}
	}

	return result, nil
}

// ============ 笔记处理器辅助函数 ============

// ParseNoteTags 解析笔记标签
func ParseNoteTags(tagsJSON string) []string {
	if tagsJSON == "" {
		return nil
	}
	var tags []string
	json.Unmarshal([]byte(tagsJSON), &tags)
	return tags
}

// ParseNoteMetadata 解析笔记元数据
func ParseNoteMetadata(metadataJSON string) map[string]string {
	if metadataJSON == "" {
		return nil
	}
	var metadata map[string]string
	json.Unmarshal([]byte(metadataJSON), &metadata)
	return metadata
}

// NoteResponse 笔记响应结构
type NoteResponse struct {
	ID         string            `json:"id"`
	NotebookID string            `json:"notebook_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	Title      string            `json:"title"`
	Content    string            `json:"content"`
	Type       string            `json:"type"`
	IsPinned   bool              `json:"is_pinned"`
	Tags       []string          `json:"tags"`
	Metadata   map[string]string `json:"metadata"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}
