package history

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestInMemorySummaryJobWorkerProcessesQueue(t *testing.T) {
	done := make(chan struct{})
	runner := &blockingSummaryRunner{done: done}
	worker := NewInMemorySummaryJobWorker(runner, 4)
	worker.Start()
	defer worker.Stop()

	if err := worker.EnqueueConversationSummary(context.Background(), SummaryJobInput{
		ConversationID: "c1",
		UserID:         "u1",
		RebuildReason:  "threshold_reached",
	}); err != nil {
		t.Fatalf("EnqueueConversationSummary returned error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected queued summary job to run")
	}
}

func TestInMemorySummaryJobWorkerOverflowRunsInline(t *testing.T) {
	block := make(chan struct{})
	runner := &blockingSummaryRunner{block: block}
	worker := NewInMemorySummaryJobWorker(runner, 1)
	worker.Start()
	defer worker.Stop()

	first := SummaryJobInput{ConversationID: "c1", UserID: "u1"}
	second := SummaryJobInput{ConversationID: "c2", UserID: "u2"}
	if err := worker.EnqueueConversationSummary(context.Background(), first); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	if err := worker.EnqueueConversationSummary(context.Background(), second); err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}

	close(block)
	deadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&runner.calls) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&runner.calls) < 2 {
		t.Fatalf("expected overflow path to run inline, calls=%d", runner.calls)
	}
}

type blockingSummaryRunner struct {
	done  chan struct{}
	block chan struct{}
	calls int32
}

func (r *blockingSummaryRunner) runConversationSummaryCompression(_ context.Context, _ SummaryJobInput) error {
	atomic.AddInt32(&r.calls, 1)
	if r.block != nil {
		<-r.block
	}
	if r.done != nil {
		close(r.done)
	}
	return nil
}
