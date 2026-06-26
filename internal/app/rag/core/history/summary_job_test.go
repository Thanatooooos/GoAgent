package history

import (
	"context"
	"sync"
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

func TestInMemorySummaryJobWorkerDeduplicatesEquivalentCoverage(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &dedupeSummaryRunner{started: started, release: release}
	worker := NewInMemorySummaryJobWorker(runner, 4)
	worker.Start()
	defer worker.Stop()

	input := SummaryJobInput{
		ConversationID:  "c1",
		UserID:          "u1",
		TargetMessageID: "m12",
	}
	if err := worker.EnqueueConversationSummary(context.Background(), input); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	<-started
	if err := worker.EnqueueConversationSummary(context.Background(), input); err != nil {
		t.Fatalf("duplicate enqueue failed: %v", err)
	}
	close(release)

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&runner.calls); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
}

func TestInMemorySummaryJobWorkerCoalescesNewerCoveragePerConversation(t *testing.T) {
	runner := &serialSummaryRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
	worker := NewInMemorySummaryJobWorker(runner, 1)
	worker.Start()
	defer worker.Stop()

	if err := worker.EnqueueConversationSummary(context.Background(), SummaryJobInput{
		ConversationID:  "c1",
		UserID:          "u1",
		TargetMessageID: "12",
	}); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	<-runner.started
	for _, target := range []string{"13", "14"} {
		if err := worker.EnqueueConversationSummary(context.Background(), SummaryJobInput{
			ConversationID:  "c1",
			UserID:          "u1",
			TargetMessageID: target,
		}); err != nil {
			t.Fatalf("enqueue target %s failed: %v", target, err)
		}
	}
	close(runner.release)

	select {
	case <-runner.done:
	case <-time.After(time.Second):
		t.Fatal("expected coalesced summary work to finish")
	}

	runner.mu.Lock()
	targets := append([]string(nil), runner.targets...)
	maxConcurrent := runner.maxConcurrent
	runner.mu.Unlock()
	if maxConcurrent != 1 {
		t.Fatalf("max concurrent runs = %d, want 1", maxConcurrent)
	}
	if len(targets) != 2 || targets[0] != "12" || targets[1] != "14" {
		t.Fatalf("processed targets = %v, want [12 14]", targets)
	}
}

type blockingSummaryRunner struct {
	done  chan struct{}
	block chan struct{}
	calls int32
}

type dedupeSummaryRunner struct {
	started chan struct{}
	release chan struct{}
	calls   int32
}

type serialSummaryRunner struct {
	started       chan struct{}
	release       chan struct{}
	done          chan struct{}
	mu            sync.Mutex
	targets       []string
	concurrent    int
	maxConcurrent int
}

func (r *serialSummaryRunner) runConversationSummaryCompression(_ context.Context, input SummaryJobInput) error {
	r.mu.Lock()
	r.concurrent++
	if r.concurrent > r.maxConcurrent {
		r.maxConcurrent = r.concurrent
	}
	r.targets = append(r.targets, input.TargetMessageID)
	call := len(r.targets)
	r.mu.Unlock()

	if call == 1 {
		close(r.started)
		<-r.release
	}

	r.mu.Lock()
	r.concurrent--
	if len(r.targets) == 2 {
		close(r.done)
	}
	r.mu.Unlock()
	return nil
}

func (r *dedupeSummaryRunner) runConversationSummaryCompression(context.Context, SummaryJobInput) error {
	if atomic.AddInt32(&r.calls, 1) == 1 {
		close(r.started)
	}
	<-r.release
	return nil
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
