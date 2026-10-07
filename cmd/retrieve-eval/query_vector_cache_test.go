package main

import (
	"testing"
)

func TestQueryVectorCachePersistsAndReusesEmbedding(t *testing.T) {
	path := t.TempDir() + "/vectors.json"
	cache, err := loadQueryVectorCache(path, "embedding-test")
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if err := cache.put("原问题", []float32{0.1, 0.2}); err != nil {
		t.Fatalf("put cache: %v", err)
	}
	reloaded, err := loadQueryVectorCache(path, "embedding-test")
	if err != nil {
		t.Fatalf("reload cache: %v", err)
	}
	vector, ok := reloaded.get("原问题")
	if !ok || len(vector) != 2 || vector[0] != 0.1 {
		t.Fatalf("cached vector = %v, found=%t", vector, ok)
	}
}
