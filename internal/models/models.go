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
	ID               string     `gorm:"primaryKey;size:36"`
	UserID           string     `gorm:"index;size:36;not null"`
	FileName         string     `gorm:"size:512;not null"`
	StoredPath       string     `gorm:"column:file_path;type:text;not null"`
	LegacyStoredPath string     `gorm:"column:stored_path;type:text;not null"`
	Status           string     `gorm:"index;size:32;not null"`
	ErrorMessage     string     `gorm:"type:text"`
	FileSize         int64      `gorm:"not null"`
	ChunkCount       int        `gorm:"not null;default:0"`
	ProcessedAt      *time.Time
	CreatedAt        time.Time  `gorm:"index"`
	UpdatedAt        time.Time

	// NotebookLM specific fields
	NotebookID   string           `gorm:"index;size:36"`
	Summary      string           `gorm:"type:text"`          // Auto-generated document summary
	FaqJSON      string           `gorm:"type:text"`          // Auto-generated FAQ in JSON format
	GuideStatus  string           `gorm:"index;size:32"`      // pending, completed, failed
	GuideError   string           `gorm:"type:text"`
}

// DocumentGuide stores auto-generated summaries and FAQs for documents
type DocumentGuide struct {
	ID          string    `gorm:"primaryKey;size:36"`
	DocumentID  string    `gorm:"uniqueIndex;size:36;not null"`
	Summary     string    `gorm:"type:text;not null"`
	FaqJSON     string    `gorm:"type:text"` // JSON array of {question, answer}
	KeyPoints   string    `gorm:"type:text"` // Key takeaways
	Status      string    `gorm:"index;size:32;not null;default:'pending'"`
	ErrorMsg    string    `gorm:"type:text"`
	GeneratedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	ID            string    `gorm:"primaryKey;size:36"`
	UserID        string    `gorm:"index;size:36;not null"`
	NotebookID    string    `gorm:"index;size:36;not null"` // NotebookLM: 关联笔记本
	Title         string    `gorm:"size:255;not null"`
	LastMessageAt time.Time `gorm:"index"`
	CreatedAt     time.Time `gorm:"index"`
	UpdatedAt     time.Time
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
