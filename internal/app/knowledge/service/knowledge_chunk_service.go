package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/paging"
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

func (s *KnowledgeChunkService) Page(ctx context.Context, input PageKnowledgeChunkInput) (KnowledgeChunkPageResult, error) {
	if s == nil || s.chunkRepo == nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeChunkPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	total, err := s.chunkRepo.CountByDocumentID(ctx, documentID, input.Enabled)
	if err != nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("failed to count knowledge chunks", err)
	}
	items, err := s.chunkRepo.List(ctx, port.KnowledgeChunkListFilter{
		DocumentID: documentID,
		Enabled:    input.Enabled,
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeChunkPageResult{}, exception.NewServiceException("failed to page knowledge chunks", err)
	}
	return KnowledgeChunkPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

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

func (s *KnowledgeChunkService) requireDocument(ctx context.Context, documentID string) (domain.KnowledgeDocument, error) {
	if s == nil || s.documentRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.chunkRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document id is required", nil)
	}
	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document not found", nil)
	}
	return document, nil
}

func (s *KnowledgeChunkService) requireChunk(ctx context.Context, documentID, chunkID string) (domain.KnowledgeChunk, []domain.KnowledgeChunk, error) {
	allChunks, err := s.chunkRepo.List(ctx, port.KnowledgeChunkListFilter{DocumentID: documentID})
	if err != nil {
		return domain.KnowledgeChunk{}, nil, exception.NewServiceException("failed to list knowledge chunks", err)
	}
	chunkID = strings.TrimSpace(chunkID)
	for _, item := range allChunks {
		if item.ID == chunkID {
			return item, allChunks, nil
		}
	}
	return domain.KnowledgeChunk{}, nil, exception.NewClientException("knowledge chunk not found", nil)
}

func (s *KnowledgeChunkService) requireChunkWithRepo(ctx context.Context, chunkRepo port.KnowledgeChunkRepository, documentID, chunkID string) (domain.KnowledgeChunk, error) {
	if chunkRepo == nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("knowledge chunk repository is required", nil)
	}
	allChunks, err := chunkRepo.List(ctx, port.KnowledgeChunkListFilter{DocumentID: documentID})
	if err != nil {
		return domain.KnowledgeChunk{}, exception.NewServiceException("failed to list knowledge chunks", err)
	}
	chunkID = strings.TrimSpace(chunkID)
	for _, item := range allChunks {
		if item.ID == chunkID {
			return item, nil
		}
	}
	return domain.KnowledgeChunk{}, exception.NewClientException("knowledge chunk not found", nil)
}

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

func (s *KnowledgeChunkService) withMutationDeps(
	ctx context.Context,
	fn func(ctx context.Context, documentRepo port.KnowledgeDocumentRepository, chunkRepo port.KnowledgeChunkRepository, vectorStore port.VectorStore) error,
) error {
	if s.transaction == nil {
		return fn(ctx, s.documentRepo, s.chunkRepo, s.vectorStore)
	}
	return s.transaction(ctx, fn)
}

func (s *KnowledgeChunkService) updateDocumentChunkCount(ctx context.Context, documentID string, count int, operatorID string) error {
	if s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	return s.updateDocumentChunkCountWithRepo(ctx, s.documentRepo, documentID, count, operatorID)
}

func (s *KnowledgeChunkService) updateDocumentChunkCountWithRepo(
	ctx context.Context,
	documentRepo port.KnowledgeDocumentRepository,
	documentID string,
	count int,
	operatorID string,
) error {
	if documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	operatorID = strings.TrimSpace(operatorID)
	if operatorID == "" {
		operatorID = "system"
	}
	_, err := documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(documentID),
	), port.Set(
		port.KnowledgeDocument.ChunkCount.To(count),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document chunk count", err)
	}
	return nil
}

func enrichKnowledgeChunk(chunk *domain.KnowledgeChunk) {
	if chunk == nil {
		return
	}
	chunk.ContentHash = hashChunkContent(chunk.Content)
	chunk.CharCount = utf8.RuneCountInString(chunk.Content)
	chunk.TokenCount = len(strings.Fields(chunk.Content))
}

func hashChunkContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func nextChunkIndex(chunks []domain.KnowledgeChunk) int {
	maxIndex := -1
	for _, chunk := range chunks {
		if chunk.ChunkIndex > maxIndex {
			maxIndex = chunk.ChunkIndex
		}
	}
	return maxIndex + 1
}

func resolveChunkOperatorID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "system"
	}
	return value
}
