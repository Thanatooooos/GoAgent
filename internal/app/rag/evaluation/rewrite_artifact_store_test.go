package evaluation_test

import (
	"os"
	"path/filepath"
	"testing"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

func TestRewriteArtifactStoreSaveAndResume(t *testing.T) {
	dir := t.TempDir()
	store := rageval.NewRewriteArtifactStore(dir, true)

	checkpoint := rageval.RewriteSampleCheckpoint{
		SampleName: "sample-a",
		Query:      "what is RAG",
		SampleResult: rageval.SharedSampleResult{
			Name:   "sample-a",
			Passed: true,
			Scores: map[string]any{"semantic_score": 0.9},
		},
		Artifact: map[string]any{
			"semantic_evaluation": map[string]any{
				"semantic_score": 0.9,
				"rewrite_similarity": 0.88,
			},
		},
	}
	if err := store.Save(checkpoint); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, ok, err := store.Load("sample-a", "what is RAG")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !ok {
		t.Fatal("Load() ok = false, want true")
	}
	if loaded.SampleResult.Name != "sample-a" {
		t.Fatalf("loaded sample = %q", loaded.SampleResult.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "semantic_evaluations.json")); err != nil {
		t.Fatalf("semantic_evaluations.json missing: %v", err)
	}

	_, ok, err = store.Load("sample-a", "changed query")
	if err != nil {
		t.Fatalf("Load() mismatch error = %v", err)
	}
	if ok {
		t.Fatal("Load() ok = true for mismatched query, want false")
	}
}
