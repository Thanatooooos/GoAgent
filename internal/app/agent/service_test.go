package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestServiceRunReturnsFetchedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>Generics allow reusable code with type parameters.</body></html>`))
	}))
	defer server.Close()

	service, err := NewService(ServiceOptions{
		HTTPClient: server.Client(),
		Provider: stubProvider{
			name: "stub",
			search: func(query string) ([]searchprovider.SearchResult, error) {
				if query != "Go generics tutorial" {
					t.Fatalf("unexpected query: %q", query)
				}
				return []searchprovider.SearchResult{{
					Title:   "Go Docs",
					URL:     server.URL,
					Snippet: "Generics in Go.",
				}}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	resp, err := service.Run(context.Background(), Request{Question: "Go generics tutorial"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Query != "Go generics tutorial" {
		t.Fatalf("unexpected query: %+v", resp)
	}
	if len(resp.Results) != 1 || resp.Provider != "stub" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Pages) != 1 || resp.CombinedText == "" {
		t.Fatalf("expected fetched page content, got %+v", resp)
	}
}
