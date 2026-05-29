package provider

import "strings"

// SearchResult is a single web search result returned by any provider.
type SearchResult struct {
	Title          string
	URL            string
	Snippet        string
	Domain         string
	Provider       string
	ActualProvider string
	FallbackUsed   bool
	ProviderScore  *float64
	SourceType     string
	Policy         string
	RiskFlags      []string
	Reasons        []string
}

// SearchProvider abstracts an external web search backend.
type SearchProvider interface {
	Search(query string) ([]SearchResult, error)
}

// ProviderName returns the configured provider name for logging and payloads.
func ProviderName(provider SearchProvider) string {
	if named, ok := provider.(interface{ ProviderName() string }); ok {
		if name := strings.TrimSpace(named.ProviderName()); name != "" {
			return name
		}
	}
	switch provider.(type) {
	case *TavilyProvider:
		return "tavily"
	case *TavilyMCPProvider:
		return "tavily-mcp"
	case *DuckDuckGoProvider:
		return "duckduckgo"
	case *FallbackSearchProvider:
		return provider.(*FallbackSearchProvider).ProviderName()
	default:
		return "unknown"
	}
}

func truncateText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return strings.TrimSpace(text[:max-3]) + "..."
}
