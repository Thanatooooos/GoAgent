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
	if !strings.Contains(chunks[0].Text, "。") {
		t.Fatalf("expected first chunk to keep a sentence boundary, got %q", chunks[0].Text)
	}
}

func TestFixedSizeChunkerNoSentenceBoundaryFallsBack(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()

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

func TestFixedSizeChunkerRespectsExplicitOverlap(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()
	text := strings.Repeat("a", 1000)

	chunks, err := chunker.Chunk(text, chunk.Options{
		ChunkSize:   300,
		OverlapSize: 120,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	overlap := commonPrefixLen(chunks[0].Text[len(chunks[0].Text)-120:], chunks[1].Text[:120])
	if overlap != 120 {
		t.Fatalf("expected default overlap 120, got %d", overlap)
	}
}

func TestFixedSizeChunkerPrefersChineseOutlineBoundary(t *testing.T) {
	chunker := chunk.NewFixedSizeChunker()
	text := "这是导言部分主要介绍背景信息和前置说明\n第一条 适用范围\n本条说明制度适用的对象和范围\n第二条 管理职责\n本条说明职责分工"

	chunks, err := chunker.Chunk(text, chunk.Options{
		ChunkSize:   26,
		OverlapSize: 0,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	joined := make([]string, 0, len(chunks))
	for _, item := range chunks {
		joined = append(joined, item.Text)
	}
	combined := strings.Join(joined, "\n")
	if !strings.Contains(combined, "第一条 适用范围") || !strings.Contains(combined, "第二条 管理职责") {
		t.Fatalf("expected outline lines to stay intact, got %q", combined)
	}
}

func commonPrefixLen(left string, right string) int {
	runesLeft := []rune(left)
	runesRight := []rune(right)
	limit := len(runesLeft)
	if len(runesRight) < limit {
		limit = len(runesRight)
	}
	count := 0
	for i := 0; i < limit; i++ {
		if runesLeft[i] != runesRight[i] {
			break
		}
		count++
	}
	return count
}
