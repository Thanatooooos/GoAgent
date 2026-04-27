package port

import "context"

type ChunkDocumentTask struct {
	TaskID      string
	DocumentID  string
	TriggeredBy string
}

type RefreshRemoteDocumentTask struct {
	TaskID     string
	DocumentID string
	ScheduleID string
}

type TaskQueue interface {
	SubmitChunkDocument(ctx context.Context, task ChunkDocumentTask) error
	SubmitRefreshRemoteDocument(ctx context.Context, task RefreshRemoteDocumentTask) error
}
