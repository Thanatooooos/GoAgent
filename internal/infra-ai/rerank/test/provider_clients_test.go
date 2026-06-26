package test

import (
	"net/http"
	"testing"

	"local/rag-project/internal/infra-ai/rerank"
)

func TestRerankProviderConstructors(t *testing.T) {
	clients := rerank.NewDefaultRerankClients(&http.Client{})
	if len(clients) != 3 {
		t.Fatalf("unexpected client count: %d", len(clients))
	}
	if clients[0].Provider() != "bailian" {
		t.Fatalf("unexpected first provider: %s", clients[0].Provider())
	}
	if clients[1].Provider() != "siliconflow" {
		t.Fatalf("unexpected second provider: %s", clients[1].Provider())
	}
	if clients[2].Provider() != "noop" {
		t.Fatalf("unexpected third provider: %s", clients[2].Provider())
	}
}
