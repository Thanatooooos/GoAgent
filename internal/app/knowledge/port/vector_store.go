package port

import "context"

type ChunkVector struct {
	ChunkID         string
	DocumentID      string
	KnowledgeBaseID string
	Index           int
	Text            string
	Embedding       []float32
	Metadata        map[string]any
}

type VectorStore interface {
	UpsertDocumentChunks(ctx context.Context, chunks []ChunkVector) error
	DeleteByDocumentID(ctx context.Context, documentID string) error
	DeleteChunk(ctx context.Context, chunkID string) error
	DeleteChunks(ctx context.Context, chunkIDs []string) error
	UpdateChunk(ctx context.Context, chunk ChunkVector) error
}
