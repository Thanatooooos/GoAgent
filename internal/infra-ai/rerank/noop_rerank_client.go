package rerank

import (
	"local/rag-project/internal/framework/convention"
	aienum "local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

type NoopRerankClient struct{}

func NewNoopRerankClient() *NoopRerankClient {
	return &NoopRerankClient{}
}

func (n *NoopRerankClient) Provider() string {
	return aienum.ModelProviderNoop.ID()
}

func (n *NoopRerankClient) Rerank(query string, candidates []convention.RetrievedChunk, topN int, target model.ModelTarget) ([]convention.RetrievedChunk, error) {
	if len(candidates) == 0 {
		return []convention.RetrievedChunk{}, nil
	}
	if topN <= 0 || len(candidates) <= topN {
		return candidates, nil
	}
	return candidates[:topN], nil
}
