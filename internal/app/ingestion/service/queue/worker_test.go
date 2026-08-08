package queue

import (
	"context"
	"testing"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

type taskRepoStub struct{ task domain.Task }

func (s taskRepoStub) Create(context.Context, domain.Task) (domain.Task, error) {
	return domain.Task{}, nil
}
func (s taskRepoStub) Update(context.Context, domain.Task) (domain.Task, error) {
	return domain.Task{}, nil
}
func (s taskRepoStub) GetByID(context.Context, string) (domain.Task, error)    { return s.task, nil }
func (s taskRepoStub) Count(context.Context, port.TaskListFilter) (int, error) { return 0, nil }
func (s taskRepoStub) List(context.Context, port.TaskListFilter) ([]domain.Task, error) {
	return nil, nil
}

type pipelineRepoStub struct{ pipeline domain.Pipeline }

func (s pipelineRepoStub) Create(context.Context, domain.Pipeline) (domain.Pipeline, error) {
	return domain.Pipeline{}, nil
}
func (s pipelineRepoStub) Update(context.Context, domain.Pipeline) (domain.Pipeline, error) {
	return domain.Pipeline{}, nil
}
func (s pipelineRepoStub) Delete(context.Context, string) error { return nil }
func (s pipelineRepoStub) GetByID(context.Context, string) (domain.Pipeline, error) {
	return s.pipeline, nil
}
func (s pipelineRepoStub) Count(context.Context, port.PipelineListFilter) (int, error) { return 0, nil }
func (s pipelineRepoStub) List(context.Context, port.PipelineListFilter) ([]domain.Pipeline, error) {
	return nil, nil
}

type runnerStub struct{ calls int }

func (s *runnerStub) Execute(context.Context, domain.Pipeline, domain.Task) error {
	s.calls++
	return nil
}

func TestWorkerRunsPersistedPendingTask(t *testing.T) {
	runner := &runnerStub{}
	worker := NewWorker(
		taskRepoStub{task: domain.Task{ID: "task-1", PipelineID: "pipe-1", Status: domain.TaskStatusPending}},
		pipelineRepoStub{pipeline: domain.Pipeline{ID: "pipe-1"}},
		runner,
	)

	if err := worker.Handle(context.Background(), "task-1"); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestWorkerSkipsCompletedTask(t *testing.T) {
	runner := &runnerStub{}
	worker := NewWorker(
		taskRepoStub{task: domain.Task{ID: "task-1", PipelineID: "pipe-1", Status: domain.TaskStatusSuccess}},
		pipelineRepoStub{pipeline: domain.Pipeline{ID: "pipe-1"}},
		runner,
	)

	if err := worker.Handle(context.Background(), "task-1"); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}
