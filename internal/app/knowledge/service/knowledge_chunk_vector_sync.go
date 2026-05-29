package service

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

func (s *KnowledgeChunkService) rebuildDocumentVectors(ctx context.Context, document domain.KnowledgeDocument, chunks []domain.KnowledgeChunk) error {
	if s.vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	if err := s.vectorStore.DeleteByDocumentID(ctx, document.ID); err != nil {
		return exception.NewServiceException("failed to delete knowledge chunk vectors", err)
	}

	enabledChunks := make([]domain.KnowledgeChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Enabled {
			enabledChunks = append(enabledChunks, chunk)
		}
	}
	if len(enabledChunks) == 0 {
		return nil
	}
	if s.embedding == nil {
		return exception.NewServiceException("embedding service is required", nil)
	}
	if s.baseRepo == nil {
		return exception.NewServiceException("knowledge base repository is required", nil)
	}
	base, err := s.baseRepo.GetByID(ctx, document.KnowledgeBaseID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge base", err)
	}
	if base.ID == "" {
		return exception.NewClientException("knowledge base not found", nil)
	}

	texts := make([]string, 0, len(enabledChunks))
	for _, chunk := range enabledChunks {
		texts = append(texts, chunk.Content)
	}
	embeddings, err := s.embedding.EmbedBatchWithModel(texts, base.EmbeddingModel)
	if err != nil {
		return exception.NewServiceException("failed to embed knowledge chunks", err)
	}
	if len(embeddings) != len(enabledChunks) {
		return exception.NewServiceException("knowledge chunk embedding result size mismatch", nil)
	}

	vectors := make([]port.ChunkVector, 0, len(enabledChunks))
	for i, chunk := range enabledChunks {
		vectors = append(vectors, port.ChunkVector{
			ChunkID:         chunk.ID,
			DocumentID:      document.ID,
			KnowledgeBaseID: document.KnowledgeBaseID,
			Index:           chunk.ChunkIndex,
			Text:            chunk.Content,
			Embedding:       embeddings[i],
			Metadata:        buildKnowledgeVectorMetadata(document, chunk.ChunkIndex, nil),
		})
	}
	if err := s.vectorStore.UpsertDocumentChunks(ctx, vectors); err != nil {
		return exception.NewServiceException("failed to upsert knowledge chunk vectors", err)
	}
	return nil
}

func (s *KnowledgeChunkService) syncVectorsForAllChunks(ctx context.Context, document domain.KnowledgeDocument) error {
	allChunks, err := s.chunkRepo.List(ctx, port.KnowledgeChunkListFilter{DocumentID: document.ID})
	if err != nil {
		return exception.NewServiceException("failed to reload knowledge chunks", err)
	}
	return s.rebuildDocumentVectors(ctx, document, allChunks)
}

func (s *KnowledgeChunkService) syncChunkVector(ctx context.Context, document domain.KnowledgeDocument, chunk domain.KnowledgeChunk) error {
	return s.syncChunkVectorWithStore(ctx, s.vectorStore, document, chunk)
}

func (s *KnowledgeChunkService) syncChunkVectorWithStore(ctx context.Context, vectorStore port.VectorStore, document domain.KnowledgeDocument, chunk domain.KnowledgeChunk) error {
	if chunk.Enabled {
		return s.upsertChunkVectorWithStore(ctx, vectorStore, document, chunk)
	}
	return s.deleteChunkVectorWithStore(ctx, vectorStore, chunk.ID)
}

func (s *KnowledgeChunkService) upsertChunkVector(ctx context.Context, document domain.KnowledgeDocument, chunk domain.KnowledgeChunk) error {
	return s.upsertChunkVectorWithStore(ctx, s.vectorStore, document, chunk)
}

func (s *KnowledgeChunkService) upsertChunkVectorWithStore(ctx context.Context, vectorStore port.VectorStore, document domain.KnowledgeDocument, chunk domain.KnowledgeChunk) error {
	return s.upsertChunkVectorsWithStore(ctx, vectorStore, document, []domain.KnowledgeChunk{chunk})
}

func (s *KnowledgeChunkService) upsertChunkVectors(ctx context.Context, document domain.KnowledgeDocument, chunks []domain.KnowledgeChunk) error {
	return s.upsertChunkVectorsWithStore(ctx, s.vectorStore, document, chunks)
}

