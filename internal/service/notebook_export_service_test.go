package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"NotebookAI/internal/models"
	"NotebookAI/internal/worker/tasks"
)

type fakeExportProducer struct {
	payload tasks.RenderNotebookExportPayload
	called  bool
}

func (f *fakeExportProducer) EnqueueRenderNotebookExport(_ context.Context, payload tasks.RenderNotebookExportPayload) (string, error) {
	f.payload = payload
	f.called = true
	return "task-1", nil
}

func TestCreateExportOutlineFallsBackWhenLLMReturnsInvalidJSON(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		notebooks: map[string]*models.Notebook{
			"nb-1": {ID: "nb-1", UserID: "user-1", Title: "Research Notebook"},
		},
		documents: []models.Document{
			{ID: "doc-1", FileName: "Annual_Report_2024.pdf", Summary: "Revenue reached $1.85B.", Status: models.DocumentStatusCompleted},
			{ID: "doc-2", FileName: "Risk_Assessment_2024.pdf", Summary: "Cybersecurity remained a material risk.", Status: models.DocumentStatusCompleted},
		},
	}
	svc := &notebookExportService{
		repo: repo,
		generate: func(context.Context, string) (string, error) {
			return "not json", nil
		},
	}

	artifact, err := svc.CreateOutline(context.Background(), "user-1", "nb-1", ExportOutlineRequest{
		Format:       "markdown",
		DocumentIDs:  []string{"doc-1"},
		Requirements: "生成管理层总结",
	})
	if err != nil {
		t.Fatalf("CreateOutline returned error: %v", err)
	}
	if artifact.Status != models.ArtifactStatusOutlineReady {
		t.Fatalf("expected outline_ready, got %q", artifact.Status)
	}
	if artifact.Type != models.ArtifactTypeExportMarkdown {
		t.Fatalf("expected markdown export type, got %q", artifact.Type)
	}
	if !strings.Contains(artifact.ContentJSON, `"outline"`) {
		t.Fatalf("expected outline content, got %s", artifact.ContentJSON)
	}
	if !strings.Contains(artifact.SourceRefsJSON, "doc-1") {
		t.Fatalf("expected selected source ref, got %s", artifact.SourceRefsJSON)
	}
	if strings.Contains(artifact.SourceRefsJSON, "doc-2") {
		t.Fatalf("expected unselected document to be excluded, got %s", artifact.SourceRefsJSON)
	}
}

func TestConfirmOutlineRejectsEmptyOutline(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:         "artifact-1",
				UserID:     "user-1",
				NotebookID: "nb-1",
				Type:       models.ArtifactTypeExportMarkdown,
				Status:     models.ArtifactStatusOutlineReady,
			},
		},
	}
	svc := &notebookExportService{repo: repo}

	_, err := svc.ConfirmOutline(context.Background(), "user-1", "nb-1", "artifact-1", nil)
	if err == nil {
		t.Fatalf("expected empty outline error")
	}
}

func TestConfirmOutlineEnqueuesRenderTask(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:          "artifact-1",
				UserID:      "user-1",
				NotebookID:  "nb-1",
				Type:        models.ArtifactTypeExportMarkdown,
				Status:      models.ArtifactStatusOutlineReady,
				ContentJSON: `{"format":"markdown","outline":[]}`,
			},
		},
	}
	producer := &fakeExportProducer{}
	svc := &notebookExportService{repo: repo, producer: producer}

	artifact, err := svc.ConfirmOutline(context.Background(), "user-1", "nb-1", "artifact-1", []ExportOutlineSection{
		{Heading: "核心结论", Bullets: []string{"收入增长"}},
	})
	if err != nil {
		t.Fatalf("ConfirmOutline returned error: %v", err)
	}
	if artifact.Status != models.ArtifactStatusGenerating {
		t.Fatalf("expected generating, got %q", artifact.Status)
	}
	if artifact.TaskID != "task-1" {
		t.Fatalf("expected task id to be stored, got %q", artifact.TaskID)
	}
	if !producer.called || producer.payload.ArtifactID != "artifact-1" {
		t.Fatalf("expected render task to be enqueued, got %+v", producer.payload)
	}
}

func TestRenderMarkdownExportCompletesArtifact(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:             "artifact-1",
				UserID:         "user-1",
				NotebookID:     "nb-1",
				Type:           models.ArtifactTypeExportMarkdown,
				Title:          "Management Brief",
				Status:         models.ArtifactStatusGenerating,
				ContentJSON:    `{"format":"markdown","outline":[{"heading":"核心结论","bullets":["收入增长"]}]}`,
				SourceRefsJSON: `[{"document_id":"doc-1","document_name":"Annual_Report_2024.pdf","page":1,"quote":"Revenue reached $1.85B."}]`,
			},
		},
	}
	svc := &notebookExportService{repo: repo, outputRoot: t.TempDir()}

	if err := svc.RenderExport(context.Background(), "artifact-1"); err != nil {
		t.Fatalf("RenderExport returned error: %v", err)
	}
	artifact := repo.artifacts["artifact-1"]
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed, got %q", artifact.Status)
	}
	if artifact.FilePath == "" || artifact.FileName == "" {
		t.Fatalf("expected file metadata, got path=%q name=%q", artifact.FilePath, artifact.FileName)
	}
	var content exportContent
	if err := json.Unmarshal([]byte(artifact.ContentJSON), &content); err != nil {
		t.Fatalf("unmarshal rendered content: %v", err)
	}
	if !strings.HasPrefix(content.RenderedText, "# Management Brief") {
		t.Fatalf("expected markdown heading, got %q", content.RenderedText)
	}
}

