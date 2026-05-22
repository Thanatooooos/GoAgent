package domain

import "time"

type SessionChunk struct {
	ID             string
	ConversationID string
	MessageID      string
	UserID         string
	ChunkIndex     int
	Content        string
	ContentSummary string
	TokenEstimate  int
	CreateTime     time.Time
	UpdateTime     time.Time
}

type SessionChunkEmbedding struct {
	ChunkID    string
	Embedding  []float32
	CreateTime time.Time
	UpdateTime time.Time
}

type SessionChunkSearchHit struct {
	SessionChunk
	Score float32
}
