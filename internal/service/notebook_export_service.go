package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"NotebookAI/internal/models"
	"NotebookAI/internal/repository"
	"NotebookAI/internal/worker/tasks"
	"github.com/google/uuid"
	"github.com/tmc/langchaingo/llms"
)

const defaultExportOutputRoot = "storage/exports"

type NotebookExportService interface {
	CreateOutline(ctx context.Context, userID, notebookID string, req ExportOutlineRequest) (*models.NotebookArtifact, error)
	ConfirmOutline(ctx context.Context, userID, notebookID, artifactID string, outline []ExportOutlineSection) (*models.NotebookArtifact, error)
	RenderExport(ctx context.Context, artifactID string) error
}

type ExportOutlineRequest struct {
	Format           string   `json:"format"`
	DocumentIDs      []string `json:"document_ids"`
	Language         string   `json:"language"`
	Style            string   `json:"style"`
	Length           string   `json:"length"`
	Requirements     string   `json:"requirements"`
	IncludeCitations bool     `json:"include_citations"`
}

type ExportOutlineSection struct {
	Heading    string              `json:"heading"`
	Bullets    []string            `json:"bullets"`
	SourceRefs []artifactSourceRef `json:"source_refs,omitempty"`
}

type exportContent struct {
	Format       string                 `json:"format"`
	Language     string                 `json:"language"`
	Style        string                 `json:"style"`
	Length       string                 `json:"length"`
	Requirements string                 `json:"requirements"`
	Outline      []ExportOutlineSection `json:"outline"`
	RenderedText string                 `json:"rendered_text,omitempty"`
}

type exportTaskProducer interface {
	EnqueueRenderNotebookExport(ctx context.Context, payload tasks.RenderNotebookExportPayload) (string, error)
}

type notebookExportService struct {
	repo       repository.NotebookRepository
	llm        llms.Model
	producer   exportTaskProducer
	outputRoot string
	generate   func(ctx context.Context, prompt string) (string, error)
}

func NewNotebookExportService(repo repository.NotebookRepository, llm llms.Model, producer exportTaskProducer) NotebookExportService {
	return &notebookExportService{
		repo:       repo,
		llm:        llm,
		producer:   producer,
		outputRoot: defaultExportOutputRoot,
		generate: func(ctx context.Context, prompt string) (string, error) {
			if llm == nil {
				return "", fmt.Errorf("llm is not configured")
			}
			return llms.GenerateFromSinglePrompt(ctx, llm, prompt)
		},
	}
}

