package port

import (
	"context"

	"local/rag-project/internal/app/ingestion/domain"
)

// ListOptions 描述通用分页参数。
type ListOptions struct {
	Offset int
	Limit  int
}

// PipelineListFilter 描述 pipeline 列表查询条件。
type PipelineListFilter struct {
	Keyword string
	ListOptions
}

// TaskListFilter 描述 task 列表查询条件。
type TaskListFilter struct {
	PipelineID string
	Status     string
	ListOptions
}

// PipelineRepository 定义 pipeline 持久化能力。
type PipelineRepository interface {
	Create(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error)
	Update(ctx context.Context, pipeline domain.Pipeline) (domain.Pipeline, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.Pipeline, error)
	Count(ctx context.Context, filter PipelineListFilter) (int, error)
	List(ctx context.Context, filter PipelineListFilter) ([]domain.Pipeline, error)
}

// TaskRepository 定义 task 持久化能力。
type TaskRepository interface {
	Create(ctx context.Context, task domain.Task) (domain.Task, error)
	Update(ctx context.Context, task domain.Task) (domain.Task, error)
	GetByID(ctx context.Context, id string) (domain.Task, error)
	Count(ctx context.Context, filter TaskListFilter) (int, error)
	List(ctx context.Context, filter TaskListFilter) ([]domain.Task, error)
}

// TaskNodeRepository 定义 task node 持久化能力。
type TaskNodeRepository interface {
	Create(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error)
	Update(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error)
	GetByTaskIDAndNodeID(ctx context.Context, taskID string, nodeID string) (domain.TaskNode, error)
	ListByTaskID(ctx context.Context, taskID string) ([]domain.TaskNode, error)
}
