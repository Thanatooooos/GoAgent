package rag

import (
	"context"
	"time"

	raghttp "local/rag-project/internal/adapter/http/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/log"
)

func (r *Runtime) startPublicationRecovery(publisher *postgresruntime.ChatPublisher) {
	ctx, cancel := context.WithCancel(context.Background())
	r.publicationCancel = cancel
	r.publicationWG.Add(1)
	go func() {
		defer r.publicationWG.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		run := func() {
			runCtx, stop := context.WithTimeout(ctx, 45*time.Second)
			defer stop()
			err := publisher.RecoverPending(runCtx, 20, func(ctx context.Context, finish convruntime.JournalEntry) error {
				return raghttp.RecoverRuntimePublication(ctx, r.StreamManager, finish.TraceID, func(context.Context) (convruntime.JournalEntry, error) { return finish, nil })
			})
			if err != nil && ctx.Err() == nil {
				log.Warnf("chat answer publication recovery failed: %v", err)
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
