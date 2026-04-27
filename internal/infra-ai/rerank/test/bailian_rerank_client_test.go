package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/model"
	"local/rag-project/internal/infra-ai/rerank"
)

func TestBaiLianRerankClientRerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"results":[{"index":1,"relevance_score":0.9},{"index":0,"relevance_score":0.8}]}}`))
	}))
	defer srv.Close()

	client := rerank.NewBaiLianRerankClient(srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-rerank"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"rerank": "/rerank"},
		},
	}
	candidates := []convention.RetrievedChunk{
		{ID: "a", Text: "doc a", Score: 0.1},
		{ID: "b", Text: "doc b", Score: 0.2},
		{ID: "c", Text: "doc c", Score: 0.3},
	}

	results, err := client.Rerank("query", candidates, 2, target)
	if err != nil {
		t.Fatalf("Rerank returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if results[0].ID != "b" || results[0].Score != 0.9 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
}
