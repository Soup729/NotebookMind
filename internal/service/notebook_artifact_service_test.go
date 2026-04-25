package service

import (
	"context"
	"strings"
	"testing"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
)

type fakeNotebookArtifactRepo struct {
	notebooks map[string]*models.Notebook
	documents []models.Document
	artifacts map[string]*models.NotebookArtifact
}

func (f *fakeNotebookArtifactRepo) Create(context.Context, *models.Notebook) error { return nil }
func (f *fakeNotebookArtifactRepo) GetByID(_ context.Context, userID, notebookID string) (*models.Notebook, error) {
	nb := f.notebooks[notebookID]
	if nb == nil || nb.UserID != userID {
		return nil, repository.ErrNotFound
	}
	return nb, nil
}
func (f *fakeNotebookArtifactRepo) ListByUser(context.Context, string) ([]models.Notebook, error) {
	return nil, nil
}
func (f *fakeNotebookArtifactRepo) Update(context.Context, *models.Notebook) error       { return nil }
func (f *fakeNotebookArtifactRepo) Delete(context.Context, string, string) error         { return nil }
func (f *fakeNotebookArtifactRepo) AddDocument(context.Context, string, string) error    { return nil }
func (f *fakeNotebookArtifactRepo) RemoveDocument(context.Context, string, string) error { return nil }
func (f *fakeNotebookArtifactRepo) ListDocuments(context.Context, string) ([]models.Document, error) {
	return f.documents, nil
}
func (f *fakeNotebookArtifactRepo) GetDocumentIDs(context.Context, string) ([]string, error) {
	return nil, nil
}
func (f *fakeNotebookArtifactRepo) UpsertGuide(context.Context, *models.DocumentGuide) error {
	return nil
}
func (f *fakeNotebookArtifactRepo) GetGuide(context.Context, string) (*models.DocumentGuide, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeNotebookArtifactRepo) UpsertArtifact(_ context.Context, artifact *models.NotebookArtifact) error {
	if f.artifacts == nil {
		f.artifacts = make(map[string]*models.NotebookArtifact)
	}
	f.artifacts[artifact.ID] = artifact
	return nil
}
func (f *fakeNotebookArtifactRepo) GetArtifact(_ context.Context, userID, notebookID, artifactID string) (*models.NotebookArtifact, error) {
	artifact := f.artifacts[artifactID]
	if artifact == nil || artifact.UserID != userID || artifact.NotebookID != notebookID {
		return nil, repository.ErrNotFound
	}
	return artifact, nil
}
func (f *fakeNotebookArtifactRepo) GetArtifactByID(_ context.Context, artifactID string) (*models.NotebookArtifact, error) {
	artifact := f.artifacts[artifactID]
	if artifact == nil {
		return nil, repository.ErrNotFound
	}
	return artifact, nil
}
func (f *fakeNotebookArtifactRepo) ListArtifacts(context.Context, string, string) ([]models.NotebookArtifact, error) {
	return nil, nil
}
func (f *fakeNotebookArtifactRepo) DeleteArtifact(context.Context, string, string, string) error {
	return nil
}

func TestGenerateNotebookArtifactRequiresSupportedType(t *testing.T) {
	svc := NewNotebookArtifactService(&fakeNotebookArtifactRepo{}, nil)

	_, err := svc.GenerateArtifact(context.Background(), "user-1", "nb-1", "unknown")
	if err == nil {
		t.Fatalf("expected unsupported artifact type error")
	}
}

func TestGenerateNotebookArtifactPersistsBriefingWithSourceRefs(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		notebooks: map[string]*models.Notebook{
			"nb-1": {ID: "nb-1", UserID: "user-1", Title: "Research"},
		},
		documents: []models.Document{
			{ID: "doc-1", FileName: "Annual_Report_2024.pdf", Summary: "Revenue grew to $1.85B.", Status: models.DocumentStatusCompleted},
			{ID: "doc-2", FileName: "Risk_Assessment_2024.pdf", Summary: "Cybersecurity risk partially materialized.", Status: models.DocumentStatusCompleted},
		},
	}
	svc := &notebookArtifactService{
		repo: repo,
		generate: func(context.Context, string) (string, error) {
			return `{"title":"Notebook Briefing","summary":"Revenue grew while cybersecurity remained a key risk.","sections":[{"heading":"Findings","bullets":["Revenue reached $1.85B.","Cybersecurity risk partially materialized."]}],"source_refs":[{"document_id":"doc-1","document_name":"Annual_Report_2024.pdf","page":1,"quote":"Revenue grew to $1.85B."},{"document_id":"doc-2","document_name":"Risk_Assessment_2024.pdf","page":1,"quote":"Cybersecurity risk partially materialized."}]}`, nil
		},
	}

	artifact, err := svc.GenerateArtifact(context.Background(), "user-1", "nb-1", models.ArtifactTypeBriefing)
	if err != nil {
		t.Fatalf("GenerateArtifact returned error: %v", err)
	}
	if artifact.Type != models.ArtifactTypeBriefing {
		t.Fatalf("expected briefing artifact, got %q", artifact.Type)
	}
	if !strings.Contains(artifact.ContentJSON, "Notebook Briefing") {
		t.Fatalf("expected content JSON to be persisted, got %s", artifact.ContentJSON)
	}
	if !strings.Contains(artifact.SourceRefsJSON, "doc-1") || !strings.Contains(artifact.SourceRefsJSON, "doc-2") {
		t.Fatalf("expected source refs to be persisted, got %s", artifact.SourceRefsJSON)
	}
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed artifact, got %q", artifact.Status)
	}
}
