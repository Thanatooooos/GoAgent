package main

import (
	"path/filepath"
	"testing"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

func TestLoadRewriteEvalSamples(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "rewrite_eval_samples.json")
	samples, err := loadSamples(path)
	if err != nil {
		t.Fatalf("loadSamples returned error: %v", err)
	}
	if len(samples) < 55 {
		t.Fatalf("expected at least 55 rewrite eval samples, got %d", len(samples))
	}
	for _, sample := range samples {
		if sample.Name == "" || sample.Query == "" {
			t.Fatalf("sample missing name or query: %+v", sample)
		}
	}

	summary, err := rageval.EvaluateRewriteSamples(samples)
	if err != nil {
		t.Fatalf("EvaluateRewriteSamples returned error: %v", err)
	}
	if summary.Overall.SampleCount != len(samples) {
		t.Fatalf("expected %d samples evaluated, got %d", len(samples), summary.Overall.SampleCount)
	}
}
