package worker

import (
	"context"
	"fmt"

	"NotebookAI/internal/configs"
	"NotebookAI/internal/worker/tasks"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

type TaskProducer interface {
	EnqueueProcessDocument(ctx context.Context, payload tasks.ProcessDocumentPayload) (string, error)
	EnqueueRenderNotebookExport(ctx context.Context, payload tasks.RenderNotebookExportPayload) (string, error)
	Close() error
}

type asynqProducer struct {
	client *asynq.Client
}

func NewTaskProducer(redisCfg *configs.RedisConfig) TaskProducer {
	return &asynqProducer{
		client: asynq.NewClient(asynq.RedisClientOpt{
			Addr:     redisCfg.Addr,
			Password: redisCfg.Password,
			DB:       redisCfg.DB,
		}),
	}
}

func (p *asynqProducer) EnqueueProcessDocument(ctx context.Context, payload tasks.ProcessDocumentPayload) (string, error) {
	task, err := tasks.NewProcessDocumentTask(payload)
	if err != nil {
		return "", err
	}

	info, err := p.client.EnqueueContext(
		ctx,
		task,
		asynq.Queue("documents"),
		asynq.MaxRetry(5),
	)
	if err != nil {
		return "", fmt.Errorf("enqueue process document task: %w", err)
	}

	zap.L().Info("document task enqueued",
		zap.String("task_id", info.ID),
		zap.String("queue", info.Queue),
		zap.String("file_path", payload.FilePath),
		zap.String("user_id", payload.UserID),
	)

	return info.ID, nil
}

func (p *asynqProducer) EnqueueRenderNotebookExport(ctx context.Context, payload tasks.RenderNotebookExportPayload) (string, error) {
	task, err := tasks.NewRenderNotebookExportTask(payload)
	if err != nil {
		return "", err
	}

	info, err := p.client.EnqueueContext(
		ctx,
		task,
		asynq.Queue("exports"),
		asynq.MaxRetry(3),
	)
	if err != nil {
		return "", fmt.Errorf("enqueue render notebook export task: %w", err)
	}

	zap.L().Info("notebook export task enqueued",
		zap.String("task_id", info.ID),
		zap.String("queue", info.Queue),
		zap.String("user_id", payload.UserID),
		zap.String("notebook_id", payload.NotebookID),
		zap.String("artifact_id", payload.ArtifactID),
	)

	return info.ID, nil
}

func (p *asynqProducer) Close() error {
	return p.client.Close()
}
