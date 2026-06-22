package observer

import (
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

type taskObserverTaskNodeRepoStub struct {
	records map[string]domain.TaskNode
}

func newTaskObserverTaskNodeRepoStub() *taskObserverTaskNodeRepoStub {
	return &taskObserverTaskNodeRepoStub{records: map[string]domain.TaskNode{}}
}

func (s *taskObserverTaskNodeRepoStub) key(taskID, nodeID string) string {
	return taskID + ":" + nodeID
}

func (s *taskObserverTaskNodeRepoStub) Create(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	s.records[s.key(node.TaskID, node.NodeID)] = node
	return node, nil
}

func (s *taskObserverTaskNodeRepoStub) Update(ctx context.Context, node domain.TaskNode) (domain.TaskNode, error) {
	current := s.records[s.key(node.TaskID, node.NodeID)]
	if current.CreatedAt.IsZero() {
		current.CreatedAt = node.CreatedAt
	}
	if node.Output == nil {
		node.Output = current.Output
	}
	if node.Message == "" {
		node.Message = current.Message
	}
	s.records[s.key(node.TaskID, node.NodeID)] = node
	return node, nil
}

func (s *taskObserverTaskNodeRepoStub) GetByTaskIDAndNodeID(ctx context.Context, taskID string, nodeID string) (domain.TaskNode, error) {
	return s.records[s.key(taskID, nodeID)], nil
}

func (s *taskObserverTaskNodeRepoStub) ListByTaskID(ctx context.Context, taskID string) ([]domain.TaskNode, error) {
	return nil, nil
}

func TestRepositoryTaskObserverPersistsRetryMetadata(t *testing.T) {
	t.Parallel()

	nodeRepo := newTaskObserverTaskNodeRepoStub()
	observer := NewRepositoryTaskObserver(nil, nodeRepo)
	observer.now = func() time.Time {
		return time.Unix(1714713600, 0)
	}

	task := domain.Task{ID: "task-1", PipelineID: "pipe-1"}
	node := ingestionworkflow.WorkflowNodeSpec{
		Order: 1,
		Node: domain.PipelineNode{
			NodeID:   "node-1",
			NodeType: domain.PipelineNodeTypeFetcher,
		},
	}

	if err := observer.OnNodeStarted(context.Background(), task, node); err != nil {
		t.Fatalf("OnNodeStarted() error = %v", err)
	}
	retryErr := exception.NewServiceException("temporary unavailable", errors.New("timeout"))
	if err := observer.OnNodeRetry(context.Background(), task, node, 2, 3*time.Second, retryErr); err != nil {
		t.Fatalf("OnNodeRetry() error = %v", err)
	}

	record, err := nodeRepo.GetByTaskIDAndNodeID(context.Background(), task.ID, node.Node.NodeID)
	if err != nil {
		t.Fatalf("GetByTaskIDAndNodeID() error = %v", err)
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "retryCount"); got != 2 {
		t.Fatalf("expected retryCount=2, got %d", got)
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "lastRetryAttempt"); got != 2 {
		t.Fatalf("expected lastRetryAttempt=2, got %d", got)
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "lastRetryBackoffMs"); got != 3000 {
		t.Fatalf("expected lastRetryBackoffMs=3000, got %d", got)
	}
	if got := ingestionworkflow.ReadStringSetting(record.Output, "errorCategory"); got != "service" {
		t.Fatalf("expected errorCategory=service, got %q", got)
	}
	if record.Status != TaskNodeStatusRunning {
		t.Fatalf("expected status running, got %q", record.Status)
	}
}

func TestRepositoryTaskObserverPersistsCompletionMetadata(t *testing.T) {
	t.Parallel()

	nodeRepo := newTaskObserverTaskNodeRepoStub()
	observer := NewRepositoryTaskObserver(nil, nodeRepo)
	task := domain.Task{ID: "task-1", PipelineID: "pipe-1"}
	node := ingestionworkflow.WorkflowNodeSpec{
		Order: 1,
		Node: domain.PipelineNode{
			NodeID:   "node-1",
			NodeType: domain.PipelineNodeTypeIndexer,
		},
	}

	if err := observer.OnNodeStarted(context.Background(), task, node); err != nil {
		t.Fatalf("OnNodeStarted() error = %v", err)
	}
	if err := observer.OnNodeRetry(context.Background(), task, node, 1, time.Second, exception.NewServiceException("vector store timeout", nil)); err != nil {
		t.Fatalf("OnNodeRetry() error = %v", err)
	}
	if err := observer.OnNodeCompleted(context.Background(), task, node, map[string]any{"chunkCount": 3}, 2*time.Second, nil); err != nil {
		t.Fatalf("OnNodeCompleted() error = %v", err)
	}

	record, err := nodeRepo.GetByTaskIDAndNodeID(context.Background(), task.ID, node.Node.NodeID)
	if err != nil {
		t.Fatalf("GetByTaskIDAndNodeID() error = %v", err)
	}
	if record.Status != TaskNodeStatusSuccess {
		t.Fatalf("expected status success, got %q", record.Status)
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "attemptCount"); got != 2 {
		t.Fatalf("expected attemptCount=2, got %d", got)
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "durationMs"); got != 2000 {
		t.Fatalf("expected durationMs=2000, got %d", got)
	}
	if got, ok := record.Output["success"].(bool); !ok || !got {
		t.Fatalf("expected success=true, got %#v", record.Output["success"])
	}
	if got := ingestionworkflow.ReadIntSetting(record.Output, "chunkCount"); got != 3 {
		t.Fatalf("expected chunkCount=3, got %d", got)
	}
}
