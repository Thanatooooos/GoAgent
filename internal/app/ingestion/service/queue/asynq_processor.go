package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// AsynqProcessor adapts the durable queue payload to the ingestion worker.
type AsynqProcessor struct {
	worker *Worker
}

func NewAsynqProcessor(worker *Worker) *AsynqProcessor {
	return &AsynqProcessor{worker: worker}
}

func (p *AsynqProcessor) ProcessTask(ctx context.Context, task *asynq.Task) error {
	if p == nil || p.worker == nil {
		return fmt.Errorf("asynq ingestion processor worker is required")
	}
	if task == nil || task.Type() != ingestionTaskType {
		return fmt.Errorf("unsupported ingestion queue task")
	}
	var payload ingestionTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode ingestion queue payload: %w", err)
	}
	return p.worker.Handle(ctx, payload.TaskID)
}

func (p *AsynqProcessor) Register(mux *asynq.ServeMux) error {
	if p == nil || mux == nil {
		return fmt.Errorf("asynq ingestion processor and mux are required")
	}
	mux.Handle(ingestionTaskType, p)
	return nil
}
