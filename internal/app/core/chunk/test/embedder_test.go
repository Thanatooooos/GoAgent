package chunk_test

import (
	"errors"
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

type stubEmbeddingService struct {
	vectors [][]float32
	err     error
	modelID string
}

func (s *stubEmbeddingService) Embed(text string) ([]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *stubEmbeddingService) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return nil, errors.New("not implemented")
}

func (s *stubEmbeddingService) EmbedBatch(texts []string) ([][]float32, error) {
	return s.vectors, s.err
}

func (s *stubEmbeddingService) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	s.modelID = modelID
	return s.vectors, s.err
}

func (s *stubEmbeddingService) Dimension() int {
	if len(s.vectors) == 0 || len(s.vectors[0]) == 0 {
		return 0
	}
	return len(s.vectors[0])
}

func TestEmbedderAttachEmbeddings(t *testing.T) {
	service := &stubEmbeddingService{
		vectors: [][]float32{{0.1, 0.2}, {0.3, 0.4}},
	}
	embedder := chunk.NewEmbedder(service)

	result, err := embedder.AttachEmbeddings([]chunk.Chunk{
		chunk.NewChunk("chunk-0000", 0, "first"),
		chunk.NewChunk("chunk-0001", 1, "second"),
	})
	if err != nil {
		t.Fatalf("attach embeddings returned error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(result))
	}
	if len(result[0].Embedding) != 2 || result[1].Embedding[1] != 0.4 {
		t.Fatalf("unexpected embeddings: %#v", result)
	}
}

func TestEmbedderAttachEmbeddingsWithModel(t *testing.T) {
	service := &stubEmbeddingService{
		vectors: [][]float32{{0.1}},
	}
	embedder := chunk.NewEmbedder(service)

	_, err := embedder.AttachEmbeddingsWithModel([]chunk.Chunk{
		chunk.NewChunk("chunk-0000", 0, "first"),
	}, "demo-embedding")
	if err != nil {
		t.Fatalf("attach embeddings with model returned error: %v", err)
	}
	if service.modelID != "demo-embedding" {
		t.Fatalf("expected model id to be passed through, got %s", service.modelID)
	}
}

func TestEmbedderReturnsMismatchError(t *testing.T) {
	service := &stubEmbeddingService{
		vectors: [][]float32{{0.1}},
	}
	embedder := chunk.NewEmbedder(service)

	_, err := embedder.AttachEmbeddings([]chunk.Chunk{
		chunk.NewChunk("chunk-0000", 0, "first"),
		chunk.NewChunk("chunk-0001", 1, "second"),
	})
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}
