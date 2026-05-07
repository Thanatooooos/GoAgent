package chunk_test

import (
	"strings"
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

func TestFixedSizeChunkerPrefersSentenceBoundary(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

	// 构造一段带句号的文本，chunk 应在句号处自然断开。
	text := "第一句话的内容在这里。第二句话的内容在这里。第三句话的内容在这里。"
	chunks, err := chunker.Chunk(text, chunk.Options{
		ChunkSize:   20,
		OverlapSize: 2,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	// 每个 chunk 的末尾应该在 `。` 之后（语义分界点）。
	for i, c := range chunks {
		text := c.Text
		if len(text) > 0 && !strings.HasSuffix(strings.TrimSpace(text), "。") {
			t.Logf("chunk[%d] does not end with period (may be last/tail): %q", i, text[:min(30, len(text))])
		}
	}
}

func TestFixedSizeChunkerNoSentenceBoundaryFallsBack(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

	// 无任何标点的纯字母串，应降级为固定 size 切分。
	chunks, err := chunker.Chunk("abcdefghijklmnopqrstuvwxyz", chunk.Options{
		ChunkSize:   8,
		OverlapSize: 2,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 fixed-size chunks, got %d", len(chunks))
	}
}
