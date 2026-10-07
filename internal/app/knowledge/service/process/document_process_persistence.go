package process

import (
	"context"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

func (s *DocumentProcessService) updateDocumentEnrichment(ctx context.Context, documentID string, enrichment documentEnrichment, operatorID string) error {
	return s.updateDocumentEnrichmentWithRepo(ctx, s.documentRepo, documentID, enrichment, operatorID)
}

func (s *DocumentProcessService) updateDocumentEnrichmentWithRepo(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, documentID string, enrichment documentEnrichment, operatorID string) error {
	_, err := documentRepo.UpdateFields(ctx, port.Where(port.KnowledgeDocument.ID.Eq(documentID)), port.Set(
		port.KnowledgeDocument.Summary.To(enrichment.Summary),
		port.KnowledgeDocument.SummaryStatus.To(enrichment.SummaryStatus),
		port.KnowledgeDocument.SummaryErrorMessage.To(enrichment.SummaryError),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(s.now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document enrichment", err)
	}
	return nil
}

func (s *DocumentProcessService) persistDocumentChunks(
	ctx context.Context,
	documentID string,
	domainChunks []domain.KnowledgeChunk,
	vectorChunks []port.ChunkVector,
	operatorID string,
	imageGeneration string,
	staged bool,
	enrichment documentEnrichment,
	result documentProcessResult,
) error {
	if s.transaction == nil {
		return s.persistDocumentChunksWithDeps(ctx, s.documentRepo, s.chunkRepo, s.vectorStore, documentID, domainChunks, vectorChunks, operatorID, imageGeneration, staged, enrichment)
	}
	persistStartedAt := s.now()
	return s.transaction(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		if err := s.persistDocumentChunksWithDeps(txCtx, documentRepo, chunkRepo, vectorStore, documentID, domainChunks, vectorChunks, operatorID, imageGeneration, staged, enrichment); err != nil {
			return err
		}
		result.PersistDuration = elapsedMillis(persistStartedAt, s.now())
		return s.completeOwnedPublication(txCtx, documentRepo, result)
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
	imageGeneration string,
	staged bool,
	enrichment documentEnrichment,
) error {
	if locker, ok := documentRepo.(interface {
		LockDocumentForPublication(context.Context, string) error
	}); ok {
		if err := locker.LockDocumentForPublication(ctx, documentID); err != nil {
			return exception.NewServiceException("failed to lock document for publication", err)
		}
	}
	if _, owned := port.CurrentChunkJob(ctx); owned {
		fencer, ok := documentRepo.(interface{ FenceChunkJob(context.Context) error })
		if !ok {
			return port.ErrChunkLeaseLost
		}
		if err := fencer.FenceChunkJob(ctx); err != nil {
			return err
		}
	}
	if err := chunkRepo.DeleteByDocumentID(ctx, documentID); err != nil {
		return exception.NewServiceException("failed to delete old knowledge document chunks", err)
	}
	if s.imageEvidence != nil && !staged {
		if revisions, ok := documentRepo.(interface {
			ClearActiveImageRevision(context.Context, string, string) error
		}); ok {
			if err := revisions.ClearActiveImageRevision(ctx, documentID, imageGeneration); err != nil {
				return exception.NewServiceException("failed to fence old image revision", err)
			}
		}
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
	if err := s.updateDocumentEnrichmentWithRepo(ctx, documentRepo, documentID, enrichment, operatorID); err != nil {
		return err
	}
	if staged {
		revisions, ok := documentRepo.(interface {
			PublishStagedImageRevision(context.Context, string, string) error
		})
		if !ok {
			return exception.NewServiceException("staged image publication is unavailable", nil)
		}
		if err := revisions.PublishStagedImageRevision(ctx, documentID, imageGeneration); err != nil {
			return exception.NewServiceException("failed to publish staged image revision", err)
		}
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
