package provider

import (
	"testing"

	"local/rag-project/internal/framework/config"
)

func TestBuildProviderUsesPublicFallbackWhenTavilyFallbackHasNoAPIKey(t *testing.T) {
	provider := BuildProvider(&config.Config{
		Rag: config.RagConfig{
			Search: config.RagSearchConfig{
				WebSearch: config.RagWebSearchConfig{
					Provider:         "tavily-mcp",
					FallbackProvider: "tavily",
				},
			},
		},
	}, nil)

	fallback, ok := provider.(*FallbackSearchProvider)
	if !ok {
		t.Fatalf("expected fallback provider, got %T", provider)
	}
	if _, ok := fallback.Secondary.(*DuckDuckGoProvider); !ok {
		t.Fatalf("expected duckduckgo as no-key public fallback, got %T", fallback.Secondary)
	}
}
