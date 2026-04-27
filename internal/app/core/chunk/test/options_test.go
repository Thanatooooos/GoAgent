package chunk_test

import (
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestOptionsNormalize(t *testing.T) {
	opts := (chunk.Options{ChunkSize: 0, OverlapSize: 999, MinChunkSize: -1}).Normalize()

	if opts.Strategy != chunk.StrategyFixedSize {
		t.Fatalf("expected default strategy, got %s", opts.Strategy)
	}
	if opts.ChunkSize != 800 {
		t.Fatalf("expected default chunk size, got %d", opts.ChunkSize)
	}
	if opts.OverlapSize >= opts.ChunkSize {
		t.Fatalf("expected overlap to be clamped, got %d >= %d", opts.OverlapSize, opts.ChunkSize)
	}
	if opts.MinChunkSize != 0 {
		t.Fatalf("expected min chunk size to be normalized to 0, got %d", opts.MinChunkSize)
	}
}
