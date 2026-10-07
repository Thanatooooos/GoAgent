package port

import (
	"context"
	"errors"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
)

var ErrChunkLeaseLost = errors.New("document processing lease is no longer current")

type ChunkJob struct {
	ChunkDocumentTask
	Epoch         int64
	Owner         string
	Document      domain.KnowledgeDocument
	Refreshed     bool
	SourceFileURL string
}
type ChunkJobs interface {
	Admit(context.Context, ChunkDocumentTask, *domain.KnowledgeDocument) (ChunkJob, error)
	Claim(context.Context, string, string, time.Duration) (*ChunkJob, error)
	Renew(context.Context, ChunkJob, time.Duration) error
	Fail(context.Context, ChunkJob, string) error
	Pending(context.Context, int) ([]ChunkDocumentTask, error)
	RecoverExpired(context.Context) (int64, error)
}
type chunkJobContextKey struct{}

func WithChunkJob(ctx context.Context, job ChunkJob) context.Context {
	return context.WithValue(ctx, chunkJobContextKey{}, job)
}
func CurrentChunkJob(ctx context.Context) (ChunkJob, bool) {
	job, ok := ctx.Value(chunkJobContextKey{}).(ChunkJob)
	return job, ok
}
