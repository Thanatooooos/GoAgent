package builtin

import (
	"context"
	"fmt"
	"strings"

	searchprovider "local/rag-project/internal/app/agent/search/provider"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

// WebSearchTool performs an external web search via a configurable SearchProvider.
type WebSearchTool struct {
	provider     searchprovider.SearchProvider
	sourcePolicy *searchprovider.SourcePolicyEngine
}

func NewWebSearchTool(provider searchprovider.SearchProvider, policy ...*searchprovider.SourcePolicyEngine) *WebSearchTool {
	var engine *searchprovider.SourcePolicyEngine
	if len(policy) > 0 {
		engine = policy[0]
	}
	return &WebSearchTool{provider: provider, sourcePolicy: engine}
}

func (t *WebSearchTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "web_search",
		Description: "Search the web for information beyond the local knowledge base. Use this when the user asks about general concepts, troubleshooting steps, or anything not covered by local documents. Returns up to 5 relevant results with titles, URLs, and snippets.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "query",
				Type:        ragtool.ParamTypeString,
				Description: "The search query. Keep it concise and specific, e.g. 'vector database connection refused troubleshooting'.",
				Required:    true,
			},
		},
	}
}

func (t *WebSearchTool) Invoke(_ context.Context, call ragtool.Call) (ragtool.Result, error) {
	query := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "query"))
	if query == "" {
		return ragtool.Result{Name: "web_search", Status: ragtool.CallStatusFailed, ErrorMessage: "query is required"}, nil
	}

	if t.provider == nil {
		return ragtool.Result{Name: "web_search", Status: ragtool.CallStatusFailed, ErrorMessage: "no search provider configured"}, nil
	}

	results, err := t.provider.Search(query)
	if err != nil {
		return ragtool.Result{
			Name:         "web_search",
			Status:       ragtool.CallStatusFailed,
			Summary:      fmt.Sprintf("web search failed: %v", err),
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(results) == 0 {
		return ragtool.Result{
			Name:    "web_search",
			Status:  ragtool.CallStatusSuccess,
			Summary: fmt.Sprintf("no results found for query %q", query),
			Data: map[string]any{
				"query":   query,
				"results": []map[string]any{},
			},
		}, nil
	}

	items := make([]map[string]any, 0, len(results))
	fetchableURLs := make([]string, 0, len(results))
	providerName := searchProviderName(t.provider)
	actualProvider := ""
	fallbackUsed := false
	allowedCount := 0
	neutralCount := 0
	deniedCount := 0
	for _, r := range results {
		result := t.enrichResult(r, providerName)
		if strings.TrimSpace(result.ActualProvider) != "" && actualProvider == "" {
			actualProvider = strings.TrimSpace(result.ActualProvider)
		}
		fallbackUsed = fallbackUsed || result.FallbackUsed
		if result.Policy == searchprovider.SourcePolicyDeny {
			deniedCount++
		} else if result.Policy == searchprovider.SourcePolicyAllow {
			allowedCount++
		} else {
			neutralCount++
		}
		if result.Policy != searchprovider.SourcePolicyDeny && strings.TrimSpace(result.URL) != "" {
			fetchableURLs = append(fetchableURLs, strings.TrimSpace(result.URL))
		}
		items = append(items, map[string]any{
			"title":         result.Title,
			"url":           result.URL,
			"snippet":       result.Snippet,
			"domain":        result.Domain,
			"provider":      result.Provider,
			"providerScore": derefFloat64(result.ProviderScore),
			"sourceType":    result.SourceType,
			"policy":        result.Policy,
			"riskFlags":     result.RiskFlags,
			"reasons":       result.Reasons,
		})
	}

	return ragtool.Result{
		Name:   "web_search",
		Status: ragtool.CallStatusSuccess,
		Summary: fmt.Sprintf(
			"found %d web results for %q (allow=%d, neutral=%d, deny=%d)",
			len(results), query, allowedCount, neutralCount, deniedCount,
		),
		Data: map[string]any{
			"query":                query,
			"provider":             providerName,
			"providerActual":       actualProvider,
			"results":              items,
			"resultCount":          len(results),
			"allowedCount":         allowedCount,
			"neutralCount":         neutralCount,
			"deniedCount":          deniedCount,
			"urls":                 fetchableURLs,
			"providerFallbackUsed": fallbackUsed,
		},
	}, nil
}

func (t *WebSearchTool) enrichResult(result searchprovider.SearchResult, providerName string) searchprovider.SearchResult {
	result.Title = strings.TrimSpace(result.Title)
	result.URL = strings.TrimSpace(result.URL)
	result.Snippet = strings.TrimSpace(result.Snippet)
	result.Provider = ragcore.FirstNonEmpty(result.Provider, providerName)

	if t.sourcePolicy == nil {
		t.sourcePolicy = searchprovider.NewSourcePolicyEngine(searchprovider.SourcePolicyConfig{})
	}
	assessment := t.sourcePolicy.Evaluate(result.URL)
	result.Domain = ragcore.FirstNonEmpty(result.Domain, assessment.Domain)
	result.SourceType = ragcore.FirstNonEmpty(result.SourceType, assessment.SourceType)
	result.Policy = ragcore.FirstNonEmpty(result.Policy, assessment.Policy)
	result.RiskFlags = uniqueTrimmedValues(append(result.RiskFlags, assessment.RiskFlags...))
	result.Reasons = uniqueTrimmedValues(append(result.Reasons, assessment.Reasons...))
	return result
}

func searchProviderName(provider searchprovider.SearchProvider) string {
	return searchprovider.ProviderName(provider)
}

func derefFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

// Ensure WebSearchTool implements Tool.
var _ ragtool.Tool = (*WebSearchTool)(nil)

func uniqueTrimmedValues(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}
