package provider

import (
	"strings"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	inframcp "local/rag-project/internal/infra-mcp"
)

func BuildProvider(cfg *config.Config, mcpManager inframcp.ToolClient) SearchProvider {
	if cfg == nil {
		return NewDuckDuckGoProvider()
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Rag.Search.WebSearch.Provider))
	fallbackProvider := strings.ToLower(strings.TrimSpace(cfg.Rag.Search.WebSearch.FallbackProvider))
	apiKey := strings.TrimSpace(cfg.Rag.Search.WebSearch.ApiKey)

	switch provider {
	case "tavily-mcp":
		serverName := strings.TrimSpace(cfg.Rag.Search.WebSearch.MCP.Server)
		if serverName == "" {
			serverName = "tavily"
		}
		toolName := strings.TrimSpace(cfg.Rag.Search.WebSearch.MCP.SearchTool)
		if toolName == "" {
			toolName = defaultTavilyMCPToolName
		}
		primary := NewTavilyMCPProvider(mcpManager, serverName, toolName)
		secondary := fallbackProviderByName(fallbackProvider, apiKey)
		if secondary == nil {
			log.Warnf("rag.search.web-search.provider=tavily-mcp but fallback-provider is unavailable, MCP failures will surface directly")
			return primary
		}
		return NewFallbackSearchProvider("tavily-mcp", primary, secondary)
	case "tavily":
		if apiKey == "" {
			log.Warnf("rag.search.web-search.provider=tavily but api-key is empty, falling back to duckduckgo")
			return NewDuckDuckGoProvider()
		}
		return NewTavilyProvider(apiKey)
	default:
		return NewDuckDuckGoProvider()
	}
}

func BuildSourcePolicy(cfg *config.Config) *SourcePolicyEngine {
	if cfg == nil {
		return NewSourcePolicyEngine(SourcePolicyConfig{})
	}
	policy := cfg.Rag.Search.WebSearch.SourcePolicy
	return NewSourcePolicyEngine(SourcePolicyConfig{
		AllowDomains:  policy.AllowDomains,
		DenyDomains:   policy.DenyDomains,
		AllowSuffixes: policy.AllowSuffixes,
		DenySuffixes:  policy.DenySuffixes,
	})
}

func fallbackProviderByName(name string, tavilyAPIKey string) SearchProvider {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return nil
	case "duckduckgo":
		return NewDuckDuckGoProvider()
	case "tavily":
		if strings.TrimSpace(tavilyAPIKey) == "" {
			log.Warnf("rag.search.web-search.fallback-provider=tavily but api-key is empty, skipping Tavily fallback")
			return nil
		}
		return NewTavilyProvider(tavilyAPIKey)
	default:
		log.Warnf("unsupported rag.search.web-search.fallback-provider=%q, skipping fallback", name)
		return nil
	}
}
