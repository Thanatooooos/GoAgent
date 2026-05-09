package service

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/framework/exception"
)

func TestMetricsObserverAggregatesTaskAndNodeMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService(8)
	observer := NewMetricsObserver(metrics)
	task := domain.Task{ID: "t-1", PipelineID: "p-1"}
	node := WorkflowNodeSpec{
		Order: 1,
		Node: domain.PipelineNode{
			NodeID:   "n-1",
			NodeType: "fetcher",
		},
	}

	metrics.RecordTaskSubmitted(task)
	if err := observer.OnTaskStarted(context.Background(), task); err != nil {
		t.Fatalf("OnTaskStarted() error = %v", err)
	}
	if err := observer.OnNodeStarted(context.Background(), task, node); err != nil {
		t.Fatalf("OnNodeStarted() error = %v", err)
	}
	if err := observer.OnNodeRetry(context.Background(), task, node, 1, 100*time.Millisecond, exception.NewServiceException("retry", nil)); err != nil {
		t.Fatalf("OnNodeRetry() error = %v", err)
	}
	if err := observer.OnNodeCompleted(context.Background(), task, node, nil, 250*time.Millisecond, nil); err != nil {
		t.Fatalf("OnNodeCompleted() error = %v", err)
	}
	if err := observer.OnTaskCompleted(context.Background(), task, ExecutionState{}, nil); err != nil {
		t.Fatalf("OnTaskCompleted() error = %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.RunningTasks != 0 || snapshot.UsedSlots != 0 {
		t.Fatalf("unexpected running snapshot: %+v", snapshot)
	}
	if snapshot.MaxConcurrent != 8 {
		t.Fatalf("MaxConcurrent = %d, want 8", snapshot.MaxConcurrent)
	}
	if snapshot.Totals.Submitted != 1 || snapshot.Totals.Started != 1 || snapshot.Totals.Succeeded != 1 {
		t.Fatalf("unexpected totals: %+v", snapshot.Totals)
	}
	if snapshot.Totals.Retries != 1 || snapshot.Totals.Failed != 0 || snapshot.Totals.Canceled != 0 {
		t.Fatalf("unexpected retry/cancel totals: %+v", snapshot.Totals)
	}
	if snapshot.Rates.SuccessRate != 1 || snapshot.Rates.FailureRate != 0 {
		t.Fatalf("unexpected rates: %+v", snapshot.Rates)
	}
	if len(snapshot.Nodes) != 1 {
		t.Fatalf("expected 1 node metric, got %d", len(snapshot.Nodes))
	}
	nodeSnapshot := snapshot.Nodes[0]
	if nodeSnapshot.NodeType != "fetcher" ||
		nodeSnapshot.Runs != 1 ||
		nodeSnapshot.Successes != 1 ||
		nodeSnapshot.Failures != 0 ||
		nodeSnapshot.Retries != 1 ||
		nodeSnapshot.AvgDurationMs != 250 ||
		nodeSnapshot.MaxDurationMs != 250 {
		t.Fatalf("unexpected node snapshot: %+v", nodeSnapshot)
	}
}

func TestMetricsObserverCountsCanceledTask(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService(4)
	observer := NewMetricsObserver(metrics)
	task := domain.Task{ID: "t-cancel"}

	metrics.RecordTaskSubmitted(task)
	if err := observer.OnTaskStarted(context.Background(), task); err != nil {
		t.Fatalf("OnTaskStarted() error = %v", err)
	}
	err := observer.OnTaskCompleted(context.Background(), task, ExecutionState{}, exception.NewServiceException("workflow canceled", context.Canceled))
	if err != nil {
		t.Fatalf("OnTaskCompleted() error = %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Totals.Canceled != 1 || snapshot.Totals.Failed != 0 || snapshot.Totals.Succeeded != 0 {
		t.Fatalf("unexpected canceled totals: %+v", snapshot.Totals)
	}
	if snapshot.Rates.SuccessRate != 0 || snapshot.Rates.FailureRate != 0 {
		t.Fatalf("unexpected rates for canceled task: %+v", snapshot.Rates)
	}
}

func TestMetricsServiceRecordsReconcileEvents(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService(2)
	metrics.RecordReconcileEvent(ReconcileMetricsEvent{
		Source:          "task_completion",
		TaskID:          "task-1",
		DocumentID:      "doc-1",
		DocumentUpdated: true,
		ChunkLogUpdated: true,
	})
	metrics.RecordReconcileEvent(ReconcileMetricsEvent{
		Source:       "scan",
		TaskID:       "task-2",
		DocumentID:   "doc-2",
		Skipped:      true,
		ErrorMessage: "chunk log mismatch",
	})

	snapshot := metrics.Snapshot()
	if snapshot.Reconcile.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2", snapshot.Reconcile.Attempts)
	}
	if snapshot.Reconcile.DocumentUpdated != 1 || snapshot.Reconcile.ChunkLogUpdated != 1 {
		t.Fatalf("unexpected reconcile update totals: %+v", snapshot.Reconcile)
	}
	if snapshot.Reconcile.Skipped != 1 || snapshot.Reconcile.Failures != 1 {
		t.Fatalf("unexpected reconcile skipped/failure totals: %+v", snapshot.Reconcile)
	}
	if snapshot.Reconcile.LastFailure == nil {
		t.Fatalf("expected last failure to be recorded")
	}
	if snapshot.Reconcile.LastFailure.TaskID != "task-2" ||
		snapshot.Reconcile.LastFailure.DocumentID != "doc-2" ||
		snapshot.Reconcile.LastFailure.Source != "scan" {
		t.Fatalf("unexpected last failure snapshot: %+v", snapshot.Reconcile.LastFailure)
	}
}
