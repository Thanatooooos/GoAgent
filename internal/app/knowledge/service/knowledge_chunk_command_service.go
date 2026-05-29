package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

func (s *KnowledgeChunkService) Create(ctx context.Context, input CreateKnowledgeChunkInput) (domain.KnowledgeChunk, error) {
	document, err := s.requireDocument(ctx, input.DocumentID)
	if err != nil {
		return domain.KnowledgeChunk{}, err
	}
	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeChunk{}, exception.NewClientException("operator id is required", nil)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return domain.KnowledgeChunk{}, exception.NewClientException("chunk content is required", nil)
	}

	chunkID := strings.TrimSpace(input.ChunkID)
	if chunkID == "" {
		id, err := distributedid.NextID()
		if err != nil {
			return domain.KnowledgeChunk{}, exception.NewServiceException("failed to generate knowledge chunk id", err)
		}
		chunkID = fmt.Sprintf("%d", id)
	}

	var created domain.KnowledgeChunk
	err = s.withMutationDeps(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		allChunks, err := chunkRepo.List(txCtx, port.KnowledgeChunkListFilter{DocumentID: document.ID})
		if err != nil {
			return exception.NewServiceException("failed to list knowledge chunks", err)
		}
		index := nextChunkIndex(allChunks)
		if input.Index != nil && *input.Index >= 0 {
			index = *input.Index
		}

		chunk := domain.NewKnowledgeChunk(chunkID, document.KnowledgeBaseID, document.ID, index, content, operatorID)
		enrichKnowledgeChunk(&chunk)
		if _, err := chunkRepo.Create(txCtx, chunk); err != nil {
			return exception.NewServiceException("failed to create knowledge chunk", err)
		}
		if err := s.upsertChunkVectorWithStore(txCtx, vectorStore, document, chunk); err != nil {
			return err
		}
		if err := s.updateDocumentChunkCountWithRepo(txCtx, documentRepo, document.ID, len(allChunks)+1, operatorID); err != nil {
			return err
		}
		created = chunk
		return nil
	})
	if err != nil {
		return domain.KnowledgeChunk{}, err
	}
	return created, nil
}

func (s *KnowledgeChunkService) Update(ctx context.Context, input UpdateKnowledgeChunkInput) error {
	document, err := s.requireDocument(ctx, input.DocumentID)
	if err != nil {
		return err
	}
	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return exception.NewClientException("chunk content is required", nil)
	}

	return s.withMutationDeps(ctx, func(
		txCtx context.Context,
		_ port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		chunk, err := s.requireChunkWithRepo(txCtx, chunkRepo, document.ID, input.ChunkID)
		if err != nil {
			return err
		}
		chunk.Content = content
		chunk.UpdatedBy = operatorID
		chunk.UpdatedAt = time.Now()
		enrichKnowledgeChunk(&chunk)

		if _, err := chunkRepo.Update(txCtx, chunk); err != nil {
			return exception.NewServiceException("failed to update knowledge chunk", err)
		}
		return s.syncChunkVectorWithStore(txCtx, vectorStore, document, chunk)
	})
}

func (s *KnowledgeChunkService) Delete(ctx context.Context, input DeleteKnowledgeChunkInput) error {
	document, err := s.requireDocument(ctx, input.DocumentID)
	if err != nil {
		return err
	}
	return s.withMutationDeps(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		chunk, err := s.requireChunkWithRepo(txCtx, chunkRepo, document.ID, input.ChunkID)
		if err != nil {
			return err
		}
		if err := chunkRepo.Delete(txCtx, chunk.ID); err != nil {
			return exception.NewServiceException("failed to delete knowledge chunk", err)
		}
		if err := s.deleteChunkVectorWithStore(txCtx, vectorStore, chunk.ID); err != nil {
			return err
		}
		remaining, err := chunkRepo.CountByDocumentID(txCtx, document.ID, nil)
		if err != nil {
			return exception.NewServiceException("failed to count remaining knowledge chunks", err)
		}
		return s.updateDocumentChunkCountWithRepo(txCtx, documentRepo, document.ID, remaining, resolveChunkOperatorID(document.UpdatedBy))
	})
}

func (s *KnowledgeChunkService) Enable(ctx context.Context, input EnableKnowledgeChunkInput) error {
	document, err := s.requireDocument(ctx, input.DocumentID)
	if err != nil {
		return err
	}
	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}
	return s.withMutationDeps(ctx, func(
		txCtx context.Context,
		_ port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		chunk, err := s.requireChunkWithRepo(txCtx, chunkRepo, document.ID, input.ChunkID)
		if err != nil {
			return err
		}
		chunk.Enabled = input.Enabled
		chunk.UpdatedBy = operatorID
		chunk.UpdatedAt = time.Now()
		if _, err := chunkRepo.Update(txCtx, chunk); err != nil {
			return exception.NewServiceException("failed to update knowledge chunk enabled status", err)
		}
		return s.syncChunkVectorWithStore(txCtx, vectorStore, document, chunk)
	})
}

func (s *KnowledgeChunkService) BatchToggleEnabled(ctx context.Context, input BatchToggleKnowledgeChunksInput) error {
	document, err := s.requireDocument(ctx, input.DocumentID)
	if err != nil {
		return err
	}
	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}
	return s.withMutationDeps(ctx, func(
		txCtx context.Context,
		_ port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
	) error {
		allChunks, err := chunkRepo.List(txCtx, port.KnowledgeChunkListFilter{DocumentID: document.ID})
		if err != nil {
			return exception.NewServiceException("failed to list knowledge chunks", err)
		}

		targetSet := map[string]struct{}{}
		for _, id := range input.ChunkIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				targetSet[id] = struct{}{}
			}
		}

		updatedAny := false
		now := time.Now()
		if len(targetSet) == 0 {
			rows, err := chunkRepo.UpdateEnabledByDocumentID(txCtx, document.ID, input.Enabled, operatorID, now)
			if err != nil {
				return exception.NewServiceException("failed to batch update knowledge chunks", err)
			}
			updatedAny = rows > 0
		} else {
			targetIDs := make([]string, 0, len(targetSet))
			for _, chunk := range allChunks {
				if _, ok := targetSet[chunk.ID]; ok {
					targetIDs = append(targetIDs, chunk.ID)
				}
			}
			rows, err := chunkRepo.UpdateEnabledByIDs(txCtx, document.ID, targetIDs, input.Enabled, operatorID, now)
			if err != nil {
				return exception.NewServiceException("failed to batch update knowledge chunks", err)
			}
			updatedAny = rows > 0
		}
		if !updatedAny {
			return nil
		}
		if len(targetSet) == 0 {
			return s.syncVectorsForAllChunksWithDeps(txCtx, chunkRepo, vectorStore, document)
		}
		if input.Enabled {
			targetChunks := make([]domain.KnowledgeChunk, 0, len(targetSet))
			for _, chunk := range allChunks {
				if _, ok := targetSet[chunk.ID]; ok {
					chunk.Enabled = true
					chunk.UpdatedBy = operatorID
					chunk.UpdatedAt = now
					targetChunks = append(targetChunks, chunk)
				}
			}
			return s.upsertChunkVectorsWithStore(txCtx, vectorStore, document, targetChunks)
		}
		targetIDs := make([]string, 0, len(targetSet))
		for id := range targetSet {
			targetIDs = append(targetIDs, id)
		}
		return s.deleteChunkVectorsWithStore(txCtx, vectorStore, targetIDs)
	})
}
