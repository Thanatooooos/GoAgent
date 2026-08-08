package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

// RecoverPending closes the small database-to-queue handoff gap by re-enqueueing
// pending tasks left behind by an interrupted API process. Existing Asynq task IDs
// are treated as already recovered.
func RecoverPending(ctx context.Context, repo port.TaskRepository, queue port.TaskQueue, batchSize int) error {
	if repo == nil || queue == nil {
		return fmt.Errorf("pending ingestion recovery dependencies are required")
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	for offset := 0; ; offset += batchSize {
		tasks, err := repo.List(ctx, port.TaskListFilter{
			Status: domain.TaskStatusPending,
			ListOptions: port.ListOptions{
				Offset: offset,
				Limit:  batchSize,
			},
		})
		if err != nil {
			return fmt.Errorf("list pending ingestion tasks: %w", err)
		}
		for _, task := range tasks {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" {
				continue
			}
			if err := queue.Enqueue(ctx, taskID); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
				return fmt.Errorf("recover pending ingestion task %q: %w", taskID, err)
			}
		}
		if len(tasks) < batchSize {
			return nil
		}
	}
}
