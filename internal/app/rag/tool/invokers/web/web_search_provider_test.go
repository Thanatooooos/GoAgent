package builtin

import (
	"context"
	"errors"
	"testing"

	searchprovider "local/rag-project/internal/app/agent/search/provider"
	ragtool "local/rag-project/internal/app/rag/tool"
	inframcp "local/rag-project/internal/infra-mcp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubSearchProvider struct {
	name   string
	search func(query string) ([]searchprovider.SearchResult, error)
}

func (p stubSearchProvider) Search(query string) ([]searchprovider.SearchResult, error) {
	return p.search(query)
}

func (p stubSearchProvider) ProviderName() string {
	return p.name
}

type stubToolClient struct {
	callTool func(ctx context.Context, serverName string, toolName string, args map[string]any) (*mcp.CallToolResult, error)
}

func (c stubToolClient) ListTools(context.Context, string) ([]*mcp.Tool, error) {
	return nil, nil
}

func (c stubToolClient) CallTool(ctx context.Context, serverName string, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
	return c.callTool(ctx, serverName, toolName, args)
}

func (c stubToolClient) Close() error {
	return nil
}

func TestTavilyMCPProviderNormalizesStructuredResults(t *testing.T) {
	provider := searchprovider.NewTavilyMCPProvider(stubToolClient{
		callTool: func(_ context.Context, serverName string, toolName string, args map[string]any) (*mcp.CallToolResult, error) {
			if serverName != "tavily" || toolName != "tavily-search" {
				t.Fatalf("unexpected server/tool: %s %s", serverName, toolName)
			}
			if args["query"] != "golang generics" {
				t.Fatalf("unexpected query args: %+v", args)
			}
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{
					"results": []any{
						map[string]any{
							"title":   "Go Generics",
							"url":     "https://go.dev/doc/tutorial/generics",
							"content": "An introduction to generics in Go.",
							"score":   0.98,
						},
					},
				},
			}, nil
		},
	}, "tavily", "tavily-search")

	results, err := provider.Search("golang generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].Provider != "tavily-mcp" {
		t.Fatalf("expected provider tavily-mcp, got %q", results[0].Provider)
	}
	if results[0].URL != "https://go.dev/doc/tutorial/generics" {
		t.Fatalf("unexpected URL: %q", results[0].URL)
	}
	if results[0].ProviderScore == nil || *results[0].ProviderScore != 0.98 {
		t.Fatalf("unexpected provider score: %#v", results[0].ProviderScore)
	}
}

func TestTavilyMCPProviderRejectsMalformedResults(t *testing.T) {
	provider := searchprovider.NewTavilyMCPProvider(stubToolClient{
		callTool: func(context.Context, string, string, map[string]any) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				StructuredContent: map[string]any{"unexpected": true},
			}, nil
		},
	}, "tavily", "tavily-search")

	if _, err := provider.Search("golang generics"); err == nil {
		t.Fatal("expected malformed MCP response to fail")
	}
}

func TestFallbackSearchProviderFallsBackOnPrimaryError(t *testing.T) {
	provider := searchprovider.NewFallbackSearchProvider(
		"tavily-mcp",
		stubSearchProvider{
			name: "tavily-mcp",
			search: func(string) ([]searchprovider.SearchResult, error) {
				return nil, errors.New("mcp unavailable")
			},
		},
		stubSearchProvider{
			name: "tavily",
			search: func(string) ([]searchprovider.SearchResult, error) {
				return []searchprovider.SearchResult{{
					Title:    "Go Generics",
					URL:      "https://go.dev/doc/tutorial/generics",
					Snippet:  "An introduction to generics in Go.",
					Provider: "tavily",
				}}, nil
			},
		},
	)

	tool := NewWebSearchTool(provider)
	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name:      "web_search",
		Arguments: map[string]any{"query": "golang generics"},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if got := result.GetString("provider"); got != "tavily-mcp" {
		t.Fatalf("expected logical provider tavily-mcp, got %q", got)
	}
	if got := result.GetString("providerActual"); got != "tavily" {
		t.Fatalf("expected actual provider tavily, got %q", got)
	}
	if used, ok := result.Data["providerFallbackUsed"].(bool); !ok || !used {
		t.Fatal("expected fallback metadata to be true")
	}
}

func TestFallbackSearchProviderDoesNotFallbackOnEmptySuccess(t *testing.T) {
	secondaryCalled := false
	provider := searchprovider.NewFallbackSearchProvider(
		"tavily-mcp",
		stubSearchProvider{
			name: "tavily-mcp",
			search: func(string) ([]searchprovider.SearchResult, error) {
				return []searchprovider.SearchResult{}, nil
			},
		},
		stubSearchProvider{
			name: "tavily",
			search: func(string) ([]searchprovider.SearchResult, error) {
				secondaryCalled = true
				return nil, nil
			},
		},
	)

	results, err := provider.Search("golang generics")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if secondaryCalled {
		t.Fatal("expected secondary provider to remain unused on empty success")
	}
	if len(results) != 0 {
		t.Fatalf("expected empty primary results, got %+v", results)
	}
}

var _ inframcp.ToolClient = (*stubToolClient)(nil)
