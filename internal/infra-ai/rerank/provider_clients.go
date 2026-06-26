package rerank

import (
	"net/http"

	aienum "local/rag-project/internal/infra-ai/enum"
)

func NewSiliconFlowRerankClient(httpClient *http.Client) *OpenAIStyleRerankClient {
	return NewOpenAIStyleRerankClient(aienum.ModelProviderSiliconFlow.ID(), httpClient)
}

func NewDefaultRerankClients(httpClient *http.Client) []RerankClient {
	return []RerankClient{
		NewBaiLianRerankClient(httpClient),
		NewSiliconFlowRerankClient(httpClient),
		NewNoopRerankClient(),
	}
}
