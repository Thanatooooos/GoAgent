package evaluation

import (
	"fmt"
	"strings"

	"local/rag-project/internal/infra-ai/embedding"
)

// ModelPinnedQueryEmbedder calls EmbedWithModel so offline eval uses the
// configured primary embedding candidate (e.g. siliconflow qwen-emb-8b)
// instead of falling back to local providers such as ollama.
type ModelPinnedQueryEmbedder struct {
	embedding embedding.EmbeddingService
	modelID   string
}

func NewModelPinnedQueryEmbedder(service embedding.EmbeddingService, modelID string) QueryEmbedder {
	return &ModelPinnedQueryEmbedder{
		embedding: service,
		modelID:   strings.TrimSpace(modelID),
	}
}

func (e *ModelPinnedQueryEmbedder) Embed(text string) ([]float32, error) {
	if e == nil || e.embedding == nil {
		return nil, fmt.Errorf("embedding service is required")
	}
	if e.modelID != "" {
		return e.embedding.EmbedWithModel(text, e.modelID)
	}
	return e.embedding.Embed(text)
}

func (e *ModelPinnedQueryEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	if e == nil || e.embedding == nil {
		return nil, fmt.Errorf("embedding service is required")
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("texts are required")
	}
	if e.modelID != "" {
		return e.embedding.EmbedBatchWithModel(texts, e.modelID)
	}
	return e.embedding.EmbedBatch(texts)
}
