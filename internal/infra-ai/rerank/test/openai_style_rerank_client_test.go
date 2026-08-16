package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/model"
	"local/rag-project/internal/infra-ai/rerank"
)

func TestSiliconFlowRerankClientRerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/rerank" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if payload["model"] != "Qwen/Qwen3-Reranker-8B" {
			t.Fatalf("unexpected model: %#v", payload["model"])
		}
		if payload["query"] != "query" {
			t.Fatalf("unexpected query: %#v", payload["query"])
		}
		documents, ok := payload["documents"].([]any)
		if !ok || len(documents) != 3 {
			t.Fatalf("unexpected documents: %#v", payload["documents"])
		}
		if payload["top_n"] != float64(2) {
			t.Fatalf("unexpected top_n: %#v", payload["top_n"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":2,"relevance_score":0.95},{"index":1,"relevance_score":0.88}]}`))
	}))
	defer srv.Close()

	client := rerank.NewSiliconFlowRerankClient(srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "Qwen/Qwen3-Reranker-8B"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"rerank": "/v1/rerank"},
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
	if results[0].ID != "c" || results[0].Score != 0.95 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].ID != "b" || results[1].Score != 0.88 {
		t.Fatalf("unexpected second result: %+v", results[1])
	}
}

func TestSiliconFlowRerankClientReranksWhenTopNEqualsCandidateCount(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":2,"relevance_score":0.95},{"index":1,"relevance_score":0.88},{"index":0,"relevance_score":0.7}]}`))
	}))
	defer srv.Close()

	client := rerank.NewSiliconFlowRerankClient(srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "Qwen/Qwen3-Reranker-8B"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"rerank": "/v1/rerank"},
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
	if results[0].ID != "c" || results[0].Score != 0.95 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[2].ID != "a" || results[2].Score != 0.7 {
		t.Fatalf("unexpected third result: %+v", results[2])
	}
}

func TestSiliconFlowRerankClientSkipsWhenTopNZero(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
	}))
	defer srv.Close()

	client := rerank.NewSiliconFlowRerankClient(srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "Qwen/Qwen3-Reranker-8B"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"rerank": "/v1/rerank"},
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

func TestSiliconFlowRerankClientClampsTopNAboveCandidateCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.9},{"index":1,"relevance_score":0.8},{"index":2,"relevance_score":0.7}]}`))
	}))
	defer srv.Close()

	client := rerank.NewSiliconFlowRerankClient(srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "Qwen/Qwen3-Reranker-8B"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"rerank": "/v1/rerank"},
		},
	}
	candidates := []convention.RetrievedChunk{
		{ID: "a", Text: "doc a", Score: 0.1},
		{ID: "b", Text: "doc b", Score: 0.2},
		{ID: "c", Text: "doc c", Score: 0.3},
	}

	results, err := client.Rerank("query", candidates, 10, target)
	if err != nil {
		t.Fatalf("Rerank returned error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("unexpected result count: %d", len(results))
	}
	if results[0].ID != "a" || results[0].Score != 0.9 {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
}
