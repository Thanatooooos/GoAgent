package service

import (
	"context"
	"strings"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

// WorkflowBuilder 定义 pipeline 到编排工作流的转换边界。
type WorkflowBuilder interface {
	Build(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (WorkflowSpec, error)
}

// LinearWorkflowBuilder 提供第一阶段线性 pipeline 构建实现。
type LinearWorkflowBuilder struct{}

// NewLinearWorkflowBuilder 创建线性 workflow 构建器。
func NewLinearWorkflowBuilder() *LinearWorkflowBuilder {
	return &LinearWorkflowBuilder{}
}

// Build 把 pipeline 节点序列转换为最小可执行的线性工作流。
func (b *LinearWorkflowBuilder) Build(ctx context.Context, pipeline domain.Pipeline, task domain.Task) (WorkflowSpec, error) {
	_ = ctx

	if strings.TrimSpace(task.ID) == "" {
		return WorkflowSpec{}, exception.NewClientException("task id is required", nil)
	}
	if strings.TrimSpace(pipeline.ID) == "" {
		return WorkflowSpec{}, exception.NewClientException("pipeline id is required", nil)
	}
	if len(pipeline.Nodes) == 0 {
		return WorkflowSpec{}, exception.NewClientException("pipeline nodes are required", nil)
	}

	items := make([]WorkflowNodeSpec, 0, len(pipeline.Nodes))
	for index, node := range pipeline.Nodes {
		items = append(items, WorkflowNodeSpec{
			Order: index + 1,
			Node:  node,
		})
	}

	return WorkflowSpec{
		TaskID:    task.ID,
		Pipeline:  pipeline,
		NodeOrder: items,
	}, nil
}
