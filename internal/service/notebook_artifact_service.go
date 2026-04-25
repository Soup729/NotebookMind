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
	"github.com/tmc/langchaingo/llms"
)

type NotebookArtifactService interface {
	ListArtifacts(ctx context.Context, userID, notebookID string) ([]models.NotebookArtifact, error)
	GetArtifact(ctx context.Context, userID, notebookID, artifactID string) (*models.NotebookArtifact, error)
	GenerateArtifact(ctx context.Context, userID, notebookID, artifactType string) (*models.NotebookArtifact, error)
	DeleteArtifact(ctx context.Context, userID, notebookID, artifactID string) error
}

type notebookArtifactService struct {
	repo     repository.NotebookRepository
	llm      llms.Model
	generate func(ctx context.Context, prompt string) (string, error)
}

type artifactSourceRef struct {
	DocumentID   string    `json:"document_id"`
	DocumentName string    `json:"document_name"`
	Page         int       `json:"page"`
	ChunkType    string    `json:"chunk_type,omitempty"`
	BBox         []float32 `json:"bbox,omitempty"`
	Quote        string    `json:"quote"`
}

type artifactSection struct {
	Heading string   `json:"heading"`
	Bullets []string `json:"bullets"`
}

type artifactContent struct {
	Title      string              `json:"title"`
	Summary    string              `json:"summary"`
	Sections   []artifactSection   `json:"sections"`
	SourceRefs []artifactSourceRef `json:"source_refs"`
}

func NewNotebookArtifactService(repo repository.NotebookRepository, llm llms.Model) NotebookArtifactService {
	return &notebookArtifactService{
		repo: repo,
		llm:  llm,
		generate: func(ctx context.Context, prompt string) (string, error) {
			return llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		},
	}
}

func (s *notebookArtifactService) ListArtifacts(ctx context.Context, userID, notebookID string) ([]models.NotebookArtifact, error) {
	if _, err := s.repo.GetByID(ctx, userID, notebookID); err != nil {
		return nil, err
	}
	return s.repo.ListArtifacts(ctx, userID, notebookID)
}

func (s *notebookArtifactService) GetArtifact(ctx context.Context, userID, notebookID, artifactID string) (*models.NotebookArtifact, error) {
	return s.repo.GetArtifact(ctx, userID, notebookID, artifactID)
}

func (s *notebookArtifactService) DeleteArtifact(ctx context.Context, userID, notebookID, artifactID string) error {
	return s.repo.DeleteArtifact(ctx, userID, notebookID, artifactID)
}

func (s *notebookArtifactService) GenerateArtifact(ctx context.Context, userID, notebookID, artifactType string) (*models.NotebookArtifact, error) {
	artifactType = strings.TrimSpace(artifactType)
	if !isSupportedNotebookArtifactType(artifactType) {
		return nil, fmt.Errorf("unsupported notebook artifact type: %s", artifactType)
	}
	notebook, err := s.repo.GetByID(ctx, userID, notebookID)
	if err != nil {
		return nil, err
	}
	documents, err := s.repo.ListDocuments(ctx, notebookID)
	if err != nil {
		return nil, err
	}
	completed := completedArtifactDocuments(documents)
	if len(completed) == 0 {
		return nil, fmt.Errorf("notebook has no completed documents")
	}

	prompt := buildNotebookArtifactPrompt(notebook, completed, artifactType)
	raw, err := s.generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate notebook artifact: %w", err)
	}
	content, err := parseNotebookArtifactContent(raw)
	if err != nil {
		return nil, err
	}
	if len(content.SourceRefs) == 0 {
		content.SourceRefs = fallbackArtifactSourceRefs(completed)
	}
	sourceRefsJSON, _ := json.Marshal(content.SourceRefs)
	contentJSON, _ := json.Marshal(content)
	now := time.Now()
	artifact := &models.NotebookArtifact{
		ID:             uuid.NewString(),
		NotebookID:     notebookID,
		UserID:         userID,
		Type:           artifactType,
		Title:          artifactTitle(artifactType, content.Title),
		ContentJSON:    string(contentJSON),
		SourceRefsJSON: string(sourceRefsJSON),
		Status:         models.ArtifactStatusCompleted,
		Version:        1,
		GeneratedAt:    &now,
	}
	if err := s.repo.UpsertArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func isSupportedNotebookArtifactType(artifactType string) bool {
	switch artifactType {
	case models.ArtifactTypeBriefing, models.ArtifactTypeComparison, models.ArtifactTypeTimeline, models.ArtifactTypeTopicClusters, models.ArtifactTypeStudyPack:
		return true
	default:
		return false
	}
}

func completedArtifactDocuments(documents []models.Document) []models.Document {
	out := make([]models.Document, 0, len(documents))
	for _, doc := range documents {
		if doc.Status == models.DocumentStatusCompleted {
			out = append(out, doc)
		}
	}
	return out
}

func buildNotebookArtifactPrompt(notebook *models.Notebook, documents []models.Document, artifactType string) string {
	var builder strings.Builder
	builder.WriteString("You generate NotebookLM-style research artifacts. Return strict JSON only with fields: title, summary, sections, source_refs.\n")
	builder.WriteString("Every important conclusion must be traceable to source_refs. Do not invent facts not present in the document summaries.\n")
	builder.WriteString("Artifact type: ")
	builder.WriteString(artifactType)
	builder.WriteString("\nNotebook title: ")
	builder.WriteString(notebook.Title)
	builder.WriteString("\n\nDocuments:\n")
	for _, doc := range documents {
		builder.WriteString("- document_id: ")
		builder.WriteString(doc.ID)
		builder.WriteString("\n  document_name: ")
		builder.WriteString(doc.FileName)
		builder.WriteString("\n  summary: ")
		summary := strings.TrimSpace(doc.Summary)
		if summary == "" {
			summary = strings.TrimSpace(doc.FileName)
		}
		builder.WriteString(summary)
		builder.WriteString("\n")
	}
	builder.WriteString("\nSource refs must use existing document_id/document_name and include page=1 when exact page is unknown.\n")
	return builder.String()
}

func parseNotebookArtifactContent(raw string) (artifactContent, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	var content artifactContent
	if err := json.Unmarshal([]byte(cleaned), &content); err != nil {
		return artifactContent{}, fmt.Errorf("parse notebook artifact JSON: %w", err)
	}
	if strings.TrimSpace(content.Title) == "" {
		return artifactContent{}, fmt.Errorf("artifact JSON missing title")
	}
	return content, nil
}

func fallbackArtifactSourceRefs(documents []models.Document) []artifactSourceRef {
	refs := make([]artifactSourceRef, 0, len(documents))
	for _, doc := range documents {
		quote := strings.TrimSpace(doc.Summary)
		if quote == "" {
			quote = doc.FileName
		}
		refs = append(refs, artifactSourceRef{
			DocumentID:   doc.ID,
			DocumentName: doc.FileName,
			Page:         1,
			Quote:        quote,
		})
	}
	return refs
}

func artifactTitle(artifactType, generatedTitle string) string {
	generatedTitle = strings.TrimSpace(generatedTitle)
	if generatedTitle != "" {
		return generatedTitle
	}
	switch artifactType {
	case models.ArtifactTypeBriefing:
		return "Notebook Briefing"
	case models.ArtifactTypeComparison:
		return "Cross-document Comparison"
	case models.ArtifactTypeTimeline:
		return "Notebook Timeline"
	case models.ArtifactTypeTopicClusters:
		return "Topic Clusters"
	case models.ArtifactTypeStudyPack:
		return "Study Pack"
	default:
		return "Notebook Artifact"
	}
}
