package chunk_test

import (
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestFixedSizeChunkerChunksWithOverlap(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

	chunks, err := chunker.Chunk("abcdefghij", chunk.Options{
		ChunkSize:   4,
		OverlapSize: 1,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Text != "abcd" || chunks[1].Text != "defg" || chunks[2].Text != "ghij" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
	if chunks[0].ID == "" || chunks[1].ID == "" || chunks[2].ID == "" {
		t.Fatal("expected generated chunk ids")
	}
}

func TestFixedSizeChunkerMergesSmallTail(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

	chunks, err := chunker.Chunk("abcdefghijkl", chunk.Options{
		ChunkSize:    5,
		OverlapSize:  0,
		MinChunkSize: 3,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[1].Text != "fghijkl" {
		t.Fatalf("unexpected tail chunk: %q", chunks[1].Text)
	}
}

func TestFixedSizeChunkerSkipsBlankInput(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

	chunks, err := chunker.Chunk(" \n\t ", chunk.Options{})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(chunks))
	}
}
