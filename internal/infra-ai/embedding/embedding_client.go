package embedding

import "local/rag-project/internal/infra-ai/model"

type EmbeddingClient interface {
	Provider() string

	Embed(text string, target model.ModelTarget) ([]float32, error)

	EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error)
}
