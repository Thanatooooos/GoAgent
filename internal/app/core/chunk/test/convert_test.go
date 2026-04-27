package chunk_test

import (
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestToRetrievedChunks(t *testing.T) {
	chunks := []chunk.Chunk{
		chunk.NewChunk("chunk-0000", 0, "first"),
		chunk.NewChunk("chunk-0001", 1, "second"),
	}

	retrieved := chunk.ToRetrievedChunks(chunks)
	if len(retrieved) != 2 {
		t.Fatalf("expected 2 retrieved chunks, got %d", len(retrieved))
	}
	if retrieved[0].ID != "chunk-0000" || retrieved[1].Text != "second" {
		t.Fatalf("unexpected retrieved chunks: %#v", retrieved)
	}
}
