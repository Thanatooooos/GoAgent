package history

import (
	"context"
	"sync"
)

// SummaryJobInput describes one conversation summary rebuild request.
type SummaryJobInput struct {
	ConversationID   string
	UserID           string
	TriggerMessageID string
	RebuildReason    string
}

// SummaryJobEnqueuer schedules conversation summary rebuild work.
type SummaryJobEnqueuer interface {
	EnqueueConversationSummary(ctx context.Context, input SummaryJobInput) error
}

type summaryCompressionRunner interface {
	runConversationSummaryCompression(ctx context.Context, input SummaryJobInput) error
}

// InMemorySummaryJobWorker processes summary jobs in a background goroutine.
type InMemorySummaryJobWorker struct {
	runner summaryCompressionRunner
	queue  chan SummaryJobInput
	stop   chan struct{}
	wg     sync.WaitGroup
}

func NewInMemorySummaryJobWorker(runner summaryCompressionRunner, queueSize int) *InMemorySummaryJobWorker {
	if queueSize <= 0 {
		queueSize = 32
	}
	return &InMemorySummaryJobWorker{
		runner: runner,
		queue:  make(chan SummaryJobInput, queueSize),
		stop:   make(chan struct{}),
	}
}

func (w *InMemorySummaryJobWorker) Start() {
	if w == nil || w.runner == nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.stop:
				return
			case input := <-w.queue:
				_ = w.runner.runConversationSummaryCompression(context.Background(), input)
			}
		}
	}()
}

func (w *InMemorySummaryJobWorker) Stop() {
	if w == nil {
		return
	}
	close(w.stop)
	w.wg.Wait()
}

func (w *InMemorySummaryJobWorker) EnqueueConversationSummary(_ context.Context, input SummaryJobInput) error {
	if w == nil || w.runner == nil {
		return nil
	}
	select {
	case w.queue <- input:
	default:
		go func() {
			_ = w.runner.runConversationSummaryCompression(context.Background(), input)
		}()
	}
	return nil
}
