package chunk

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

const (
	defaultKnowledgePageSize = 10
	maxKnowledgePageSize     = 100
)

type CreateKnowledgeChunkInput struct {
	DocumentID string
	ChunkID    string
	Index      *int
	Content    string
	OperatorID string
}

type UpdateKnowledgeChunkInput struct {
	DocumentID string
	ChunkID    string
	Content    string
	OperatorID string
}

type DeleteKnowledgeChunkInput struct {
	DocumentID string
	ChunkID    string
}

type EnableKnowledgeChunkInput struct {
	DocumentID string
	ChunkID    string
	Enabled    bool
	OperatorID string
}

type BatchToggleKnowledgeChunksInput struct {
	DocumentID string
	ChunkIDs   []string
	Enabled    bool
	OperatorID string
}

type PageKnowledgeChunkInput struct {
	DocumentID string
	Page       int
	PageSize   int
	Enabled    *bool
}

type KnowledgeChunkPageResult struct {
	Items    []domain.KnowledgeChunk
	Total    int
	Page     int
	PageSize int
}

type KnowledgeChunkService struct {
	baseRepo     port.KnowledgeBaseRepository
	documentRepo port.KnowledgeDocumentRepository
	chunkRepo    port.KnowledgeChunkRepository
	vectorStore  port.VectorStore
	embedding    aiembedding.EmbeddingService
	transaction  KnowledgeChunkMutationTransaction
}

type KnowledgeChunkMutationTransaction func(
	ctx context.Context,
	fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
) error

func NewKnowledgeChunkService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
	embedding aiembedding.EmbeddingService,
	transaction ...KnowledgeChunkMutationTransaction,
) *KnowledgeChunkService {
	var tx KnowledgeChunkMutationTransaction
	if len(transaction) > 0 {
		tx = transaction[0]
	}
	return &KnowledgeChunkService{
		baseRepo:     baseRepo,
		documentRepo: documentRepo,
		chunkRepo:    chunkRepo,
		vectorStore:  vectorStore,
		embedding:    embedding,
		transaction:  tx,
	}
}
