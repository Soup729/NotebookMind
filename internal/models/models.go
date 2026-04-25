package models

import (
	"time"

	"gorm.io/gorm"
)

const (
	DocumentStatusProcessing = "processing"
	DocumentStatusCompleted  = "completed"
	DocumentStatusFailed     = "failed"

	NotebookStatusActive   = "active"
	NotebookStatusArchived = "archived"

	GuideStatusPending   = "pending"
	GuideStatusCompleted = "completed"
	GuideStatusFailed    = "failed"

	ArtifactStatusPending      = "pending"
	ArtifactStatusOutlineReady = "outline_ready"
	ArtifactStatusGenerating   = "generating"
	ArtifactStatusCompleted    = "completed"
	ArtifactStatusFailed       = "failed"

	ArtifactTypeBriefing       = "briefing"
	ArtifactTypeComparison     = "comparison"
	ArtifactTypeTimeline       = "timeline"
	ArtifactTypeTopicClusters  = "topic_clusters"
	ArtifactTypeStudyPack      = "study_pack"
	ArtifactTypeSessionMemory  = "session_memory"
	ArtifactTypeExportMarkdown = "export_markdown"
	ArtifactTypeExportMindmap  = "export_mindmap"
	ArtifactTypeExportDocx     = "export_docx"
	ArtifactTypeExportPptx     = "export_pptx"
	ArtifactTypeExportPDF      = "export_pdf"
)

type User struct {
	ID           string    `gorm:"primaryKey;size:36"`
	Email        string    `gorm:"uniqueIndex;size:320;not null"`
	Name         string    `gorm:"size:120;not null"`
	PasswordHash string    `gorm:"type:text;not null"`
	CreatedAt    time.Time `gorm:"index"`
	UpdatedAt    time.Time
}

// Notebook represents a virtual notebook for organizing documents (like Google NotebookLM)
type Notebook struct {
	ID          string    `gorm:"primaryKey;size:36"`
	UserID      string    `gorm:"index;size:36;not null"`
	Title       string    `gorm:"size:255;not null"`
	Description string    `gorm:"type:text"`
	Status      string    `gorm:"index;size:32;not null;default:'active'"`
	DocumentCnt int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time
}

// NotebookDocument represents the many-to-many relationship between notebooks and documents
type NotebookDocument struct {
	ID         string    `gorm:"primaryKey;size:36"`
	NotebookID string    `gorm:"index;size:36;not null"`
	DocumentID string    `gorm:"index;size:36;not null"`
	AddedAt    time.Time `gorm:"index"`
}

type Document struct {
	ID               string `gorm:"primaryKey;size:36"`
	UserID           string `gorm:"index;size:36;not null"`
	FileName         string `gorm:"size:512;not null"`
	StoredPath       string `gorm:"column:file_path;type:text;not null"`
	LegacyStoredPath string `gorm:"column:stored_path;type:text;not null"`
	Status           string `gorm:"index;size:32;not null"`
	ErrorMessage     string `gorm:"type:text"`
	FileSize         int64  `gorm:"not null"`
	ChunkCount       int    `gorm:"not null;default:0"`
	ProcessedAt      *time.Time
	CreatedAt        time.Time `gorm:"index"`
	UpdatedAt        time.Time

	// NotebookLM specific fields
	NotebookID  string `gorm:"index;size:36"`
	Summary     string `gorm:"type:text"`     // Auto-generated document summary
	FaqJSON     string `gorm:"type:text"`     // Auto-generated FAQ in JSON format
	GuideStatus string `gorm:"index;size:32"` // pending, completed, failed
	GuideError  string `gorm:"type:text"`
}

