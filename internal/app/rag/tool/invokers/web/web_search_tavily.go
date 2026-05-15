package builtin

import (
	ragcore "local/rag-project/internal/app/rag/tool/core"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	tavilyAPI         = "https://api.tavily.com/search"
	tavilyTimeout     = 10 * time.Second
	tavilyMaxResults  = 5
	tavilyMaxBodySize = 1 << 18 // 256KB
)

// TavilyProvider uses the Tavily Search API (https://tavily.com), which is
// designed for AI agents and is accessible from mainland China.
// Free tier: 1000 searches/month. Sign up at https://app.tavily.com to get an API key.
type TavilyProvider struct {
	apiKey string
	client *http.Client
}

func NewTavilyProvider(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		apiKey: apiKey,
		client: &http.Client{Timeout: tavilyTimeout},
	}
}

type tavilyRequest struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
}

type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func (p *TavilyProvider) Search(query string) ([]SearchResult, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("tavily api key is not configured")
	}

	reqBody := tavilyRequest{
		APIKey:      p.apiKey,
		Query:       query,
		SearchDepth: "basic",
		MaxResults:  tavilyMaxResults,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal tavily request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, tavilyAPI, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "goagent/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, tavilyMaxBodySize))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tavily http %d: %s", resp.StatusCode, ragcore.TruncateText(string(body), 200))
	}

	var parsed tavilyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse tavily response: %w", err)
	}

	results := make([]SearchResult, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		score := r.Score
		results = append(results, SearchResult{
			Title:         strings.TrimSpace(r.Title),
			URL:           strings.TrimSpace(r.URL),
			Snippet:       ragcore.TruncateText(strings.TrimSpace(r.Content), 300),
			Provider:      "tavily",
			ProviderScore: &score,
		})
	}
	return results, nil
}

// Ensure TavilyProvider implements SearchProvider.
var _ SearchProvider = (*TavilyProvider)(nil)
