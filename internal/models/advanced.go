package models

import (
	"encoding/json"
	"time"
)

// ============ 多租户支持 ============

// Tenant represents an organization/tenant in the system
type Tenant struct {
	ID        string    `gorm:"primaryKey;size:36"`
	Name      string    `gorm:"size:255;not null"`
	Plan      string    `gorm:"size:32;not null;default:'free'"` // free, pro, enterprise
	Status    string    `gorm:"size:32;not null;default:'active'"` // active, suspended
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
}

// User tenant extension
type UserTenant struct {
	UserID   string `gorm:"primaryKey;size:36"`
	TenantID string `gorm:"primaryKey;size:36"`
	Role     string `gorm:"size:32;not null;default:'member'"` // owner, admin, member
}

// ============ 研究笔记 ============

// Note represents a research note pinned by user
type Note struct {
	ID        string    `gorm:"primaryKey;size:36"`
	UserID    string    `gorm:"index;size:36;not null"`
	NotebookID string   `gorm:"index;size:36"` // Optional: associated notebook
	SessionID string   `gorm:"index;size:36"`  // Optional: associated chat session
	Title     string   `gorm:"size:255;not null"`
	Content   string   `gorm:"type:text;not null"`
	Type      string   `gorm:"size:32;not null"` // ai_response, original_text, summary, custom
	IsPinned  bool     `gorm:"not null;default:false"`
	Tags      string   `gorm:"type:text"` // JSON array of tags
	Metadata  string   `gorm:"type:text"` // JSON for additional metadata (e.g., source document IDs)
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time
}

// ============ 增强文档模型 ============

// DocumentChunkExt 扩展文档切片，包含坐标信息和父子关系
type DocumentChunkExt struct {
	ID         string  `gorm:"primaryKey;size:36"`
	UserID     string  `gorm:"index;size:36;not null"`
	DocumentID string  `gorm:"index;size:36;not null"`
	ChunkIndex int     `gorm:"not null"`
	Content    string  `gorm:"type:text;not null"`
	// 父子切片关系
	ParentID   string  `gorm:"index;size:36"` // 父切片ID (用于提供完整上下文)
	// 坐标信息 (用于前端高亮)
	BoundingBox []float32 `gorm:"type:float8[]"` // [x0, y0, x1, y1]
	PageNum    int        `gorm:"not null;default:1"`
	// 块类型
	BlockType  string  `gorm:"size:32;not null"` // text, table, heading, list, image
	TableHTML  string  `gorm:"type:text"`        // 如果是表格，存储HTML格式
	CreatedAt  time.Time `gorm:"index"`
}

// ============ 混合检索相关 ============

// SearchQueryLog 存储查询日志用于分析
type SearchQueryLog struct {
	ID            string    `gorm:"primaryKey;size:36"`
	UserID        string    `gorm:"index;size:36;not null"`
	NotebookID    string    `gorm:"index;size:36"`
	OriginalQuery string    `gorm:"type:text;not null"`
	RewrittenQuery string   `gorm:"type:text"` // 意图重写后的查询
	Intent        string   `gorm:"size:64"`   // 识别的查询意图
	ResultsCount  int      `gorm:"not null;default:0"`
	LatencyMs     int      `gorm:"not null;default:0"`
	CreatedAt     time.Time `gorm:"index"`
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

// ToResponse 将 Note 转换为 NoteResponse
func (n *Note) ToResponse() *NoteResponse {
	return &NoteResponse{
		ID:         n.ID,
		NotebookID: n.NotebookID,
		SessionID:  n.SessionID,
		Title:      n.Title,
		Content:    n.Content,
		Type:       n.Type,
		IsPinned:   n.IsPinned,
		Tags:       parseNoteTags(n.Tags),
		Metadata:   parseNoteMetadata(n.Metadata),
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}
}

// parseNoteTags 解析笔记标签
func parseNoteTags(tagsJSON string) []string {
	if tagsJSON == "" {
		return nil
	}
	var tags []string
	json.Unmarshal([]byte(tagsJSON), &tags)
	return tags
}

// parseNoteMetadata 解析笔记元数据
func parseNoteMetadata(metadataJSON string) map[string]string {
	if metadataJSON == "" {
		return nil
	}
	var metadata map[string]string
	json.Unmarshal([]byte(metadataJSON), &metadata)
	return metadata
}
