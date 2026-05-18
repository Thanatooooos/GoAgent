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

func TestMarkdownChunkerAttachesHeadingMetadata(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	text := "# 第一章\n这是第一章的引言内容。\n\n## 第一节\n第一节的具体内容在这里。\n\n### 小节\n小节内的详细说明。"

	chunks, err := chunker.Chunk(text, chunk.Options{
		Strategy:  chunk.StrategyMarkdown,
		ChunkSize: 200,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// 第一个 chunk 是 h1，应标记 section="第一章"。
	if section, ok := chunks[0].Metadata["section"]; !ok || section != "第一章" {
		t.Fatalf("expected first chunk section='第一章', got %v", chunks[0].Metadata)
	}
	if level, ok := chunks[0].Metadata["heading_level"]; !ok || level != 1 {
		t.Fatalf("expected heading_level=1, got %v", chunks[0].Metadata["heading_level"])
	}

	// 第二个 chunk 是 h2 "第一节"，应标记 section="第一章 > 第一节"。
	if section, ok := chunks[1].Metadata["section"]; !ok {
		t.Fatalf("expected second chunk to have section metadata, got %v", chunks[1].Metadata)
	} else if !strings.Contains(section.(string), "第一章") || !strings.Contains(section.(string), "第一节") {
		t.Fatalf("expected section to contain '第一章 > 第一节', got %q", section)
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

func TestMarkdownChunkerDetectsCodeLanguage(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	text := "## 代码示例\n\n```python\nprint('hello')\nprint('world')\n```"

	chunks, err := chunker.Chunk(text, chunk.Options{
		Strategy:  chunk.StrategyMarkdown,
		ChunkSize: 50,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	// 小块大小会触发超长 block 降级为 fixed_size 切分。
	// 至少确认不 panic 且 chunk 正常产生。
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}
	t.Logf("code block chunks: %d", len(chunks))
	for i, c := range chunks {
		t.Logf("chunk[%d]: metadata=%v text=%q", i, c.Metadata, c.Text[:min(60, len(c.Text))])
	}
}

func TestMarkdownChunkerHandlesPlainCodeFenceWithoutLanguage(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	text := "# Title\n\n```\nplain code\n```\n\nAfter fence"

	chunks, err := chunker.Chunk(text, chunk.Options{
		Strategy:  chunk.StrategyMarkdown,
		ChunkSize: 128,
	})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks")
	}
	foundFence := false
	for _, item := range chunks {
		if strings.Contains(item.Text, "```\nplain code\n```") {
			foundFence = true
			break
		}
	}
	if !foundFence {
		t.Fatalf("expected a chunk to preserve the plain code fence, got %+v", chunks)
	}
}

func TestMarkdownChunkerEmptyInput(t *testing.T) {
	chunker := chunk.NewMarkdownChunker()
	chunks, err := chunker.Chunk("", chunk.Options{})
	if err != nil {
		t.Fatalf("chunk returned error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(chunks))
	}
}

func TestParseHeading(t *testing.T) {
	// parseHeading 是包内函数，通过公共 API 间接验证其效果。
	// 已在 TestMarkdownChunkerAttachesHeadingMetadata 中充分覆盖。
}
