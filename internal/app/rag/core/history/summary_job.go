package history

import (
	"context"
	"math/big"
	"strings"
	"sync"

	"local/rag-project/internal/framework/log"
)

// SummaryJobInput describes one conversation summary rebuild request.
type SummaryJobInput struct {
	ConversationID  string
	UserID          string
	TargetMessageID string
	RebuildReason   string
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
	mu     sync.Mutex
	active map[string]*summaryJobState
}

type summaryJobState struct {
	current SummaryJobInput
	pending *SummaryJobInput
}

func NewInMemorySummaryJobWorker(runner summaryCompressionRunner, queueSize int) *InMemorySummaryJobWorker {
	if queueSize <= 0 {
		queueSize = 32
	}
	return &InMemorySummaryJobWorker{
		runner: runner,
		queue:  make(chan SummaryJobInput, queueSize),
		stop:   make(chan struct{}),
		active: make(map[string]*summaryJobState),
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
				w.run(input)
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
	key := summaryJobKey(input)
	w.mu.Lock()
	if state, exists := w.active[key]; exists {
		if summaryJobTargetCompare(input.TargetMessageID, state.current.TargetMessageID) > 0 &&
			(state.pending == nil || summaryJobTargetCompare(input.TargetMessageID, state.pending.TargetMessageID) > 0) {
			pending := input
			state.pending = &pending
		}
		w.mu.Unlock()
		return nil
	}
	w.active[key] = &summaryJobState{current: input}
	w.mu.Unlock()

	select {
	case w.queue <- input:
	default:
		go w.run(input)
	}
	return nil
}

func (w *InMemorySummaryJobWorker) run(input SummaryJobInput) {
	for {
		ctx := context.Background()
		if err := w.runner.runConversationSummaryCompression(ctx, input); err != nil {
			log.FromContext(ctx).Warnw(
				"summary compression job failed",
				"conversation_id", input.ConversationID,
				"user_id", input.UserID,
				"target_message_id", input.TargetMessageID,
				"error", err,
			)
		}

		key := summaryJobKey(input)
		w.mu.Lock()
		state := w.active[key]
		if state != nil && state.pending != nil {
			input = *state.pending
			state.current = input
			state.pending = nil
			w.mu.Unlock()
			continue
		}
		delete(w.active, key)
		w.mu.Unlock()
		return
	}
}

func summaryJobKey(input SummaryJobInput) string {
	return strings.TrimSpace(input.ConversationID) + ":" +
		strings.TrimSpace(input.UserID)
}

func summaryJobTargetCompare(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	leftInt, leftOK := new(big.Int).SetString(left, 10)
	rightInt, rightOK := new(big.Int).SetString(right, 10)
	if leftOK && rightOK {
		return leftInt.Cmp(rightInt)
	}
	return strings.Compare(left, right)
}
