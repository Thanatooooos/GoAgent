package service

import (
	"context"
	"testing"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/ingestion/port"
)

type taskServicePipelineRepoStub struct {
	pipeline domain.Pipeline
}

func (s *taskServicePipelineRepoStub) Create(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error) {
	return pipeline, nil
}

func (s *taskServicePipelineRepoStub) Update(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error) {
	return pipeline, nil
}

func (s *taskServicePipelineRepoStub) Delete(ctx context.Context, id string) error { return nil }

func (s *taskServicePipelineRepoStub) GetByID(ctx context.Context, id string) (domain.Pipeline, error) {
	if s.pipeline.ID == id {
		return s.pipeline, nil
	}
	return domain.Pipeline{}, nil
}

func (s *taskServicePipelineRepoStub) Count(ctx context.Context, filter port.PipelineListFilter) (int, error) {
	return 0, nil
}

func (s *taskServicePipelineRepoStub) List(ctx context.Context, filter port.PipelineListFilter) ([]domain.Pipeline, error) {
	return nil, nil
}

type taskServiceTaskRepoStub struct {
	created           []domain.Task
	activeDocumentIDs map[string]bool
}

func (s *taskServiceTaskRepoStub) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	s.created = append(s.created, task)
	return task, nil
}

func (s *taskServiceTaskRepoStub) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	return task, nil
}

func (s *taskServiceTaskRepoStub) GetByID(ctx context.Context, id string) (domain.Task, error) {
	return domain.Task{}, nil
}

func (s *taskServiceTaskRepoStub) Count(ctx context.Context, filter port.TaskListFilter) (int, error) {
	return 0, nil
}

func (s *taskServiceTaskRepoStub) List(ctx context.Context, filter port.TaskListFilter) ([]domain.Task, error) {
	return nil, nil
}

func (s *taskServiceTaskRepoStub) HasActiveTaskForDocument(ctx context.Context, documentID string) (bool, error) {
	return s.activeDocumentIDs[documentID], nil
}

type taskServiceTaskNodeRepoStub struct{}

func (s *taskServiceTaskNodeRepoStub) Create(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	return node, nil
}

func (s *taskServiceTaskNodeRepoStub) Update(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	return node, nil
}

func (s *taskServiceTaskNodeRepoStub) GetByTaskIDAndNodeID(ctx context.Context, taskID string, nodeID string) (domain.TaskNode, error) {
	return domain.TaskNode{}, nil
}

func (s *taskServiceTaskNodeRepoStub) ListByTaskID(ctx context.Context, taskID string) ([]domain.TaskNode, error) {
	return nil, nil
}

func TestTaskServiceCreateRejectsActiveDocumentTask(t *testing.T) {
	t.Parallel()

	taskRepo := &taskServiceTaskRepoStub{
		activeDocumentIDs: map[string]bool{"doc-1": true},
	}
	svc := NewTaskService(
		&taskServicePipelineRepoStub{
			pipeline: domain.Pipeline{ID: "pipe-1", Name: "demo"},
		},
		taskRepo,
		&taskServiceTaskNodeRepoStub{},
		nil,
	)

	_, err := svc.Create(context.Background(), CreateTaskInput{
		PipelineID:     "pipe-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/demo.md",
		Metadata: map[string]any{
			"documentId": "doc-1",
		},
		CreatedBy: "alice",
	})
	if err == nil {
		t.Fatal("expected active document task error, got nil")
	}
	if len(taskRepo.created) != 0 {
		t.Fatalf("expected task not to be created, got %d", len(taskRepo.created))
	}
}

func TestTaskServiceCreateAllowsNewDocumentTask(t *testing.T) {
	t.Parallel()

	taskRepo := &taskServiceTaskRepoStub{
		activeDocumentIDs: map[string]bool{},
	}
	svc := NewTaskService(
		&taskServicePipelineRepoStub{
			pipeline: domain.Pipeline{ID: "pipe-1", Name: "demo"},
		},
		taskRepo,
		&taskServiceTaskNodeRepoStub{},
		nil,
	)

	item, err := svc.Create(context.Background(), CreateTaskInput{
		ID:             "task-1",
		PipelineID:     "pipe-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/demo.md",
		Metadata: map[string]any{
			"documentId": "doc-1",
		},
		CreatedBy: "alice",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if item.ID != "task-1" {
		t.Fatalf("unexpected task id: %q", item.ID)
	}
	if len(taskRepo.created) != 1 {
		t.Fatalf("expected one task created, got %d", len(taskRepo.created))
	}
}
