package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

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

// WebSearchTool performs an external web search via a configurable SearchProvider.
type WebSearchTool struct {
	provider     SearchProvider
	sourcePolicy *SourcePolicyEngine
}

func NewWebSearchTool(provider SearchProvider, policy ...*SourcePolicyEngine) *WebSearchTool {
	var engine *SourcePolicyEngine
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
		if result.Policy == SourcePolicyDeny {
			deniedCount++
		} else if result.Policy == SourcePolicyAllow {
			allowedCount++
		} else {
			neutralCount++
		}
		if result.Policy != SourcePolicyDeny && strings.TrimSpace(result.URL) != "" {
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

func (t *WebSearchTool) enrichResult(result SearchResult, providerName string) SearchResult {
	result.Title = strings.TrimSpace(result.Title)
	result.URL = strings.TrimSpace(result.URL)
	result.Snippet = strings.TrimSpace(result.Snippet)
	result.Provider = ragcore.FirstNonEmpty(result.Provider, providerName)

	if t.sourcePolicy == nil {
		t.sourcePolicy = NewSourcePolicyEngine(SourcePolicyConfig{})
	}
	assessment := t.sourcePolicy.Evaluate(result.URL)
	result.Domain = ragcore.FirstNonEmpty(result.Domain, assessment.Domain)
	result.SourceType = ragcore.FirstNonEmpty(result.SourceType, assessment.SourceType)
	result.Policy = ragcore.FirstNonEmpty(result.Policy, assessment.Policy)
	result.RiskFlags = uniqueTrimmedValues(append(result.RiskFlags, assessment.RiskFlags...))
	result.Reasons = uniqueTrimmedValues(append(result.Reasons, assessment.Reasons...))
	return result
}

func searchProviderName(provider SearchProvider) string {
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
	default:
		return "unknown"
	}
}

func derefFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

// Ensure WebSearchTool implements Tool.
var _ ragtool.Tool = (*WebSearchTool)(nil)

// =============================================================================
// DuckDuckGo Provider (free, no API key, blocked in mainland China)
// =============================================================================

const (
	duckDuckGoAPI       = "https://api.duckduckgo.com/"
	webSearchTimeout    = 8 * time.Second
	webSearchMaxResults = 5
)

type DuckDuckGoProvider struct {
	client *http.Client
}

func NewDuckDuckGoProvider() *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		client: &http.Client{Timeout: webSearchTimeout},
	}
}

func (p *DuckDuckGoProvider) Search(query string) ([]SearchResult, error) {
	requestURL := fmt.Sprintf("%s?q=%s&format=json&no_html=1&skip_disambig=1", duckDuckGoAPI, url.QueryEscape(query))

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goagent/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<18)) // 256KB limit
	if err != nil {
		return nil, err
	}

	var parsed duckDuckGoResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}

	return parsed.extractResults(), nil
}

type duckDuckGoResponse struct {
	AbstractText  string            `json:"AbstractText"`
	AbstractURL   string            `json:"AbstractURL"`
	Heading       string            `json:"Heading"`
	Answer        string            `json:"Answer"`
	RelatedTopics []duckDuckGoTopic `json:"RelatedTopics"`
}

type duckDuckGoTopic struct {
	Text     string            `json:"Text"`
	FirstURL string            `json:"FirstURL"`
	Topics   []duckDuckGoTopic `json:"Topics,omitempty"`
}

func (r *duckDuckGoResponse) extractResults() []SearchResult {
	var results []SearchResult

	if r.AbstractText != "" {
		results = append(results, SearchResult{
			Title:   ragcore.FirstNonEmpty(r.Heading, "Abstract"),
			URL:     r.AbstractURL,
			Snippet: ragcore.TruncateText(r.AbstractText, 300),
		})
	}

	collectTopics := func(topics []duckDuckGoTopic) {
		for _, topic := range topics {
			if len(results) >= webSearchMaxResults {
				return
			}
			text := strings.TrimSpace(topic.Text)
			url := strings.TrimSpace(topic.FirstURL)
			if text == "" || url == "" {
				continue
			}
			title, snippet := splitTopicText(text)
			results = append(results, SearchResult{
				Title:   title,
				URL:     url,
				Snippet: snippet,
			})
		}
	}

	collectTopics(r.RelatedTopics)

	for _, topic := range r.RelatedTopics {
		if len(results) >= webSearchMaxResults {
			break
		}
		collectTopics(topic.Topics)
	}

	if len(results) > webSearchMaxResults {
		results = results[:webSearchMaxResults]
	}
	return results
}

func splitTopicText(text string) (string, string) {
	idx := strings.Index(text, " - ")
	if idx == -1 {
		return text, ""
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+3:])
}