func TestRenderMindmapExportCompletesArtifact(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:          "artifact-1",
				UserID:      "user-1",
				NotebookID:  "nb-1",
				Type:        models.ArtifactTypeExportMindmap,
				Title:       "Research Map",
				Status:      models.ArtifactStatusGenerating,
				ContentJSON: `{"format":"mindmap","outline":[{"heading":"市场","bullets":["竞争加剧"]}]}`,
			},
		},
	}
	svc := &notebookExportService{repo: repo, outputRoot: t.TempDir()}

	if err := svc.RenderExport(context.Background(), "artifact-1"); err != nil {
		t.Fatalf("RenderExport returned error: %v", err)
	}
	artifact := repo.artifacts["artifact-1"]
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed, got %q", artifact.Status)
	}
	var content exportContent
	if err := json.Unmarshal([]byte(artifact.ContentJSON), &content); err != nil {
		t.Fatalf("unmarshal rendered content: %v", err)
	}
	if !strings.Contains(content.RenderedText, "mindmap") {
		t.Fatalf("expected mindmap output, got %q", content.RenderedText)
	}
}

func TestCreateExportOutlineSupportsOfficeFormats(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		notebooks: map[string]*models.Notebook{
			"nb-1": {ID: "nb-1", UserID: "user-1", Title: "Research Notebook"},
		},
		documents: []models.Document{
			{ID: "doc-1", FileName: "Annual_Report_2024.pdf", Summary: "Revenue reached $1.85B.", Status: models.DocumentStatusCompleted},
		},
	}
	svc := &notebookExportService{
		repo: repo,
		generate: func(context.Context, string) (string, error) {
			return "not json", nil
		},
	}

	cases := []struct {
		format       string
		artifactType string
	}{
		{format: "docx", artifactType: models.ArtifactTypeExportDocx},
		{format: "pptx", artifactType: models.ArtifactTypeExportPptx},
		{format: "pdf", artifactType: models.ArtifactTypeExportPDF},
	}
	for _, tc := range cases {
		artifact, err := svc.CreateOutline(context.Background(), "user-1", "nb-1", ExportOutlineRequest{
			Format:       tc.format,
			DocumentIDs:  []string{"doc-1"},
			Requirements: "扩展格式测试",
		})
		if err != nil {
			t.Fatalf("CreateOutline(%s) returned error: %v", tc.format, err)
		}
		if artifact.Type != tc.artifactType {
			t.Fatalf("expected %s artifact type, got %q", tc.artifactType, artifact.Type)
		}
	}
}

func TestRenderDocxExportCompletesArtifact(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:          "artifact-1",
				UserID:      "user-1",
				NotebookID:  "nb-1",
				Type:        models.ArtifactTypeExportDocx,
				Title:       "Management Brief",
				Status:      models.ArtifactStatusGenerating,
				ContentJSON: `{"format":"docx","outline":[{"heading":"核心结论","bullets":["收入增长"]}]}`,
			},
		},
	}
	svc := &notebookExportService{repo: repo, outputRoot: t.TempDir()}

	if err := svc.RenderExport(context.Background(), "artifact-1"); err != nil {
		t.Fatalf("RenderExport returned error: %v", err)
	}
	artifact := repo.artifacts["artifact-1"]
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed, got %q", artifact.Status)
	}
	if !strings.HasSuffix(artifact.FileName, ".docx") {
		t.Fatalf("expected docx file, got %q", artifact.FileName)
	}
}

func TestRenderPptxExportCompletesArtifact(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:          "artifact-1",
				UserID:      "user-1",
				NotebookID:  "nb-1",
				Type:        models.ArtifactTypeExportPptx,
				Title:       "Management Brief",
				Status:      models.ArtifactStatusGenerating,
				ContentJSON: `{"format":"pptx","outline":[{"heading":"核心结论","bullets":["收入增长"]}]}`,
			},
		},
	}
	svc := &notebookExportService{repo: repo, outputRoot: t.TempDir()}

	if err := svc.RenderExport(context.Background(), "artifact-1"); err != nil {
		t.Fatalf("RenderExport returned error: %v", err)
	}
	artifact := repo.artifacts["artifact-1"]
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed, got %q", artifact.Status)
	}
	if !strings.HasSuffix(artifact.FileName, ".pptx") {
		t.Fatalf("expected pptx file, got %q", artifact.FileName)
	}
}

func TestRenderPDFExportCompletesArtifact(t *testing.T) {
	repo := &fakeNotebookArtifactRepo{
		artifacts: map[string]*models.NotebookArtifact{
			"artifact-1": {
				ID:          "artifact-1",
				UserID:      "user-1",
				NotebookID:  "nb-1",
				Type:        models.ArtifactTypeExportPDF,
				Title:       "Management Brief",
				Status:      models.ArtifactStatusGenerating,
				ContentJSON: `{"format":"pdf","outline":[{"heading":"核心结论","bullets":["收入增长"]}]}`,
			},
		},
	}
	svc := &notebookExportService{repo: repo, outputRoot: t.TempDir()}

	if err := svc.RenderExport(context.Background(), "artifact-1"); err != nil {
		t.Fatalf("RenderExport returned error: %v", err)
	}
	artifact := repo.artifacts["artifact-1"]
	if artifact.Status != models.ArtifactStatusCompleted {
		t.Fatalf("expected completed, got %q", artifact.Status)
	}
	if !strings.HasSuffix(artifact.FileName, ".pdf") {
		t.Fatalf("expected pdf file, got %q", artifact.FileName)
	}
}
