package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

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
