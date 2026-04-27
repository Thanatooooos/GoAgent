package domain

import "time"

type KnowledgeChunk struct {
	ID              string
	KnowledgeBaseID string
	DocumentID      string
	ChunkIndex      int
	Content         string
	ContentHash     string
	CharCount       int
	TokenCount      int
	Enabled         bool
	CreatedBy       string
	UpdatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewKnowledgeChunk(id, knowledgeBaseID, documentID string, chunkIndex int, content, createdBy string) KnowledgeChunk {
	now := time.Now()
	return KnowledgeChunk{
		ID:              id,
		KnowledgeBaseID: knowledgeBaseID,
		DocumentID:      documentID,
		ChunkIndex:      chunkIndex,
		Content:         content,
		Enabled:         true,
		CreatedBy:       createdBy,
		UpdatedBy:       createdBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (c KnowledgeChunk) IsEnabled() bool {
	return c.Enabled
}
