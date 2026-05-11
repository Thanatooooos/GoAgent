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
)

const (
	duckDuckGoAPI       = "https://api.duckduckgo.com/"
	webSearchTimeout    = 8 * time.Second
	webSearchMaxResults = 5
)

// WebSearchTool performs an external web search via DuckDuckGo Instant Answer API.
// No API key is required.
type WebSearchTool struct {
	client *http.Client
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		client: &http.Client{Timeout: webSearchTimeout},
	}
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
	query := strings.TrimSpace(readStringArg(call.Arguments, "query"))
	if query == "" {
		return ragtool.Result{Name: "web_search", Status: ragtool.CallStatusFailed, ErrorMessage: "query is required"}, nil
	}

	results, err := t.search(query)
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
	snippets := make([]string, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"title":   r.Title,
			"url":     r.URL,
			"snippet": r.Snippet,
		})
		snippets = append(snippets, fmt.Sprintf("- %s (%s): %s", r.Title, r.URL, r.Snippet))
	}

	return ragtool.Result{
		Name:    "web_search",
		Status:  ragtool.CallStatusSuccess,
		Summary: fmt.Sprintf("found %d web results for %q", len(results), query),
		Data: map[string]any{
			"query":   query,
			"results": items,
		},
	}, nil
}

type webSearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (t *WebSearchTool) search(query string) ([]webSearchResult, error) {
	requestURL := fmt.Sprintf("%s?q=%s&format=json&no_html=1&skip_disambig=1", duckDuckGoAPI, url.QueryEscape(query))

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "goagent/1.0")

	resp, err := t.client.Do(req)
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
	AbstractText string            `json:"AbstractText"`
	AbstractURL  string            `json:"AbstractURL"`
	Heading      string            `json:"Heading"`
	Answer       string            `json:"Answer"`
	RelatedTopics []duckDuckGoTopic `json:"RelatedTopics"`
}

type duckDuckGoTopic struct {
	Text     string `json:"Text"`
	FirstURL string `json:"FirstURL"`
	Topics   []duckDuckGoTopic `json:"Topics,omitempty"`
}

func (r *duckDuckGoResponse) extractResults() []webSearchResult {
	var results []webSearchResult

	if r.AbstractText != "" {
		results = append(results, webSearchResult{
			Title:   firstNonEmpty(r.Heading, "Abstract"),
			URL:     r.AbstractURL,
			Snippet: truncateText(r.AbstractText, 300),
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
			results = append(results, webSearchResult{
				Title:   title,
				URL:     url,
				Snippet: snippet,
			})
		}
	}

	collectTopics(r.RelatedTopics)

	// Also collect nested topics.
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

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Ensure WebSearchTool implements Tool.
var _ ragtool.Tool = (*WebSearchTool)(nil)
