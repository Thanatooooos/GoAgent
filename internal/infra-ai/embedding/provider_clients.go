package embedding

import (
	"net/http"

	aienum "local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

type OpenAIStyleEmbeddingClientOption func(*OpenAIStyleEmbeddingClient)

func WithAPIKeyRequired(required bool) OpenAIStyleEmbeddingClientOption {
	return func(client *OpenAIStyleEmbeddingClient) {
		if client != nil {
			client.requireAPIKey = required
		}
	}
}

func WithHeaderBuilder(builder func(target model.ModelTarget) http.Header) OpenAIStyleEmbeddingClientOption {
	return func(client *OpenAIStyleEmbeddingClient) {
		if client != nil && builder != nil {
			client.buildHeaders = builder
		}
	}
}

func WithBodyCustomizer(customizer func(body map[string]any, target model.ModelTarget)) OpenAIStyleEmbeddingClientOption {
	return func(client *OpenAIStyleEmbeddingClient) {
		if client != nil && customizer != nil {
			client.customizeBody = customizer
		}
	}
}

func WithMaxBatchSize(size int) OpenAIStyleEmbeddingClientOption {
	return func(client *OpenAIStyleEmbeddingClient) {
		if client != nil && size >= 0 {
			client.maxBatchSize = size
		}
	}
}

func NewSiliconFlowEmbeddingClient(httpClient *http.Client, opts ...OpenAIStyleEmbeddingClientOption) *OpenAIStyleEmbeddingClient {
	return newProviderEmbeddingClient(aienum.ModelProviderSiliconFlow.ID(), httpClient, opts...)
}

func NewOllamaEmbeddingClient(httpClient *http.Client, opts ...OpenAIStyleEmbeddingClientOption) *OpenAIStyleEmbeddingClient {
	options := append([]OpenAIStyleEmbeddingClientOption{WithAPIKeyRequired(false)}, opts...)
	return newProviderEmbeddingClient(aienum.ModelProviderOllama.ID(), httpClient, options...)
}

func NewDefaultOpenAIStyleEmbeddingClients(httpClient *http.Client, opts ...OpenAIStyleEmbeddingClientOption) []EmbeddingClient {
	return []EmbeddingClient{
		NewSiliconFlowEmbeddingClient(httpClient, opts...),
		NewOllamaEmbeddingClient(httpClient, opts...),
	}
}

func newProviderEmbeddingClient(provider string, httpClient *http.Client, opts ...OpenAIStyleEmbeddingClientOption) *OpenAIStyleEmbeddingClient {
	client := NewOpenAIStyleEmbeddingClient(provider, httpClient)
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}
