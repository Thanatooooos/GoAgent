package domain

import "time"

const (
	KnowledgeDocumentChunkLogStatusPending = "pending"
	KnowledgeDocumentChunkLogStatusRunning = "running"
	KnowledgeDocumentChunkLogStatusSuccess = "success"
	KnowledgeDocumentChunkLogStatusFailed  = "failed"
)

type KnowledgeDocumentChunkLog struct {
	ID              string
	DocumentID      string
	Status          string
	ProcessMode     string
	ChunkStrategy   string
	PipelineID      string
	ExtractDuration int64
	ChunkDuration   int64
	EmbedDuration   int64
	PersistDuration int64
	TotalDuration   int64
	ChunkCount      int
	ErrorMessage    string
	StartTime       *time.Time
	EndTime         *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewKnowledgeDocumentChunkLog(id, documentID string) KnowledgeDocumentChunkLog {
	now := time.Now()
	return KnowledgeDocumentChunkLog{
		ID:         id,
		DocumentID: documentID,
		Status:     KnowledgeDocumentChunkLogStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
