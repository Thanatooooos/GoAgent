package port

import (
	"context"

	"local/rag-project/internal/app/ingestion/domain"
)

// TaskExecutor 定义 task 提交到执行层的边界。
type TaskExecutor interface {
	Submit(ctx context.Context, pipeline domain.Pipeline, task domain.Task) error
}

// TaskQueue persists task delivery independently from the current process.
type TaskQueue interface {
	Enqueue(ctx context.Context, taskID string) error
}
