package evaluation

import (
	"testing"

	"local/rag-project/internal/infra-ai/embedding"
)

type recordingEmbeddingService struct {
	calls    []string
	modelIDs []string
}

func (s *recordingEmbeddingService) Embed(text string) ([]float32, error) {
	s.calls = append(s.calls, text)
	s.modelIDs = append(s.modelIDs, "")
	return []float32{1, 0}, nil
}

func (s *recordingEmbeddingService) EmbedWithModel(text string, modelID string) ([]float32, error) {
	s.calls = append(s.calls, text)
	s.modelIDs = append(s.modelIDs, modelID)
	return []float32{1, 0}, nil
}

func (s *recordingEmbeddingService) EmbedBatch(texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, err := s.Embed(text)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (s *recordingEmbeddingService) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector, err := s.EmbedWithModel(text, modelID)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (s *recordingEmbeddingService) Dimension() int { return 2 }

var _ embedding.EmbeddingService = (*recordingEmbeddingService)(nil)

func TestModelPinnedQueryEmbedderUsesConfiguredModel(t *testing.T) {
	service := &recordingEmbeddingService{}
	embedder := NewModelPinnedQueryEmbedder(service, "qwen-emb-8b")
	if _, err := embedder.EmbedBatch([]string{"hello", "world"}); err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(service.modelIDs) != 2 || service.modelIDs[0] != "qwen-emb-8b" || service.modelIDs[1] != "qwen-emb-8b" {
		t.Fatalf("modelIDs = %v, want qwen-emb-8b for each call", service.modelIDs)
	}
}
