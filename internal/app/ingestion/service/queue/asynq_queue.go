package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
)

const ingestionTaskType = "ingestion:execute"

type ingestionTaskPayload struct {
	TaskID string `json:"task_id"`
}

type asynqEnqueuer interface {
	EnqueueContext(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error)
}

// AsynqTaskQueue persists ingestion task delivery in Redis.
type AsynqTaskQueue struct {
	client     asynqEnqueuer
	queueName  string
	maxRetries int
}

func newAsynqTaskQueue(client asynqEnqueuer, queueName string, maxRetries int) *AsynqTaskQueue {
	if strings.TrimSpace(queueName) == "" {
		queueName = "ingestion"
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &AsynqTaskQueue{client: client, queueName: queueName, maxRetries: maxRetries}
}

// NewAsynqTaskQueue creates a Redis-backed durable task queue.
func NewAsynqTaskQueue(redis asynq.RedisClientOpt, queueName string, maxRetries int) *AsynqTaskQueue {
	return newAsynqTaskQueue(asynq.NewClient(redis), queueName, maxRetries)
}

func (q *AsynqTaskQueue) Enqueue(ctx context.Context, taskID string) error {
	if q == nil || q.client == nil {
		return fmt.Errorf("asynq ingestion queue client is required")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("ingestion task id is required")
	}
	payload, err := json.Marshal(ingestionTaskPayload{TaskID: taskID})
	if err != nil {
		return fmt.Errorf("marshal ingestion queue payload: %w", err)
	}
	_, err = q.client.EnqueueContext(ctx, asynq.NewTask(ingestionTaskType, payload),
		asynq.Queue(q.queueName),
		asynq.MaxRetry(q.maxRetries),
		asynq.TaskID(taskID),
	)
	if err != nil {
		return fmt.Errorf("enqueue ingestion task %q: %w", taskID, err)
	}
	return nil
}
