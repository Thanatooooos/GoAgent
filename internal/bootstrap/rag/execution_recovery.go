package rag

import (
	"context"
	"time"

	raghttp "local/rag-project/internal/adapter/http/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/log"
)

func (r *Runtime) startExecutionRecovery(store *postgresruntime.Store) {
	ctx, cancel := context.WithCancel(context.Background())
	r.executionCancel = cancel
	r.executionWG.Add(1)
	go func() {
		defer r.executionWG.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		run := func() {
			batch, stop := context.WithTimeout(ctx, 30*time.Second)
			defer stop()
			if err := store.RecoverChatExecutions(batch, 20, func(ctx context.Context, event convruntime.JournalEntry) error {
				return raghttp.RecoverRuntimePublication(ctx, r.StreamManager, event.TraceID, func(context.Context) (convruntime.JournalEntry, error) { return event, nil })
			}); err != nil && ctx.Err() == nil {
				log.Warnf("chat execution recovery failed: %v", err)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
