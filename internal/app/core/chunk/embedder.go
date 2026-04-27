package chunk

import (
	"fmt"

	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type Embedder struct {
	service aiembedding.EmbeddingService
}

func NewEmbedder(service aiembedding.EmbeddingService) *Embedder {
	return &Embedder{service: service}
}

func (e *Embedder) AttachEmbeddings(chunks []Chunk) ([]Chunk, error) {
	return e.attach(chunks, "")
}

func (e *Embedder) AttachEmbeddingsWithModel(chunks []Chunk, modelID string) ([]Chunk, error) {
	return e.attach(chunks, modelID)
}

func (e *Embedder) attach(chunks []Chunk, modelID string) ([]Chunk, error) {
	if len(chunks) == 0 {
		return []Chunk{}, nil
	}
	if e == nil || e.service == nil {
		return nil, fmt.Errorf("chunk embedder service is nil")
	}

	result := make([]Chunk, len(chunks))
	copy(result, chunks)
	texts := make([]string, 0, len(chunks))
	for i, each := range result {
		if each.Metadata == nil {
			result[i].Metadata = map[string]any{}
		}
		texts = append(texts, each.Text)
	}

	var (
		vectors [][]float32
		err     error
	)
	if modelID == "" {
		vectors, err = e.service.EmbedBatch(texts)
	} else {
		vectors, err = e.service.EmbedBatchWithModel(texts, modelID)
	}
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(result) {
		return nil, fmt.Errorf("embedding result size mismatch: chunks=%d vectors=%d", len(result), len(vectors))
	}

	for i := range result {
		result[i].Embedding = vectors[i]
	}
	return result, nil
}
