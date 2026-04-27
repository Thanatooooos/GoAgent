package chat

import (
	"net/http"

	"local/rag-project/internal/framework/convention"
	aienum "local/rag-project/internal/infra-ai/enum"
	"local/rag-project/internal/infra-ai/model"
)

type OpenAIStyleChatClientOption func(*OpenAIStyleChatClient)

func WithStreamHTTPClient(streamClient *http.Client) OpenAIStyleChatClientOption {
	return func(client *OpenAIStyleChatClient) {
		if client != nil && streamClient != nil {
			client.streamClient = streamClient
		}
	}
}

func WithAPIKeyRequired(required bool) OpenAIStyleChatClientOption {
	return func(client *OpenAIStyleChatClient) {
		if client != nil {
			client.requireAPIKey = required
		}
	}
}

func WithHeaderBuilder(builder func(target model.ModelTarget) http.Header) OpenAIStyleChatClientOption {
	return func(client *OpenAIStyleChatClient) {
		if client != nil && builder != nil {
			client.buildHeaders = builder
		}
	}
}

func WithBodyCustomizer(customizer func(body map[string]any, req convention.ChatRequest, target model.ModelTarget)) OpenAIStyleChatClientOption {
	return func(client *OpenAIStyleChatClient) {
		if client != nil && customizer != nil {
			client.customizeBody = customizer
		}
	}
}

func WithStreamParser(parser func(line string, reasoningEnabled bool) (ParsedEvent, error)) OpenAIStyleChatClientOption {
	return func(client *OpenAIStyleChatClient) {
		if client != nil && parser != nil {
			client.parseStream = parser
		}
	}
}

func NewBaiLianChatClient(httpClient *http.Client, opts ...OpenAIStyleChatClientOption) *OpenAIStyleChatClient {
	return newProviderChatClient(aienum.ModelProviderBaiLian.ID(), httpClient, opts...)
}

func NewSiliconFlowChatClient(httpClient *http.Client, opts ...OpenAIStyleChatClientOption) *OpenAIStyleChatClient {
	return newProviderChatClient(aienum.ModelProviderSiliconFlow.ID(), httpClient, opts...)
}

func NewOllamaChatClient(httpClient *http.Client, opts ...OpenAIStyleChatClientOption) *OpenAIStyleChatClient {
	options := append([]OpenAIStyleChatClientOption{WithAPIKeyRequired(false)}, opts...)
	return newProviderChatClient(aienum.ModelProviderOllama.ID(), httpClient, options...)
}

func NewDefaultOpenAIStyleChatClients(httpClient *http.Client, opts ...OpenAIStyleChatClientOption) []ChatClient {
	return []ChatClient{
		NewBaiLianChatClient(httpClient, opts...),
		NewSiliconFlowChatClient(httpClient, opts...),
		NewOllamaChatClient(httpClient, opts...),
	}
}

func newProviderChatClient(provider string, httpClient *http.Client, opts ...OpenAIStyleChatClientOption) *OpenAIStyleChatClient {
	client := NewOpenAIStyleChatClient(provider, httpClient)
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
}
