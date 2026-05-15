package port

import "context"

type ChunkDocumentTask struct {
	TaskID      string
	DocumentID  string
	TriggeredBy string
}

type TaskQueue interface {
	SubmitChunkDocument(ctx context.Context, task ChunkDocumentTask) error
}
