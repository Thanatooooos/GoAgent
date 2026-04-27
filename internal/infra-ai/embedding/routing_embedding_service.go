package embedding

import (
	"fmt"
	"slices"

	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

type RoutingEmbeddingService struct {
	selector          *model.ModelSelector
	executor          *model.ModelRoutingExecutor
	clientsByProvider map[string]EmbeddingClient
}

func NewRoutingEmbeddingService(selector *model.ModelSelector, executor *model.ModelRoutingExecutor, clients []EmbeddingClient) *RoutingEmbeddingService {
	clientsByProvider := make(map[string]EmbeddingClient, len(clients))
	for _, client := range clients {
		if client != nil {
			clientsByProvider[client.Provider()] = client
		}
	}
	return &RoutingEmbeddingService{
		selector:          selector,
		executor:          executor,
		clientsByProvider: clientsByProvider,
	}
}

func (r *RoutingEmbeddingService) Embed(text string) ([]float32, error) {
	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityEmbedding,
		r.selector.SelectEmbeddingCandidates(),
		r.resolveClient,
		func(client EmbeddingClient, target model.ModelTarget) ([]float32, error) {
			return client.Embed(text, target)
		},
	)
}

func (r *RoutingEmbeddingService) EmbedWithModel(text string, modelID string) ([]float32, error) {
	target, err := r.resolveTarget(modelID)
	if err != nil {
		return nil, err
	}
	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityEmbedding,
		[]model.ModelTarget{target},
		r.resolveClient,
		func(client EmbeddingClient, target model.ModelTarget) ([]float32, error) {
			return client.Embed(text, target)
		},
	)
}

func (r *RoutingEmbeddingService) EmbedBatch(texts []string) ([][]float32, error) {
	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityEmbedding,
		r.selector.SelectEmbeddingCandidates(),
		r.resolveClient,
		func(client EmbeddingClient, target model.ModelTarget) ([][]float32, error) {
			return client.EmbedBatch(texts, target)
		},
	)
}

func (r *RoutingEmbeddingService) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	target, err := r.resolveTarget(modelID)
	if err != nil {
		return nil, err
	}
	return model.ExecuteWithFallback(
		r.executor,
		enum.ModelCapabilityEmbedding,
		[]model.ModelTarget{target},
		r.resolveClient,
		func(client EmbeddingClient, target model.ModelTarget) ([][]float32, error) {
			return client.EmbedBatch(texts, target)
		},
	)
}

func (r *RoutingEmbeddingService) Dimension() int {
	targets := r.selector.SelectEmbeddingCandidates()
	if len(targets) == 0 {
		return 0
	}
	return targets[0].Candidate.DimensionInt(0)
}

func (r *RoutingEmbeddingService) resolveClient(target model.ModelTarget) (EmbeddingClient, error) {
	client := r.clientsByProvider[target.Candidate.Provider]
	if client == nil {
		return nil, fmt.Errorf(errEmbeddingProviderMissingFmt, target.Candidate.Provider, target.Id)
	}
	return client, nil
}

func (r *RoutingEmbeddingService) resolveTarget(modelID string) (model.ModelTarget, error) {
	if modelID == "" {
		return model.ModelTarget{}, exception.NewRemoteException(errEmbeddingModelIDRequired, nil)
	}
	targets := r.selector.SelectEmbeddingCandidates()
	index := slices.IndexFunc(targets, func(target model.ModelTarget) bool {
		return target.Id == modelID
	})
	if index < 0 {
		return model.ModelTarget{}, exception.NewRemoteException(fmt.Sprintf(errEmbeddingModelUnavailable, modelID), nil)
	}
	return targets[index], nil
}
