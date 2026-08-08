package queue

import (
	"context"
	"testing"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

type pendingTaskRepoStub struct {
	tasks   []domain.Task
	filters []port.TaskListFilter
}

func (s *pendingTaskRepoStub) Create(context.Context, domain.Task) (domain.Task, error) {
	return domain.Task{}, nil
}
func (s *pendingTaskRepoStub) Update(context.Context, domain.Task) (domain.Task, error) {
	return domain.Task{}, nil
}
func (s *pendingTaskRepoStub) GetByID(context.Context, string) (domain.Task, error) {
	return domain.Task{}, nil
}
func (s *pendingTaskRepoStub) Count(context.Context, port.TaskListFilter) (int, error) { return 0, nil }
func (s *pendingTaskRepoStub) List(_ context.Context, filter port.TaskListFilter) ([]domain.Task, error) {
	s.filters = append(s.filters, filter)
	start, end := filter.Offset, filter.Offset+filter.Limit
	if start >= len(s.tasks) {
		return nil, nil
	}
	if end > len(s.tasks) {
		end = len(s.tasks)
	}
	return s.tasks[start:end], nil
}

type taskQueueStub struct{ ids []string }

func (s *taskQueueStub) Enqueue(_ context.Context, taskID string) error {
	s.ids = append(s.ids, taskID)
	return nil
}

func TestRecoverPendingEnqueuesAllPersistedPendingTasks(t *testing.T) {
	repo := &pendingTaskRepoStub{tasks: []domain.Task{
		{ID: "task-1", Status: domain.TaskStatusPending},
		{ID: "task-2", Status: domain.TaskStatusPending},
	}}
	queue := &taskQueueStub{}

	if err := RecoverPending(context.Background(), repo, queue, 1); err != nil {
		t.Fatalf("RecoverPending() error = %v", err)
	}
	if len(queue.ids) != 2 || queue.ids[0] != "task-1" || queue.ids[1] != "task-2" {
		t.Fatalf("enqueued ids = %#v", queue.ids)
	}
}
