package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type queryVectorCache struct {
	Model   string                  `json:"model"`
	Vectors map[string]cachedVector `json:"vectors"`
	path    string
	hits    int
	misses  int
	mu      sync.Mutex
}

type cachedVector struct {
	Query     string    `json:"query"`
	Embedding []float32 `json:"embedding"`
}

func loadQueryVectorCache(path, model string) (*queryVectorCache, error) {
	cache := &queryVectorCache{Model: strings.TrimSpace(model), Vectors: map[string]cachedVector{}, path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cache, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}
	if cache.Model != strings.TrimSpace(model) {
		return nil, fmt.Errorf("cache model %q does not match configured model %q", cache.Model, strings.TrimSpace(model))
	}
	if cache.Vectors == nil {
		cache.Vectors = map[string]cachedVector{}
	}
	cache.path = path
	return cache, nil
}

func (c *queryVectorCache) get(query string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.Vectors[queryVectorKey(c.Model, query)]
	if !ok || item.Query != query {
		c.misses++
		return nil, false
	}
	c.hits++
	return append([]float32(nil), item.Embedding...), true
}

func (c *queryVectorCache) put(query string, vector []float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Vectors[queryVectorKey(c.Model, query)] = cachedVector{Query: query, Embedding: append([]float32(nil), vector...)}
	data, err := json.MarshalIndent(struct {
		Model   string                  `json:"model"`
		Vectors map[string]cachedVector `json:"vectors"`
	}{Model: c.Model, Vectors: c.Vectors}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o644)
}

func (c *queryVectorCache) entryCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.Vectors)
}

func queryVectorKey(model, query string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(model) + "\n" + query))
	return hex.EncodeToString(sum[:])
}

type cachedEmbeddingService struct {
	inner aiembedding.EmbeddingService
	cache *queryVectorCache
}

func (s *cachedEmbeddingService) Embed(text string) ([]float32, error) {
	if vector, ok := s.cache.get(text); ok {
		return vector, nil
	}
	vector, err := s.inner.Embed(text)
	if err != nil {
		return nil, err
	}
	if err := s.cache.put(text, vector); err != nil {
		return nil, fmt.Errorf("persist query vector cache: %w", err)
	}
	return vector, nil
}

func (s *cachedEmbeddingService) EmbedWithModel(text, modelID string) ([]float32, error) {
	return s.inner.EmbedWithModel(text, modelID)
}

func (s *cachedEmbeddingService) EmbedBatch(texts []string) ([][]float32, error) {
	return s.inner.EmbedBatch(texts)
}

func (s *cachedEmbeddingService) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return s.inner.EmbedBatchWithModel(texts, modelID)
}

func (s *cachedEmbeddingService) Dimension() int { return s.inner.Dimension() }
