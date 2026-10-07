package capability

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/citation"
	"local/rag-project/internal/app/runtime/websource"
)

func TestWebFetchDeniesConfiguredSourceBeforeExecution(t *testing.T) {
	def := WebFetch(webFetchStub{}, websource.New(websource.Config{DenyDomains: []string{"blocked.example"}}))
	_, err := def.Describe(Value(`{"urls":["https://blocked.example/page"]}`), Context{Context: context.Background(), AllowWebSearch: true})
	if !IsDenied(err) {
		t.Fatalf("describe error = %v, want denied", err)
	}
}

func TestTaskWebDomainScopeFiltersSearchAndFetch(t *testing.T) {
	ctx := Context{Context: context.Background(), AllowWebSearch: true, AllowedWebDomains: []string{"example.com"}, Citations: citation.NewRegistry()}
	search := WebSearch(scopedSearchStub{})
	result, err := search.Execute(Value(`{"query":"release"}`), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, "unapproved.test") || !strings.Contains(result.Content, "example.com") {
		t.Fatalf("search scope result = %s", result.Content)
	}
	fetch := WebFetch(webFetchStub{})
	if _, err := fetch.Describe(Value(`{"urls":["https://unapproved.test/page"]}`), ctx); !IsDenied(err) {
		t.Fatalf("fetch outside task scope = %v, want denied", err)
	}
	if _, err := fetch.Describe(Value(`{"urls":["https://news.example.com/page"]}`), ctx); err != nil {
		t.Fatalf("fetch inside task scope = %v", err)
	}
}

func TestWebToolsProjectRegisteredCitationHandles(t *testing.T) {
	registry := citation.NewRegistry()
	search := WebSearch(webSearchStub{})
	searchResult, err := search.Execute(Value(`{"query":"release"}`), Context{Context: context.Background(), Citations: registry})
	if err != nil {
		t.Fatalf("search execute: %v", err)
	}
	if !strings.Contains(searchResult.Content, `"citation_id":"w1"`) {
		t.Fatalf("search context missing citation handle: %s", searchResult.Content)
	}
	if ref, ok := registry.ResolveWeb("w1"); !ok || ref.URL != "https://example.com/release" || ref.Title != "Release notes" {
		t.Fatalf("registered search reference = %#v, %v", ref, ok)
	}

	fetch := WebFetch(webFetchResultStub{})
	fetchResult, err := fetch.Execute(Value(`{"urls":["https://example.com/release"]}`), Context{Context: context.Background(), Citations: registry})
	if err != nil {
		t.Fatalf("fetch execute: %v", err)
	}
	if !strings.Contains(fetchResult.Content, `"citation_id":"w1"`) {
		t.Fatalf("fetch context did not reuse citation handle: %s", fetchResult.Content)
	}
}

type webFetchStub struct{}

func (webFetchStub) Fetch(context.Context, []string) ([]WebPage, error) { return nil, nil }

type webSearchStub struct{}

func (webSearchStub) Search(context.Context, string) ([]WebSearchResult, error) {
	return []WebSearchResult{{Title: "Release notes", URL: "https://example.com/release", Snippet: "New release."}}, nil
}

type scopedSearchStub struct{}

func (scopedSearchStub) Search(context.Context, string) ([]WebSearchResult, error) {
	return []WebSearchResult{
		{Title: "Official", URL: "https://news.example.com/page"},
		{Title: "Other", URL: "https://unapproved.test/page"},
	}, nil
}

type webFetchResultStub struct{}

func (webFetchResultStub) Fetch(context.Context, []string) ([]WebPage, error) {
	return []WebPage{{URL: "https://example.com/release", Text: "Release details."}}, nil
}
