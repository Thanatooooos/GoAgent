package runtimeadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"local/rag-project/internal/app/runtime/capability"
)

type TavilySearch struct {
	key    string
	client *http.Client
}

func NewTavilySearch(key string, client *http.Client) *TavilySearch {
	if client == nil {
		client = &http.Client{}
	}
	return &TavilySearch{key: key, client: client}
}
func (s *TavilySearch) Search(ctx context.Context, query string) ([]capability.WebSearchResult, error) {
	if strings.TrimSpace(s.key) == "" {
		return nil, fmt.Errorf("tavily api key is not configured")
	}
	body, _ := json.Marshal(map[string]any{"api_key": s.key, "query": query, "search_depth": "basic", "max_results": 5})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tavily http %d", resp.StatusCode)
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	result := make([]capability.WebSearchResult, 0, len(out.Results))
	for _, x := range out.Results {
		result = append(result, capability.WebSearchResult{Title: x.Title, URL: x.URL, Snippet: x.Content})
	}
	return result, nil
}

type WebFetcher struct {
	client    *http.Client
	lookupIPs func(context.Context, string) ([]net.IP, error)
}

func NewWebFetcher(client *http.Client) *WebFetcher {
	if client == nil {
		client = &http.Client{}
	}
	return &WebFetcher{client: client, lookupIPs: lookupIP}
}
func (f *WebFetcher) Fetch(ctx context.Context, urls []string) ([]capability.WebPage, error) {
	return f.FetchScoped(ctx, urls, nil)
}

func (f *WebFetcher) FetchScoped(ctx context.Context, urls []string, allowedDomains []string) ([]capability.WebPage, error) {
	pages := make([]capability.WebPage, 0, len(urls))
	for _, rawURL := range urls {
		page, err := f.fetch(ctx, rawURL, allowedDomains)
		if err != nil {
			pages = append(pages, capability.WebPage{URL: rawURL, Error: err.Error()})
			continue
		}
		pages = append(pages, page)
	}
	return pages, nil
}

const maxWebFetchRedirects = 5

func (f *WebFetcher) fetch(ctx context.Context, rawURL string, allowedDomains []string) (capability.WebPage, error) {
	client := *f.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	current := rawURL
	for redirects := 0; redirects <= maxWebFetchRedirects; redirects++ {
		if !capability.WebDomainAllowed(current, allowedDomains) {
			return capability.WebPage{}, fmt.Errorf("web fetch source is outside task scope")
		}
		if err := f.validatePublicURL(ctx, current); err != nil {
			return capability.WebPage{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return capability.WebPage{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return capability.WebPage{}, err
		}
		if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return capability.WebPage{}, fmt.Errorf("redirect has no location")
			}
			next, err := resolveRedirect(current, location)
			if err != nil {
				return capability.WebPage{}, err
			}
			current = next
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return capability.WebPage{}, readErr
		}
		text := strings.TrimSpace(string(raw))
		if len(text) > 8192 {
			text = text[:8192]
		}
		return capability.WebPage{URL: current, Text: text}, nil
	}
	return capability.WebPage{}, fmt.Errorf("too many redirects")
}

func (f *WebFetcher) validatePublicURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("invalid public http url")
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("non-public url is not permitted")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return fmt.Errorf("non-public url is not permitted")
		}
		return nil
	}
	lookup := f.lookupIPs
	if lookup == nil {
		lookup = lookupIP
	}
	ips, err := lookup(ctx, host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("resolve public url host: %w", err)
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("non-public url is not permitted")
		}
	}
	return nil
}

func lookupIP(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

func resolveRedirect(current, location string) (string, error) {
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	next, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(next).String(), nil
}
