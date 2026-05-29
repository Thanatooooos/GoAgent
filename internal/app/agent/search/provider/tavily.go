package provider

import (
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
	tavilyMaxBodySize = 1 << 18
)

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
		return nil, fmt.Errorf("tavily http %d: %s", resp.StatusCode, truncateText(string(body), 200))
	}

	var parsed tavilyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse tavily response: %w", err)
	}

	results := make([]SearchResult, 0, len(parsed.Results))
	for _, entry := range parsed.Results {
		score := entry.Score
		results = append(results, SearchResult{
			Title:         strings.TrimSpace(entry.Title),
			URL:           strings.TrimSpace(entry.URL),
			Snippet:       truncateText(strings.TrimSpace(entry.Content), 300),
			Provider:      "tavily",
			ProviderScore: &score,
		})
	}
	return results, nil
}
