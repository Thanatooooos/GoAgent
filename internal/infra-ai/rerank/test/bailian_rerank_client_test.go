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

func TestBaiLianRerankClientReranksWhenTopNEqualsCandidateCount(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"results":[{"index":2,"relevance_score":0.9},{"index":1,"relevance_score":0.8},{"index":0,"relevance_score":0.7}]}}`))
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

	results, err := client.Rerank("query", candidates, 3, target)
	if err != nil {
		t.Fatalf("Rerank returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected rerank API call when topN equals candidate count, got %d calls", calls)
	}
	if len(results) != 3 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if results[0].ID != "c" || results[0].Score != 0.9 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
}

func TestBaiLianRerankClientSkipsWhenTopNZero(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
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

	results, err := client.Rerank("query", candidates, 0, target)
	if err != nil {
		t.Fatalf("Rerank returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected no rerank API call when topN is zero, got %d calls", calls)
	}
	if len(results) != 3 || results[0].ID != "a" || results[0].Score != 0.1 {
		t.Fatalf("expected original candidates unchanged: %+v", results)
	}
}
