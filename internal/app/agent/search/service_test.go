package search

import (
	"context"
	"errors"
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

func TestServiceSearchNormalizesResultsAndPolicies(t *testing.T) {
	svc := NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "golang generics" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{Title: "Go Docs", URL: "https://go.dev/doc/tutorial/generics", Snippet: "Generics in Go."},
				{Title: "Quora", URL: "https://www.quora.com/What-is-Go-generics", Snippet: "User answer."},
			}, nil
		},
	}, searchprovider.NewSourcePolicyEngine(searchprovider.SourcePolicyConfig{
		AllowDomains: []string{"go.dev"},
		DenyDomains:  []string{"quora.com"},
	}))

	output, err := svc.Search(context.Background(), "golang generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if output.ResultCount != 2 {
		t.Fatalf("expected 2 results, got %d", output.ResultCount)
	}
	if output.AllowedCount != 1 || output.DeniedCount != 1 {
		t.Fatalf("unexpected policy counts: %+v", output)
	}
	if len(output.URLs) != 1 || output.URLs[0] != "https://go.dev/doc/tutorial/generics" {
		t.Fatalf("unexpected fetchable URLs: %+v", output.URLs)
	}
	if output.Results[0].Policy != searchprovider.SourcePolicyAllow {
		t.Fatalf("expected first result allow policy, got %+v", output.Results[0])
	}
	if output.Results[1].Policy != searchprovider.SourcePolicyDeny {
		t.Fatalf("expected second result deny policy, got %+v", output.Results[1])
	}
}

func TestServiceSearchRejectsEmptyQuery(t *testing.T) {
	svc := NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return nil, nil
		},
	}, nil)

	output, err := svc.Search(context.Background(), "   ")
	if err == nil {
		t.Fatal("expected empty query to fail")
	}
	if !output.Degraded || output.ErrorMessage == "" {
		t.Fatalf("expected degraded output, got %+v", output)
	}
}

func TestServiceSearchReturnsDegradedOutputOnProviderError(t *testing.T) {
	svc := NewService(stubProvider{
		name: "stub",
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return nil, errors.New("provider unavailable")
		},
	}, nil)

	output, err := svc.Search(context.Background(), "golang generics")
	if err == nil {
		t.Fatal("expected provider error")
	}
	if !output.Degraded {
		t.Fatalf("expected degraded output, got %+v", output)
	}
	if output.DegradeReason != "provider unavailable" {
		t.Fatalf("unexpected degrade reason: %+v", output)
	}
}

func TestServiceSearchCapturesFallbackMetadata(t *testing.T) {
	fallback := searchprovider.NewFallbackSearchProvider(
		"tavily-mcp",
		stubProvider{
			name: "tavily-mcp",
			search: func(string) ([]searchprovider.SearchResult, error) {
				return nil, errors.New("mcp unavailable")
			},
		},
		stubProvider{
			name: "tavily",
			search: func(string) ([]searchprovider.SearchResult, error) {
				return []searchprovider.SearchResult{{
					Title:    "Go Docs",
					URL:      "https://go.dev/doc/tutorial/generics",
					Snippet:  "Generics in Go.",
					Provider: "tavily",
				}}, nil
			},
		},
	)
	svc := NewService(fallback, nil)

	output, err := svc.Search(context.Background(), "golang generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !output.ProviderFallbackUsed || output.ProviderActual != "tavily" {
		t.Fatalf("unexpected fallback metadata: %+v", output)
	}
}
