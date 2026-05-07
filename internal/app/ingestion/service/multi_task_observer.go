package service

import (
	"context"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

// MultiTaskObserver 将多个 observer 组合为一个统一 observer。
type MultiTaskObserver struct {
	observers []TaskObserver
}

// NewMultiTaskObserver 创建组合 observer。
func NewMultiTaskObserver(observers ...TaskObserver) *MultiTaskObserver {
	filtered := make([]TaskObserver, 0, len(observers))
	for _, observer := range observers {
		if observer != nil {
			filtered = append(filtered, observer)
		}
	}
	return &MultiTaskObserver{observers: filtered}
}

// OnTaskStarted 广播任务开始事件。
func (o *MultiTaskObserver) OnTaskStarted(ctx context.Context, task domain.Task) error {
	for _, observer := range o.observers {
		if err := observer.OnTaskStarted(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

// OnTaskCompleted 广播任务完成事件。
func (o *MultiTaskObserver) OnTaskCompleted(ctx context.Context, task domain.Task, state ExecutionState, execErr error) error {
	for _, observer := range o.observers {
		if err := observer.OnTaskCompleted(ctx, task, state, execErr); err != nil {
			return err
		}
	}
	return nil
}

// OnNodeStarted 广播节点开始事件。
func (o *MultiTaskObserver) OnNodeStarted(ctx context.Context, task domain.Task, node WorkflowNodeSpec) error {
	for _, observer := range o.observers {
		if err := observer.OnNodeStarted(ctx, task, node); err != nil {
			return err
		}
	}
	return nil
}

// OnNodeRetry 广播节点重试事件。
func (o *MultiTaskObserver) OnNodeRetry(ctx context.Context, task domain.Task, node WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	for _, observer := range o.observers {
		if err := observer.OnNodeRetry(ctx, task, node, attempt, backoff, execErr); err != nil {
			return err
		}
	}
	return nil
}

// OnNodeCompleted 广播节点完成事件。
func (o *MultiTaskObserver) OnNodeCompleted(
	ctx context.Context,
	task domain.Task,
	node WorkflowNodeSpec,
	output map[string]any,
	duration time.Duration,
	execErr error,
) error {
	for _, observer := range o.observers {
		if err := observer.OnNodeCompleted(ctx, task, node, output, duration, execErr); err != nil {
			return err
		}
	}
	return nil
}
