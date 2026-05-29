package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
)

type stubProvider struct {
	name   string
	search func(query string) ([]searchprovider.SearchResult, error)
}

func (p stubProvider) Search(query string) ([]searchprovider.SearchResult, error) {
	return p.search(query)
}

func (p stubProvider) ProviderName() string {
	return p.name
}

func TestToolInvokesSearchService(t *testing.T) {
	service := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "golang generics" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{{
				Title:   "Go Docs",
				URL:     "https://go.dev/doc/tutorial/generics",
				Snippet: "Generics in Go.",
			}}, nil
		},
	}, nil)

	tool, err := New(service)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := tool.Invokable().InvokableRun(context.Background(), `{"query":"golang generics"}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}

	var decoded agentsearch.SearchOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if decoded.Query != "golang generics" || decoded.ResultCount != 1 {
		t.Fatalf("unexpected output: %+v", decoded)
	}
}

func TestToolEncodesProviderFailureAsDegradedOutput(t *testing.T) {
	service := agentsearch.NewService(stubProvider{
		name: "stub",
		search: func(string) ([]searchprovider.SearchResult, error) {
			return nil, errors.New("provider unavailable")
		},
	}, nil)

	tool, err := New(service)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := tool.Invokable().InvokableRun(context.Background(), `{"query":"golang generics"}`)
	if err != nil {
		t.Fatalf("InvokableRun() unexpected error = %v", err)
	}

	var decoded agentsearch.SearchOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !decoded.Degraded || decoded.DegradeReason != "provider unavailable" {
		t.Fatalf("unexpected degraded output: %+v", decoded)
	}
}
