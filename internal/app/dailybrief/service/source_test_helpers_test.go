package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type staticHTTPClient struct {
	bodies map[string][]byte
	errs   map[string]error
}

func (c *staticHTTPClient) Get(_ context.Context, url string) ([]byte, error) {
	if err, ok := c.errs[url]; ok {
		return nil, err
	}
	body, ok := c.bodies[url]
	if !ok {
		return nil, fmt.Errorf("unexpected url %q", url)
	}
	return body, nil
}

func loadSourceFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "dailybrief", "sources", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}
	return body
}
