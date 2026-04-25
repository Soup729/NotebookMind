package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TypeProcessDocument      = "document:process"
	TypeRenderNotebookExport = "notebook_export:render"
)

type ProcessDocumentPayload struct {
	TaskID     string `json:"task_id"`
	UserID     string `json:"user_id"`
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	DocumentID string `json:"document_id"`
}

type RenderNotebookExportPayload struct {
	TaskID     string `json:"task_id"`
	UserID     string `json:"user_id"`
	NotebookID string `json:"notebook_id"`
	ArtifactID string `json:"artifact_id"`
}

func NewProcessDocumentTask(payload ProcessDocumentPayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal process document payload: %w", err)
	}

	return asynq.NewTask(TypeProcessDocument, b), nil
}

func NewRenderNotebookExportTask(payload RenderNotebookExportPayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal render notebook export payload: %w", err)
	}

	return asynq.NewTask(TypeRenderNotebookExport, b), nil
}
