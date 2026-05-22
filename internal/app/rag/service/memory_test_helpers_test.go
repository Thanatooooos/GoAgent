package service

import (
	"context"
	"errors"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type memoryItemRepoStub struct {
	createFn func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	updateFn func(context.Context, domain.MemoryItem) (domain.MemoryItem, error)
	getByID  func(context.Context, string) (domain.MemoryItem, error)
	listFn   func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error)
}

func (s memoryItemRepoStub) Create(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	return s.createFn(ctx, item)
}

func (s memoryItemRepoStub) Update(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	return s.updateFn(ctx, item)
}

func (s memoryItemRepoStub) GetByID(ctx context.Context, id string) (domain.MemoryItem, error) {
	return s.getByID(ctx, id)
}

func (s memoryItemRepoStub) List(ctx context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
	return s.listFn(ctx, filter)
}

type memoryItemEmbeddingRepoStub struct {
	upsertFn func(context.Context, []domain.MemoryItemEmbedding) error
	searchFn func(context.Context, []float32, port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error)
}

func (s memoryItemEmbeddingRepoStub) UpsertBatch(ctx context.Context, embeddings []domain.MemoryItemEmbedding) error {
	if s.upsertFn == nil {
		return nil
	}
	return s.upsertFn(ctx, embeddings)
}

func (s memoryItemEmbeddingRepoStub) SearchByVector(ctx context.Context, vector []float32, filter port.MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error) {
	if s.searchFn == nil {
		return nil, nil
	}
	return s.searchFn(ctx, vector, filter)
}

type embeddingServiceStub struct {
	vector    []float32
	err       error
	lastText  string
	callCount int
}

func (s *embeddingServiceStub) Embed(text string) ([]float32, error) {
	s.callCount++
	s.lastText = text
	if s.err != nil {
		return nil, s.err
	}
	return append([]float32(nil), s.vector...), nil
}

func (s *embeddingServiceStub) EmbedWithModel(string, string) ([]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) EmbedBatch([]string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) EmbedBatchWithModel([]string, string) ([][]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *embeddingServiceStub) Dimension() int {
	return len(s.vector)
}
