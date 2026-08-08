package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hibiken/asynq"

	"local/rag-project/internal/app/ingestion/domain"
)

type asynqClientStub struct {
	task    *asynq.Task
	options []asynq.Option
}

func (c *asynqClientStub) EnqueueContext(_ context.Context, task *asynq.Task, options ...asynq.Option) (*asynq.TaskInfo, error) {
	c.task = task
	c.options = options
	return &asynq.TaskInfo{}, nil
}

func TestAsynqTaskQueueEnqueueUsesPersistedTaskID(t *testing.T) {
	client := &asynqClientStub{}
	queue := newAsynqTaskQueue(client, "ingestion", 2)

	if err := queue.Enqueue(context.Background(), "task-1"); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if client.task == nil || client.task.Type() != ingestionTaskType {
		t.Fatalf("task type = %v, want %q", client.task, ingestionTaskType)
	}

	var payload ingestionTaskPayload
	if err := json.Unmarshal(client.task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TaskID != "task-1" {
		t.Fatalf("payload task ID = %q, want task-1", payload.TaskID)
	}
	if !hasAsynqOption(client.options, "Queue(\"ingestion\")") || !hasAsynqOption(client.options, "MaxRetry(2)") || !hasAsynqOption(client.options, "TaskID(\"task-1\")") {
		t.Fatalf("queue options = %#v", client.options)
	}
}

func TestAsynqProcessorDispatchesPersistedTask(t *testing.T) {
	runner := &runnerStub{}
	processor := NewAsynqProcessor(NewWorker(
		taskRepoStub{task: domain.Task{ID: "task-1", PipelineID: "pipe-1", Status: domain.TaskStatusPending}},
		pipelineRepoStub{pipeline: domain.Pipeline{ID: "pipe-1"}},
		runner,
	))
	payload, err := json.Marshal(ingestionTaskPayload{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := processor.ProcessTask(context.Background(), asynq.NewTask(ingestionTaskType, payload)); err != nil {
		t.Fatalf("ProcessTask() error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func hasAsynqOption(options []asynq.Option, want string) bool {
	for _, option := range options {
		if option.String() == want {
			return true
		}
	}
	return false
}
