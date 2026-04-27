package rerank

import (
	"fmt"

	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

type RoutingRerankService struct {
	selector          *model.ModelSelector
	executor          *model.ModelRoutingExecutor
	clientsByProvider map[string]RerankClient
}

func NewRoutingRerankService(selector *model.ModelSelector, executor *model.ModelRoutingExecutor, clients []RerankClient) *RoutingRerankService {
	clientsByProvider := make(map[string]RerankClient, len(clients))
	for _, client := range clients {
		if client != nil {
			clientsByProvider[client.Provider()] = client
		}
	}
	return &RoutingRerankService{
		selector:          selector,
		executor:          executor,
		clientsByProvider: clientsByProvider,
	}
}

func (r *RoutingRerankService) Rerank(query string, candidates []convention.RetrievedChunk, topN int) ([]convention.RetrievedChunk, error) {
	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityRerank,
		r.selector.SelectRerankCandidates(),
		r.resolveClient,
		func(client RerankClient, target model.ModelTarget) ([]convention.RetrievedChunk, error) {
			return client.Rerank(query, candidates, topN, target)
		},
	)
}

func (r *RoutingRerankService) resolveClient(target model.ModelTarget) (RerankClient, error) {
	client := r.clientsByProvider[target.Candidate.Provider]
	if client == nil {
		return nil, fmt.Errorf(errRerankProviderMissingFmt, target.Candidate.Provider, target.Id)
	}
	return client, nil
}
