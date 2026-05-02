package chunk_test

import (
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestSelectorChunkUsesDefaultFixedSizeChunker(t *testing.T) {
	selector := chunk.NewDefaultSelector()

	chunks, err := selector.Chunk("hello world", chunk.Options{})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != "hello world" {
		t.Fatalf("unexpected chunk text: %q", chunks[0].Text)
	}
}

func TestSelectorChunkReturnsErrorWhenStrategyMissing(t *testing.T) {
	selector := chunk.NewSelector()

	_, err := selector.Chunk("hello", chunk.Options{Strategy: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelectorChunkSupportsStructureAwareAlias(t *testing.T) {
	selector := chunk.NewDefaultSelector()

	chunks, err := selector.Chunk("# title\n\nhello world", chunk.Options{Strategy: "structure_aware"})
	if err != nil {
		t.Fatalf("expected structure_aware alias to work, got error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
}
