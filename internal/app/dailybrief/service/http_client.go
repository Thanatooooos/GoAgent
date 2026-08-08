package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient(client *http.Client) *HTTPClient {
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &HTTPClient{client: client}
}

func (c *HTTPClient) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("request %q: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return body, nil
}
