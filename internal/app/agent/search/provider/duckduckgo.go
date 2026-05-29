package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<18))
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
			rawURL := strings.TrimSpace(topic.FirstURL)
			if text == "" || rawURL == "" {
				continue
			}
			title, snippet := splitTopicText(text)
			results = append(results, SearchResult{
				Title:   title,
				URL:     rawURL,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
