package queue

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

// Runner executes one persisted ingestion task.
type Runner interface {
	Execute(ctx context.Context, pipeline domain.Pipeline, task domain.Task) error
}

// Worker reloads persisted task state before dispatching it from a durable queue.
type Worker struct {
	taskRepo     port.TaskRepository
	pipelineRepo port.PipelineRepository
	runner       Runner
}

func NewWorker(taskRepo port.TaskRepository, pipelineRepo port.PipelineRepository, runner Runner) *Worker {
	return &Worker{taskRepo: taskRepo, pipelineRepo: pipelineRepo, runner: runner}
}

func (w *Worker) Handle(ctx context.Context, taskID string) error {
	if w == nil || w.taskRepo == nil || w.pipelineRepo == nil || w.runner == nil {
		return fmt.Errorf("ingestion queue worker dependencies are required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("ingestion queue task id is required")
	}
	task, err := w.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load queued ingestion task: %w", err)
	}
	if task.ID == "" || isCompleted(task.Status) {
		return nil
	}
	pipeline, err := w.pipelineRepo.GetByID(ctx, task.PipelineID)
	if err != nil {
		return fmt.Errorf("load queued ingestion pipeline: %w", err)
	}
	if pipeline.ID == "" {
		return fmt.Errorf("queued ingestion pipeline %q not found", task.PipelineID)
	}
	return w.runner.Execute(ctx, pipeline, task)
}

func isCompleted(status string) bool {
	switch strings.TrimSpace(status) {
	case domain.TaskStatusSuccess:
		return true
	default:
		return false
	}
}
