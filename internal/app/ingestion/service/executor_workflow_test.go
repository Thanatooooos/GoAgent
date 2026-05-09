package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
)

// retryTestRunner 可在指定失败次数后返回成功。
type retryTestRunner struct {
	nodeType    string
	mu          sync.Mutex
	failCount   int
	maxFails    int
	callHistory []time.Time
}

func (r *retryTestRunner) NodeType() string {
	return r.nodeType
}

func (r *retryTestRunner) Run(ctx context.Context, state ExecutionState, node domain.PipelineNode) (ExecutionState, map[string]any, error) {
	r.mu.Lock()
	r.failCount++
	count := r.failCount
	r.callHistory = append(r.callHistory, time.Now())
	r.mu.Unlock()

	if count <= r.maxFails {
		return state, nil, errors.New("transient error")
	}
	return state, map[string]any{"success": true}, nil
}

// retryTaskObserverStub 在 OnTaskCompleted 时发送信号，用于测试同步。
type retryTaskObserverStub struct {
	completed chan struct{}
}

func newRetryTaskObserverStub() *retryTaskObserverStub {
	return &retryTaskObserverStub{completed: make(chan struct{}, 1)}
}

func (o *retryTaskObserverStub) OnTaskStarted(ctx context.Context, task domain.Task) error {
	return nil
}

func (o *retryTaskObserverStub) OnTaskCompleted(ctx context.Context, task domain.Task, state ExecutionState, execErr error) error {
	select {
	case o.completed <- struct{}{}:
	default:
	}
	return nil
}

func (o *retryTaskObserverStub) OnNodeStarted(ctx context.Context, task domain.Task, node WorkflowNodeSpec) error {
	return nil
}

func (o *retryTaskObserverStub) OnNodeRetry(ctx context.Context, task domain.Task, node WorkflowNodeSpec, attempt int, backoff time.Duration, execErr error) error {
	return nil
}

func (o *retryTaskObserverStub) OnNodeCompleted(ctx context.Context, task domain.Task, node WorkflowNodeSpec, output map[string]any, duration time.Duration, execErr error) error {
	return nil
}

// 等待任务完成，带超时保护。
func waitForCompletion(observer *retryTaskObserverStub, timeout time.Duration) {
	select {
	case <-observer.completed:
	case <-time.After(timeout):
	}
}

func TestExecutorRetriesNodeOnTransientFailure(t *testing.T) {
	runner := &retryTestRunner{
		nodeType: "fetcher",
		maxFails: 2,
	}

	registry := NewNodeRunnerRegistry(runner)
	observer := newRetryTaskObserverStub()

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: NewLinearWorkflowBuilder(),
		NodeRunners:     registry,
		TaskObserver:    observer,
		MaxConcurrent:   1,
		MaxRetries:      2,
		RetryBackoffMs:  1,
	})

	pipeline := domain.Pipeline{
		ID:   "p-1",
		Name: "test",
		Nodes: []domain.PipelineNode{
			{NodeID: "n-1", NodeType: "fetcher"},
		},
	}
	task := domain.Task{
		ID:             "t-1",
		PipelineID:     "p-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/test.md",
	}

	if err := svc.Submit(context.Background(), pipeline, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	waitForCompletion(observer, 5*time.Second)
	svc.Close()

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.failCount != 3 {
		t.Fatalf("expected 3 total attempts (2 fails + 1 success), got %d", runner.failCount)
	}
}

func TestExecutorFailsAfterMaxRetries(t *testing.T) {
	runner := &retryTestRunner{
		nodeType: "fetcher",
		maxFails: 99,
	}

	registry := NewNodeRunnerRegistry(runner)
	observer := newRetryTaskObserverStub()

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: NewLinearWorkflowBuilder(),
		NodeRunners:     registry,
		TaskObserver:    observer,
		MaxConcurrent:   1,
		MaxRetries:      1,
		RetryBackoffMs:  1,
	})

	pipeline := domain.Pipeline{
		ID:   "p-1",
		Name: "test",
		Nodes: []domain.PipelineNode{
			{NodeID: "n-1", NodeType: "fetcher"},
		},
	}
	task := domain.Task{
		ID:             "t-fail",
		PipelineID:     "p-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/test.md",
	}

	if err := svc.Submit(context.Background(), pipeline, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	waitForCompletion(observer, 5*time.Second)
	svc.Close()

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.failCount != 2 {
		t.Fatalf("expected 2 total attempts (1 initial + 1 retry), got %d", runner.failCount)
	}
}