func (s *KnowledgeChunkService) upsertChunkVectorsWithStore(ctx context.Context, vectorStore port.VectorStore, document domain.KnowledgeDocument, chunks []domain.KnowledgeChunk) error {
	if vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	enabledChunks := make([]domain.KnowledgeChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Enabled {
			enabledChunks = append(enabledChunks, chunk)
		}
	}
	if len(enabledChunks) == 0 {
		return nil
	}
	if s.embedding == nil {
		return exception.NewServiceException("embedding service is required", nil)
	}
	if s.baseRepo == nil {
		return exception.NewServiceException("knowledge base repository is required", nil)
	}
	base, err := s.baseRepo.GetByID(ctx, document.KnowledgeBaseID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge base", err)
	}
	if base.ID == "" {
		return exception.NewClientException("knowledge base not found", nil)
	}
	texts := make([]string, 0, len(enabledChunks))
	for _, chunk := range enabledChunks {
		texts = append(texts, chunk.Content)
	}
	embeddings, err := s.embedding.EmbedBatchWithModel(texts, base.EmbeddingModel)
	if err != nil {
		return exception.NewServiceException("failed to embed knowledge chunks", err)
	}
	if len(embeddings) != len(enabledChunks) {
		return exception.NewServiceException("knowledge chunk embedding result size mismatch", nil)
	}
	vectors := make([]port.ChunkVector, 0, len(enabledChunks))
	for i, chunk := range enabledChunks {
		vectors = append(vectors, port.ChunkVector{
			ChunkID:         chunk.ID,
			DocumentID:      document.ID,
			KnowledgeBaseID: document.KnowledgeBaseID,
			Index:           chunk.ChunkIndex,
			Text:            chunk.Content,
			Embedding:       embeddings[i],
			Metadata:        buildKnowledgeVectorMetadata(document, chunk.ChunkIndex, nil),
		})
	}
	if len(vectors) == 1 {
		if err := vectorStore.UpdateChunk(ctx, vectors[0]); err != nil {
			return exception.NewServiceException("failed to update knowledge chunk vector", err)
		}
		return nil
	}
	if err := vectorStore.UpsertDocumentChunks(ctx, vectors); err != nil {
		return exception.NewServiceException("failed to upsert knowledge chunk vectors", err)
	}
	return nil
}

func (s *KnowledgeChunkService) deleteChunkVector(ctx context.Context, chunkID string) error {
	return s.deleteChunkVectorWithStore(ctx, s.vectorStore, chunkID)
}

func (s *KnowledgeChunkService) deleteChunkVectorWithStore(ctx context.Context, vectorStore port.VectorStore, chunkID string) error {
	if vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	if err := vectorStore.DeleteChunk(ctx, chunkID); err != nil {
		return exception.NewServiceException("failed to delete knowledge chunk vector", err)
	}
	return nil
}

func (s *KnowledgeChunkService) deleteChunkVectors(ctx context.Context, chunkIDs []string) error {
	return s.deleteChunkVectorsWithStore(ctx, s.vectorStore, chunkIDs)
}

func (s *KnowledgeChunkService) deleteChunkVectorsWithStore(ctx context.Context, vectorStore port.VectorStore, chunkIDs []string) error {
	if vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	if err := vectorStore.DeleteChunks(ctx, chunkIDs); err != nil {
		return exception.NewServiceException("failed to delete knowledge chunk vectors", err)
	}
	return nil
}

func (s *KnowledgeChunkService) syncVectorsForAllChunksWithDeps(
	ctx context.Context,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
	document domain.KnowledgeDocument,
) error {
	allChunks, err := chunkRepo.List(ctx, port.KnowledgeChunkListFilter{DocumentID: document.ID})
	if err != nil {
		return exception.NewServiceException("failed to reload knowledge chunks", err)
	}
	if vectorStore == nil {
		return exception.NewServiceException("vector store is required", nil)
	}
	if err := vectorStore.DeleteByDocumentID(ctx, document.ID); err != nil {
		return exception.NewServiceException("failed to delete knowledge chunk vectors", err)
	}
	return s.upsertChunkVectorsWithStore(ctx, vectorStore, document, allChunks)
}
