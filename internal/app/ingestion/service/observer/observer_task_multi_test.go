package observer

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

type multiObserverRecorder struct {
	taskStarted   bool
	taskCompleted bool
	nodeStarted   bool
	nodeRetried   bool
	nodeCompleted bool
	err           error
}

func (o *multiObserverRecorder) OnTaskStarted(ctx context.Context, task domain.Task) error {
	o.taskStarted = true
	return o.err
}

func (o *multiObserverRecorder) OnTaskCompleted(ctx context.Context, task domain.Task, state ingestionworkflow.ExecutionState, execErr error) error {
	o.taskCompleted = true
	return o.err
}

func (o *multiObserverRecorder) OnNodeStarted(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec) error {
	o.nodeStarted = true
	return o.err
}

func (o *multiObserverRecorder) OnNodeRetry(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	o.nodeRetried = true
	return o.err
}

func (o *multiObserverRecorder) OnNodeCompleted(ctx context.Context, task domain.Task, node ingestionworkflow.WorkflowNodeSpec, output map[string]any, duration time.Duration, execErr error) error {
	o.nodeCompleted = true
	return o.err
}

func TestMultiTaskObserverContinuesAfterObserverError(t *testing.T) {
	t.Parallel()

	first := &multiObserverRecorder{err: errors.New("observer one failed")}
	second := &multiObserverRecorder{}
	observer := NewMultiTaskObserver(first, second)

	task := domain.Task{ID: "task-1"}
	node := ingestionworkflow.WorkflowNodeSpec{Node: domain.PipelineNode{NodeID: "node-1", NodeType: "fetcher"}}

	err := observer.OnTaskStarted(context.Background(), task)
	if err == nil {
		t.Fatal("expected aggregated observer error")
	}
	if !first.taskStarted || !second.taskStarted {
		t.Fatal("expected all observers to receive task started event")
	}

	err = observer.OnNodeStarted(context.Background(), task, node)
	if err == nil {
		t.Fatal("expected aggregated node started error")
	}
	if !first.nodeStarted || !second.nodeStarted {
		t.Fatal("expected all observers to receive node started event")
	}

	err = observer.OnNodeRetry(context.Background(), task, node, 1, time.Second, errors.New("retry"))
	if err == nil {
		t.Fatal("expected aggregated node retry error")
	}
	if !first.nodeRetried || !second.nodeRetried {
		t.Fatal("expected all observers to receive node retry event")
	}

	err = observer.OnNodeCompleted(context.Background(), task, node, map[string]any{"ok": true}, time.Second, nil)
	if err == nil {
		t.Fatal("expected aggregated node completed error")
	}
	if !first.nodeCompleted || !second.nodeCompleted {
		t.Fatal("expected all observers to receive node completed event")
	}

	err = observer.OnTaskCompleted(context.Background(), task, ingestionworkflow.ExecutionState{}, nil)
	if err == nil {
		t.Fatal("expected aggregated task completed error")
	}
	if !first.taskCompleted || !second.taskCompleted {
		t.Fatal("expected all observers to receive task completed event")
	}
}
