package service

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

func (s *DocumentProcessService) persistDocumentChunks(
	ctx context.Context,
	documentID string,
	domainChunks []domain.KnowledgeChunk,
	vectorChunks []port.ChunkVector,
	operatorID string,
) error {
	if s.transaction == nil {
		return s.persistDocumentChunksWithDeps(ctx, s.documentRepo, s.chunkRepo, s.vectorStore, documentID, domainChunks, vectorChunks, operatorID)
	}
	return s.transaction(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		return s.persistDocumentChunksWithDeps(txCtx, documentRepo, chunkRepo, vectorStore, documentID, domainChunks, vectorChunks, operatorID)
	})
}

func (s *DocumentProcessService) persistDocumentChunksWithDeps(
	ctx context.Context,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
	documentID string,
	domainChunks []domain.KnowledgeChunk,
	vectorChunks []port.ChunkVector,
	operatorID string,
) error {
	if err := chunkRepo.DeleteByDocumentID(ctx, documentID); err != nil {
		return exception.NewServiceException("failed to delete old knowledge document chunks", err)
	}
	if err := vectorStore.DeleteByDocumentID(ctx, documentID); err != nil {
		return exception.NewServiceException("failed to delete old knowledge document vectors", err)
	}
	if err := chunkRepo.CreateBatch(ctx, domainChunks); err != nil {
		return exception.NewServiceException("failed to create knowledge document chunks", err)
	}
	if err := vectorStore.UpsertDocumentChunks(ctx, vectorChunks); err != nil {
		return exception.NewServiceException("failed to upsert knowledge document vectors", err)
	}
	if err := s.updateDocumentChunkCountWithRepo(ctx, documentRepo, documentID, len(domainChunks), operatorID); err != nil {
		return err
	}
	return nil
}

func (s *DocumentProcessService) updateDocumentChunkCount(ctx context.Context, documentID string, chunkCount int, operatorID string) error {
	return s.updateDocumentChunkCountWithRepo(ctx, s.documentRepo, documentID, chunkCount, operatorID)
}

func (s *DocumentProcessService) updateDocumentChunkCountWithRepo(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, documentID string, chunkCount int, operatorID string) error {
	_, err := documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
	), port.Set(
		port.KnowledgeDocument.ChunkCount.To(chunkCount),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document chunk count", err)
	}
	return nil
}
