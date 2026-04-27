package rerank

import "local/rag-project/internal/framework/convention"

type RerankService interface {
	Rerank(query string, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error)
}
