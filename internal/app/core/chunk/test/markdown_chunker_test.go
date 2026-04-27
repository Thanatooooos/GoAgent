package chunk_test

import (
	"strings"
	"testing"

	chunk "local/rag-project/internal/app/core/chunk"
)

func TestMarkdownChunkerSplitsByHeadingAndParagraph(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	text := "# Title\nintro line\n\n## Section\nsection line 1\nsection line 2"

	chunks, err := chunker.Chunk(text, chunk.Options{
		Strategy:  chunk.StrategyMarkdown,
		ChunkSize: 64,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Text, "# Title") {
		t.Fatalf("expected first chunk to contain title heading, got %q", chunks[0].Text)
	}
	if !strings.Contains(chunks[1].Text, "## Section") {
		t.Fatalf("expected second chunk to contain section heading, got %q", chunks[1].Text)
	}
	if chunks[0].ID == "" || chunks[1].ID == "" {
		t.Fatal("expected generated chunk ids")
	}
}

func TestMarkdownChunkerKeepsCodeFenceTogetherWhenPossible(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	text := "# Title\n\n```go\nfmt.Println(\"hi\")\nfmt.Println(\"there\")\n```"

	chunks, err := chunker.Chunk(text, chunk.Options{
		Strategy:  chunk.StrategyMarkdown,
		ChunkSize: 128,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.Contains(chunks[0].Text, "```go") || !strings.Contains(chunks[0].Text, "fmt.Println(\"there\")") {
		t.Fatalf("expected code fence content to stay together, got %q", chunks[0].Text)
	}
}
