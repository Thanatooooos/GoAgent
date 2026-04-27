package rerank

import "net/http"

func NewDefaultRerankClients(httpClient *http.Client) []RerankClient {
	return []RerankClient{
		NewBaiLianRerankClient(httpClient),
		NewNoopRerankClient(),
	}
}
