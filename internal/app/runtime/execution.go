package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"local/rag-project/internal/app/runtime/persistence"
)

func (r *Runtime) claimChatExecution(ctx context.Context, session persistence.Session) (context.Context, func(), error) {
	store, ok := r.Lifecycle.store.(persistence.ChatExecutionStore)
	if !ok {
		return ctx, func() {}, nil
	}
	ttl := r.ExecutionLeaseDuration
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	execution, err := store.ClaimChatExecution(ctx, session, uuid.NewString(), ttl)
	if err != nil {
		return ctx, nil, err
	}
	ctx, cancel := context.WithCancelCause(persistence.WithExecution(ctx, execution))
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(max(ttl/3, time.Millisecond))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(ctx, min(ttl/3, 5*time.Second))
				err := store.RenewChatExecution(renewCtx, execution, ttl)
				stop()
				if err != nil {
					cancel(persistence.ErrExecutionLeaseLost)
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(nil); <-done }, nil
}

func (r *Runtime) finishOwnedFailure(ctx context.Context, sink EventSink, session persistence.Session, status string, cause error) (RunResult, error) {
	store, ok := r.Lifecycle.store.(interface {
		FinishChatExecution(context.Context, persistence.Session, string, string) (JournalEntry, error)
	})
	if !ok {
		return RunResult{}, fmt.Errorf("chat execution finalization is unavailable")
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	entry, err := store.FinishChatExecution(cleanup, session, status, cause.Error())
	if err != nil {
		return RunResult{Status: StatusInterrupted, RuntimeSessionID: session.ID}, err
	}
	if sink != nil {
		if err := sink.Append(cleanup, entry); err != nil {
			return RunResult{Status: status, RuntimeSessionID: session.ID}, err
		}
	}
	return RunResult{Status: status, RuntimeSessionID: session.ID}, cause
}