func (s *notebookExportService) CreateOutline(ctx context.Context, userID, notebookID string, req ExportOutlineRequest) (*models.NotebookArtifact, error) {
	format, artifactType, err := normalizeExportFormat(req.Format)
	if err != nil {
		return nil, err
	}
	req = normalizeExportRequest(req, format)
	notebook, err := s.repo.GetByID(ctx, userID, notebookID)
	if err != nil {
		return nil, err
	}
	documents, err := s.repo.ListDocuments(ctx, notebookID)
	if err != nil {
		return nil, err
	}
	selected := selectExportDocuments(documents, req.DocumentIDs)
	if len(selected) == 0 {
		return nil, fmt.Errorf("notebook has no completed documents for export")
	}

	content := exportContent{}
	raw, genErr := s.generate(ctx, buildExportOutlinePrompt(notebook, selected, req))
	if genErr == nil {
		content, err = parseExportContent(raw)
	}
	if genErr != nil || err != nil {
		content = fallbackExportContent(req, selected)
	}
	content.Format = format
	content.Language = req.Language
	content.Style = req.Style
	content.Length = req.Length
	content.Requirements = req.Requirements
	if len(content.Outline) == 0 {
		content = fallbackExportContent(req, selected)
	}
	refs := collectExportSourceRefs(content.Outline, selected)
	contentJSON, _ := json.Marshal(content)
	sourceRefsJSON, _ := json.Marshal(refs)
	requestJSON, _ := json.Marshal(req)
	artifact := &models.NotebookArtifact{
		ID:             uuid.NewString(),
		NotebookID:     notebookID,
		UserID:         userID,
		Type:           artifactType,
		Title:          exportTitle(notebook.Title, req.Requirements),
		ContentJSON:    string(contentJSON),
		SourceRefsJSON: string(sourceRefsJSON),
		RequestJSON:    string(requestJSON),
		Status:         models.ArtifactStatusOutlineReady,
		Version:        1,
	}
	if err := s.repo.UpsertArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func (s *notebookExportService) ConfirmOutline(ctx context.Context, userID, notebookID, artifactID string, outline []ExportOutlineSection) (*models.NotebookArtifact, error) {
	if err := validateExportOutline(outline); err != nil {
		return nil, err
	}
	artifact, err := s.repo.GetArtifact(ctx, userID, notebookID, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact.Status != models.ArtifactStatusOutlineReady && artifact.Status != models.ArtifactStatusFailed {
		return nil, fmt.Errorf("artifact is not ready for export confirmation")
	}
	if s.producer == nil {
		return nil, fmt.Errorf("export task producer is not configured")
	}
	content, err := parseStoredExportContent(artifact.ContentJSON)
	if err != nil {
		return nil, err
	}
	content.Outline = outline
	content.RenderedText = ""
	contentJSON, _ := json.Marshal(content)
	artifact.ContentJSON = string(contentJSON)
	artifact.Status = models.ArtifactStatusGenerating
	artifact.ErrorMsg = ""
	payload := tasks.RenderNotebookExportPayload{
		UserID:     userID,
		NotebookID: notebookID,
		ArtifactID: artifactID,
	}
	taskID, err := s.producer.EnqueueRenderNotebookExport(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("enqueue export render task: %w", err)
	}
	artifact.TaskID = taskID
	if err := s.repo.UpsertArtifact(ctx, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func (s *notebookExportService) RenderExport(ctx context.Context, artifactID string) error {
	artifact, err := s.repo.GetArtifactByID(ctx, artifactID)
	if err != nil {
		return err
	}
	if artifact.Status != models.ArtifactStatusGenerating {
		return fmt.Errorf("artifact is not generating")
	}
	content, err := parseStoredExportContent(artifact.ContentJSON)
	if err != nil {
		return s.failRender(ctx, artifact, err)
	}
	refs, err := parseArtifactSourceRefs(artifact.SourceRefsJSON)
	if err != nil {
		return s.failRender(ctx, artifact, err)
	}
	var rendered, ext, mimeType string
	switch artifact.Type {
	case models.ArtifactTypeExportMarkdown:
		rendered = renderMarkdownExport(artifact.Title, content, refs)
		ext = ".md"
		mimeType = "text/markdown; charset=utf-8"
	case models.ArtifactTypeExportMindmap:
		rendered = renderMindmapExport(artifact.Title, content)
		ext = ".mindmap.md"
		mimeType = "text/markdown; charset=utf-8"
	case models.ArtifactTypeExportDocx:
		ext = ".docx"
		mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case models.ArtifactTypeExportPptx:
		ext = ".pptx"
		mimeType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case models.ArtifactTypeExportPDF:
		ext = ".pdf"
		mimeType = "application/pdf"
	default:
		return s.failRender(ctx, artifact, fmt.Errorf("unsupported export artifact type: %s", artifact.Type))
	}
	outputRoot := s.outputRoot
	if outputRoot == "" {
		outputRoot = defaultExportOutputRoot
	}
	dir := filepath.Join(outputRoot, artifact.UserID, artifact.NotebookID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return s.failRender(ctx, artifact, fmt.Errorf("create export directory: %w", err))
	}
	fileName := artifact.ID + ext
	filePath := filepath.Join(dir, fileName)
	switch artifact.Type {
	case models.ArtifactTypeExportMarkdown, models.ArtifactTypeExportMindmap:
		content.RenderedText = rendered
		if err := os.WriteFile(filePath, []byte(rendered), 0644); err != nil {
			return s.failRender(ctx, artifact, fmt.Errorf("write export file: %w", err))
		}
	case models.ArtifactTypeExportDocx, models.ArtifactTypeExportPptx, models.ArtifactTypeExportPDF:
		if err := s.renderViaPythonScript(ctx, artifact.Title, content, refs, filePath); err != nil {
			return s.failRender(ctx, artifact, err)
		}
	}
	contentJSON, _ := json.Marshal(content)
	now := time.Now()
	artifact.ContentJSON = string(contentJSON)
	artifact.FilePath = filePath
	artifact.FileName = fileName
	artifact.MimeType = mimeType
	artifact.Status = models.ArtifactStatusCompleted
	artifact.ErrorMsg = ""
	artifact.GeneratedAt = &now
	if err := s.repo.UpsertArtifact(ctx, artifact); err != nil {
		return err
	}
	return nil
}

func (s *notebookExportService) failRender(ctx context.Context, artifact *models.NotebookArtifact, renderErr error) error {
	artifact.Status = models.ArtifactStatusFailed
	artifact.ErrorMsg = renderErr.Error()
	_ = s.repo.UpsertArtifact(ctx, artifact)
	return renderErr
}

func normalizeExportFormat(format string) (string, string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "markdown", "md":
		return "markdown", models.ArtifactTypeExportMarkdown, nil
	case "mindmap", "mind_map", "mind-map", "map":
		return "mindmap", models.ArtifactTypeExportMindmap, nil
	case "docx", "word":
		return "docx", models.ArtifactTypeExportDocx, nil
	case "pptx", "ppt", "powerpoint":
		return "pptx", models.ArtifactTypeExportPptx, nil
	case "pdf":
		return "pdf", models.ArtifactTypeExportPDF, nil
	default:
		return "", "", fmt.Errorf("unsupported export format: %s", format)
	}
}

func normalizeExportRequest(req ExportOutlineRequest, format string) ExportOutlineRequest {
	req.Format = format
	if strings.TrimSpace(req.Language) == "" {
		req.Language = "zh-CN"
	}
	if strings.TrimSpace(req.Style) == "" {
		req.Style = "professional"
	}
	if strings.TrimSpace(req.Length) == "" {
		req.Length = "medium"
	}
	return req
}

func selectExportDocuments(documents []models.Document, ids []string) []models.Document {
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	out := make([]models.Document, 0, len(documents))
	for _, doc := range documents {
		if doc.Status != models.DocumentStatusCompleted {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[doc.ID]; !ok {
				continue
			}
		}
		out = append(out, doc)
	}
	return out
}

func buildExportOutlinePrompt(notebook *models.Notebook, documents []models.Document, req ExportOutlineRequest) string {
	var b strings.Builder
	b.WriteString("Return strict JSON only with fields: format, language, style, length, requirements, outline.\n")
	b.WriteString("Each outline item must have heading, bullets, source_refs. Do not invent facts outside document summaries.\n")
	b.WriteString("Format: " + req.Format + "\n")
	b.WriteString("Language: " + req.Language + "\n")
	b.WriteString("Style: " + req.Style + "\n")
	b.WriteString("Length: " + req.Length + "\n")
	b.WriteString("Requirements: " + req.Requirements + "\n")
	b.WriteString("Notebook: " + notebook.Title + "\n\nDocuments:\n")
	for _, doc := range documents {
		b.WriteString("- document_id: " + doc.ID + "\n")
		b.WriteString("  document_name: " + doc.FileName + "\n")
		b.WriteString("  summary: " + exportDocumentSummary(doc) + "\n")
	}
	return b.String()
}

func parseExportContent(raw string) (exportContent, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	var content exportContent
	if err := json.Unmarshal([]byte(cleaned), &content); err != nil {
		return exportContent{}, fmt.Errorf("parse export JSON: %w", err)
	}
	return content, nil
}

func parseStoredExportContent(raw string) (exportContent, error) {
	var content exportContent
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return exportContent{}, fmt.Errorf("parse stored export content: %w", err)
	}
	return content, nil
}

func fallbackExportContent(req ExportOutlineRequest, documents []models.Document) exportContent {
	return exportContent{
		Format:       req.Format,
		Language:     req.Language,
		Style:        req.Style,
		Length:       req.Length,
		Requirements: req.Requirements,
		Outline: []ExportOutlineSection{
			{
				Heading:    "概览",
				Bullets:    bulletsFromDocumentSummaries(documents),
				SourceRefs: fallbackArtifactSourceRefs(documents),
			},
		},
	}
}

func bulletsFromDocumentSummaries(documents []models.Document) []string {
	bullets := make([]string, 0, len(documents))
	for _, doc := range documents {
		summary := exportDocumentSummary(doc)
		if len([]rune(summary)) > 180 {
			summary = string([]rune(summary)[:180])
		}
		bullets = append(bullets, fmt.Sprintf("%s: %s", doc.FileName, summary))
	}
	return bullets
}

func exportDocumentSummary(doc models.Document) string {
	summary := strings.TrimSpace(doc.Summary)
	if summary == "" {
		summary = strings.TrimSpace(doc.FileName)
	}
	return summary
}

func collectExportSourceRefs(outline []ExportOutlineSection, documents []models.Document) []artifactSourceRef {
	refs := make([]artifactSourceRef, 0)
	seen := map[string]struct{}{}
	for _, section := range outline {
		for _, ref := range section.SourceRefs {
			key := ref.DocumentID + "|" + ref.DocumentName
			if key == "|" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return fallbackArtifactSourceRefs(documents)
	}
	return refs
}

func validateExportOutline(outline []ExportOutlineSection) error {
	if len(outline) == 0 {
		return fmt.Errorf("outline must contain at least one section")
	}
	if len(outline) > 30 {
		return fmt.Errorf("outline cannot contain more than 30 sections")
	}
	for _, section := range outline {
		heading := strings.TrimSpace(section.Heading)
		if heading == "" {
			return fmt.Errorf("outline section heading is required")
		}
		if len([]rune(heading)) > 160 {
			return fmt.Errorf("outline section heading is too long")
		}
		if len(section.Bullets) == 0 {
			return fmt.Errorf("outline section must contain at least one bullet")
		}
		for _, bullet := range section.Bullets {
			bullet = strings.TrimSpace(bullet)
			if bullet == "" {
				return fmt.Errorf("outline bullet is required")
			}
			if len([]rune(bullet)) > 500 {
				return fmt.Errorf("outline bullet is too long")
			}
		}
	}
	return nil
}

func parseArtifactSourceRefs(raw string) ([]artifactSourceRef, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var refs []artifactSourceRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil, fmt.Errorf("parse artifact source refs: %w", err)
	}
	return refs, nil
}

func renderMarkdownExport(title string, content exportContent, refs []artifactSourceRef) string {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	for _, section := range content.Outline {
		b.WriteString("## " + section.Heading + "\n\n")
		for _, bullet := range section.Bullets {
			b.WriteString("- " + bullet + "\n")
		}
		b.WriteString("\n")
	}
	if len(refs) > 0 {
		b.WriteString("## Sources\n\n")
		for i, ref := range refs {
			b.WriteString(fmt.Sprintf("%d. %s", i+1, ref.DocumentName))
			if ref.Page > 0 {
				b.WriteString(fmt.Sprintf(", Page %d", ref.Page))
			}
			if ref.Quote != "" {
				b.WriteString(": " + ref.Quote)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderMindmapExport(title string, content exportContent) string {
	var b strings.Builder
	b.WriteString("```mermaid\nmindmap\n")
	b.WriteString("  root((" + sanitizeMindmapText(title) + "))\n")
	for _, section := range content.Outline {
		b.WriteString("    " + sanitizeMindmapText(section.Heading) + "\n")
		for _, bullet := range section.Bullets {
			b.WriteString("      " + sanitizeMindmapText(bullet) + "\n")
		}
	}
	b.WriteString("```\n")
	return b.String()
}

func (s *notebookExportService) renderViaPythonScript(ctx context.Context, title string, content exportContent, refs []artifactSourceRef, outputPath string) error {
	payload := map[string]any{
		"title":        title,
		"format":       content.Format,
		"language":     content.Language,
		"style":        content.Style,
		"length":       content.Length,
		"requirements": content.Requirements,
		"outline":      content.Outline,
		"source_refs":  refs,
	}
	tmpInput, err := os.CreateTemp("", "notebook_export_*.json")
	if err != nil {
		return fmt.Errorf("create export temp input: %w", err)
	}
	tmpInputPath := tmpInput.Name()
	_ = tmpInput.Close()
	defer os.Remove(tmpInputPath)
	body, _ := json.Marshal(payload)
	if err := os.WriteFile(tmpInputPath, body, 0644); err != nil {
		return fmt.Errorf("write export temp input: %w", err)
	}
	scriptPath, err := resolveExportRendererScriptPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "python", scriptPath, "--input", tmpInputPath, "--output", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("render export via python: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resolveExportRendererScriptPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "scripts", "render_export.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("resolve export renderer script path: scripts/render_export.py not found from %s", wd)
}

var mindmapUnsafeChars = regexp.MustCompile(`[\r\n{}[\]()]`)

func sanitizeMindmapText(text string) string {
	text = strings.TrimSpace(text)
	text = mindmapUnsafeChars.ReplaceAllString(text, " ")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "Untitled"
	}
	return text
}

func exportTitle(notebookTitle, requirements string) string {
	requirements = strings.TrimSpace(requirements)
	if requirements != "" {
		runes := []rune(requirements)
		if len(runes) > 60 {
			requirements = string(runes[:60])
		}
		return requirements
	}
	notebookTitle = strings.TrimSpace(notebookTitle)
	if notebookTitle != "" {
		return notebookTitle + " Export"
	}
	return "Notebook Export"
}
