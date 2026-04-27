package test

import (
	"net/http"
	"testing"

	"local/rag-project/internal/infra-ai/embedding"
)

func TestEmbeddingProviderConstructors(t *testing.T) {
	httpClient := &http.Client{}

	siliconflow := embedding.NewSiliconFlowEmbeddingClient(httpClient)
	if siliconflow.Provider() != "siliconflow" {
		t.Fatalf("unexpected provider: %s", siliconflow.Provider())
	}

	ollama := embedding.NewOllamaEmbeddingClient(httpClient)
	if ollama.Provider() != "ollama" {
		t.Fatalf("unexpected provider: %s", ollama.Provider())
	}

	all := embedding.NewDefaultOpenAIStyleEmbeddingClients(httpClient)
	if len(all) != 2 {
		t.Fatalf("unexpected client count: %d", len(all))
	}
}