// DocumentGuide stores auto-generated summaries and FAQs for documents
type DocumentGuide struct {
	ID          string `gorm:"primaryKey;size:36"`
	DocumentID  string `gorm:"uniqueIndex;size:36;not null"`
	Summary     string `gorm:"type:text;not null"`
	FaqJSON     string `gorm:"type:text"` // JSON array of {question, answer}
	KeyPoints   string `gorm:"type:text"` // Key takeaways
	Status      string `gorm:"index;size:32;not null;default:'pending'"`
	ErrorMsg    string `gorm:"type:text"`
	GeneratedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NotebookArtifact stores NotebookLM-style reusable research outputs.
type NotebookArtifact struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	NotebookID     string     `gorm:"index;size:36;not null" json:"notebook_id"`
	UserID         string     `gorm:"index;size:36;not null" json:"user_id"`
	Type           string     `gorm:"index;size:64;not null" json:"type"`
	Title          string     `gorm:"size:255;not null" json:"title"`
	ContentJSON    string     `gorm:"type:text;not null" json:"content_json"`
	SourceRefsJSON string     `gorm:"type:text" json:"source_refs_json"`
	RequestJSON    string     `gorm:"type:text" json:"request_json,omitempty"`
	FilePath       string     `gorm:"type:text" json:"file_path,omitempty"`
	FileName       string     `gorm:"size:512" json:"file_name,omitempty"`
	MimeType       string     `gorm:"size:128" json:"mime_type,omitempty"`
	TaskID         string     `gorm:"size:128;index" json:"task_id,omitempty"`
	Status         string     `gorm:"index;size:32;not null;default:'pending'" json:"status"`
	ErrorMsg       string     `gorm:"type:text" json:"error_msg,omitempty"`
	Version        int        `gorm:"not null;default:1" json:"version"`
	GeneratedAt    *time.Time `json:"generated_at,omitempty"`
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (d *Document) BeforeSave(_ *gorm.DB) error {
	if d.StoredPath == "" && d.LegacyStoredPath != "" {
		d.StoredPath = d.LegacyStoredPath
	}
	if d.LegacyStoredPath == "" && d.StoredPath != "" {
		d.LegacyStoredPath = d.StoredPath
	}
	return nil
}

func (d *Document) AfterFind(_ *gorm.DB) error {
	if d.StoredPath == "" && d.LegacyStoredPath != "" {
		d.StoredPath = d.LegacyStoredPath
	}
	if d.LegacyStoredPath == "" && d.StoredPath != "" {
		d.LegacyStoredPath = d.StoredPath
	}
	return nil
}

type ChatSession struct {
	ID                 string     `gorm:"primaryKey;size:36"`
	UserID             string     `gorm:"index;size:36;not null"`
	NotebookID         string     `gorm:"index;size:36;not null"` // NotebookLM: 关联笔记本
	Title              string     `gorm:"size:255;not null"`
	LastMessageAt      time.Time  `gorm:"index"`
	MemorySummary      string     `gorm:"type:text"`
	MemoryJSON         string     `gorm:"type:text"`
	MemoryMessageCount int        `gorm:"not null;default:0"`
	MemoryUpdatedAt    *time.Time `gorm:"index"`
	CreatedAt          time.Time  `gorm:"index"`
	UpdatedAt          time.Time
}

type ChatMessage struct {
	ID               string    `gorm:"primaryKey;size:36"`
	SessionID        string    `gorm:"index;size:36;not null"`
	UserID           string    `gorm:"index;size:36;not null"`
	Role             string    `gorm:"size:16;not null"`
	Content          string    `gorm:"type:text;not null"`
	SourcesJSON      string    `gorm:"type:text"`
	PromptTokens     int       `gorm:"not null;default:0"`
	CompletionTokens int       `gorm:"not null;default:0"`
	TotalTokens      int       `gorm:"not null;default:0"`
	CreatedAt        time.Time `gorm:"index"`
}

type DocumentChunk struct {
	ID            string    `gorm:"primaryKey;size:36"`
	UserID        string    `gorm:"index;size:36;not null"`
	DocumentID    string    `gorm:"index;size:36;not null"`
	FileName      string    `gorm:"size:512;not null"`
	ChunkIndex    int       `gorm:"not null"`
	Content       string    `gorm:"type:text;not null"`
	EmbeddingJSON string    `gorm:"type:text;not null"`
	CreatedAt     time.Time `gorm:"index"`
}