func TestExecutorRetryWithNodeLevelSettings(t *testing.T) {
	runner := &retryTestRunner{
		nodeType: "fetcher",
		maxFails: 99,
	}

	registry := NewNodeRunnerRegistry(runner)
	observer := newRetryTaskObserverStub()

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: NewLinearWorkflowBuilder(),
		NodeRunners:     registry,
		TaskObserver:    observer,
		MaxConcurrent:   1,
		MaxRetries:      0,
		RetryBackoffMs:  1,
	})

	pipeline := domain.Pipeline{
		ID:   "p-1",
		Name: "test",
		Nodes: []domain.PipelineNode{
			{
				NodeID:   "n-1",
				NodeType: "fetcher",
				Settings: map[string]any{
					"retryCount":     float64(2),
					"retryBackoffMs": float64(1),
				},
			},
		},
	}
	task := domain.Task{
		ID:             "t-node-retry",
		PipelineID:     "p-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/test.md",
	}

	if err := svc.Submit(context.Background(), pipeline, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	waitForCompletion(observer, 5*time.Second)
	svc.Close()

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.failCount != 3 {
		t.Fatalf("expected 3 total attempts via node-level retry, got %d", runner.failCount)
	}
}

func TestExecutorRetryCanceledContext(t *testing.T) {
	runner := &retryTestRunner{
		nodeType: "fetcher",
		maxFails: 99,
	}

	registry := NewNodeRunnerRegistry(runner)
	observer := newRetryTaskObserverStub()

	svc := NewExecutorService(ExecutorServiceOptions{
		WorkflowBuilder: NewLinearWorkflowBuilder(),
		NodeRunners:     registry,
		TaskObserver:    observer,
		MaxConcurrent:   1,
		MaxRetries:      3,
		RetryBackoffMs:  500,
	})

	pipeline := domain.Pipeline{
		ID:   "p-1",
		Name: "test",
		Nodes: []domain.PipelineNode{
			{NodeID: "n-1", NodeType: "fetcher"},
		},
	}
	task := domain.Task{
		ID:             "t-cancel",
		PipelineID:     "p-1",
		SourceType:     domain.TaskSourceTypeFile,
		SourceLocation: "/tmp/test.md",
	}

	if err := svc.Submit(context.Background(), pipeline, task); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	// 给第一次尝试时间完成，然后立即取消以中断重试 backoff。
	time.Sleep(50 * time.Millisecond)
	svc.Close()

	waitForCompletion(observer, 5*time.Second)

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.failCount < 1 {
		t.Fatalf("expected at least 1 attempt, got %d", runner.failCount)
	}
}

func TestLinearWorkflowBuilderBuildsOrderedNodes(t *testing.T) {
	builder := NewLinearWorkflowBuilder()
	pipeline := domain.Pipeline{
		ID:   "p-1",
		Name: "test",
		Nodes: []domain.PipelineNode{
			{NodeID: "n-1", NodeType: "fetcher"},
			{NodeID: "n-2", NodeType: "parser"},
			{NodeID: "n-3", NodeType: "chunker"},
		},
	}
	task := domain.Task{
		ID:         "t-1",
		PipelineID: "p-1",
	}

	workflow, err := builder.Build(context.Background(), pipeline, task)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(workflow.NodeOrder) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(workflow.NodeOrder))
	}
	for i, item := range workflow.NodeOrder {
		expectedType := []string{"fetcher", "parser", "chunker"}[i]
		if item.Node.NodeType != expectedType {
			t.Errorf("node[%d]: expected %q, got %q", i, expectedType, item.Node.NodeType)
		}
	}
}

func TestExecutorSubmitRejectsEmptyPipeline(t *testing.T) {
	runner := &retryTestRunner{nodeType: "fetcher"}
	registry := NewNodeRunnerRegistry(runner)

	svc := NewExecutorService(ExecutorServiceOptions{
		NodeRunners: registry,
	})

	err := svc.Submit(context.Background(), domain.Pipeline{}, domain.Task{ID: "t-1"})
	if err == nil {
		t.Fatal("expected error for empty pipeline id")
	}
	if !strings.Contains(err.Error(), "pipeline") {
		t.Fatalf("expected pipeline-related error, got: %v", err)
	}
}
