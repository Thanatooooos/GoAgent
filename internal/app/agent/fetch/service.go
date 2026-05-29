package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout      = 10 * time.Second
	maxResponseBytes    = 1 << 20
	maxExtractedTextLen = 8192
	maxURLsPerFetch     = 3
)

type Service struct {
	client *http.Client
}

type fetchItem struct {
	url         string
	text        string
	originalLen int
	truncated   bool
	err         error
}

func NewService(client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Service{client: client}
}

func (s *Service) Fetch(ctx context.Context, urls []string) (Output, error) {
	normalized := normalizeURLs(urls)
	if len(normalized) == 0 {
		return Output{
			URLs:          []string{},
			Pages:         []PageResult{},
			Summary:       "web fetch failed: urls are required",
			Degraded:      true,
			DegradeReason: "urls are required",
			ErrorMessage:  "urls are required",
		}, fmt.Errorf("urls are required")
	}
	if len(normalized) > maxURLsPerFetch {
		normalized = normalized[:maxURLsPerFetch]
	}
	if s == nil || s.client == nil {
		return Output{
			URLs:          append([]string(nil), normalized...),
			Pages:         []PageResult{},
			Summary:       "web fetch failed: no http client configured",
			Degraded:      true,
			DegradeReason: "no http client configured",
			ErrorMessage:  "no http client configured",
		}, fmt.Errorf("no http client configured")
	}

	items := s.fetchMultiple(ctx, normalized)
	output := buildOutput(normalized, items)
	if output.FailCount == len(normalized) {
		errMessage := firstFetchError(items)
		if errMessage == "" {
			errMessage = "all urls failed during fetch"
		}
		output.Degraded = true
		output.DegradeReason = errMessage
		output.ErrorMessage = errMessage
		return output, fmt.Errorf("%s", errMessage)
	}
	if output.FailCount > 0 {
		output.Degraded = true
		output.DegradeReason = fmt.Sprintf("%d url(s) failed during fetch", output.FailCount)
	}
	return output, nil
}

func (s *Service) fetchMultiple(ctx context.Context, urls []string) []fetchItem {
	if len(urls) == 1 {
		return []fetchItem{s.fetchOne(ctx, urls[0])}
	}

	items := make([]fetchItem, len(urls))
	var wg sync.WaitGroup
	for i, rawURL := range urls {
		wg.Add(1)
		go func(idx int, current string) {
			defer wg.Done()
			items[idx] = s.fetchOne(ctx, current)
		}(i, rawURL)
	}
	wg.Wait()
	return items
}

func (s *Service) fetchOne(ctx context.Context, rawURL string) fetchItem {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fetchItem{url: rawURL, err: fmt.Errorf("empty url")}
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fetchItem{url: rawURL, err: fmt.Errorf("invalid url scheme")}
	}

	text, err := s.fetchAndExtract(ctx, rawURL)
	if err != nil {
		return fetchItem{url: rawURL, err: err}
	}

	originalLen := len(text)
	truncated := originalLen > maxExtractedTextLen
	text = truncateText(text, maxExtractedTextLen)
	return fetchItem{
		url:         rawURL,
		text:        text,
		originalLen: originalLen,
		truncated:   truncated,
	}
}

func (s *Service) fetchAndExtract(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "goagent/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", err
	}
	return extractText(string(body)), nil
}

func buildOutput(urls []string, items []fetchItem) Output {
	pages := make([]PageResult, 0, len(items))
	successCount := 0
	failCount := 0
	var combined strings.Builder

	for _, item := range items {
		page := PageResult{URL: item.url}
		if item.err != nil {
			page.ErrorMessage = item.err.Error()
			failCount++
		} else {
			page.Text = item.text
			page.OriginalLength = item.originalLen
			page.WasTruncated = item.truncated
			if strings.TrimSpace(item.text) != "" {
				successCount++
				if combined.Len() > 0 {
					combined.WriteString("\n\n---\n\n")
				}
				combined.WriteString(fmt.Sprintf("[%s]\n%s", item.url, item.text))
			}
		}
		pages = append(pages, page)
	}

	return Output{
		URLs:         append([]string(nil), urls...),
		Pages:        pages,
		CombinedText: combined.String(),
		SuccessCount: successCount,
		FailCount:    failCount,
		Summary:      fmt.Sprintf("fetched %d urls: %d ok, %d failed", len(urls), successCount, failCount),
	}
}

func normalizeURLs(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(urls))
	result := make([]string, 0, len(urls))
	for _, rawURL := range urls {
		trimmed := strings.TrimSpace(rawURL)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func truncateText(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}

func firstFetchError(items []fetchItem) string {
	for _, item := range items {
		if item.err != nil {
			return item.err.Error()
		}
	}
	return ""
}
