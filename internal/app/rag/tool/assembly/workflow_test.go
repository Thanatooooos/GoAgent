package assembly

import (
	"testing"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	raginvweb "local/rag-project/internal/app/rag/tool/invokers/web"
	"local/rag-project/internal/framework/config"
)

func TestRegisterGraphToolsSkipsInvalidGraphsWithoutPanic(t *testing.T) {
	registry := ragcore.NewRegistry()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("registerGraphTools should not panic, got: %v", recovered)
		}
	}()

	registerGraphTools(registry, nil, nil)

	if len(registry.ListDefinitions()) != 0 {
		t.Fatalf("expected no graph tools to be registered when executor is nil")
	}
}

func TestBuildSearchProviderBuildsTavilyMCPFallbackChain(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Search.WebSearch.Provider = "tavily-mcp"
	cfg.Rag.Search.WebSearch.FallbackProvider = "tavily"
	cfg.Rag.Search.WebSearch.ApiKey = "test-key"

	provider := buildSearchProvider(cfg, nil)
	fallback, ok := provider.(*raginvweb.FallbackSearchProvider)
	if !ok {
		t.Fatalf("expected FallbackSearchProvider, got %T", provider)
	}
	if fallback.ProviderName() != "tavily-mcp" {
		t.Fatalf("unexpected provider name: %q", fallback.ProviderName())
	}
	if _, ok := fallback.Primary.(*raginvweb.TavilyMCPProvider); !ok {
		t.Fatalf("expected TavilyMCPProvider primary, got %T", fallback.Primary)
	}
	if _, ok := fallback.Secondary.(*raginvweb.TavilyProvider); !ok {
		t.Fatalf("expected TavilyProvider secondary, got %T", fallback.Secondary)
	}
}
