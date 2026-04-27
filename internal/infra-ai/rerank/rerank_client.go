package rerank

import (
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/model"
)

type RerankClient interface {
	Provider() string

	Rerank(query string, candidates []convention.RetrievedChunk, topN int, target model.ModelTarget) ([]convention.RetrievedChunk, error)
}
