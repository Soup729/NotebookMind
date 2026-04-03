package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TypeProcessDocument = "document:process"
)

type ProcessDocumentPayload struct {
	TaskID     string `json:"task_id"`
	UserID     string `json:"user_id"`
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	DocumentID string `json:"document_id"`
}

func NewProcessDocumentTask(payload ProcessDocumentPayload) (*asynq.Task, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal process document payload: %w", err)
	}

	return asynq.NewTask(TypeProcessDocument, b), nil
}
