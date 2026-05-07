package retrieve

import (
	"context"
	"strings"
	"testing"

	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/convention"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

func TestBuildKnowledgeContext(t *testing.T) {
	context := BuildKnowledgeContext([]convention.RetrievedChunk{
		{Text: "A"},
		{Text: "B"},
	})

	if !strings.Contains(context, "[1] A") || !strings.Contains(context, "[2] B") {
		t.Fatalf("unexpected context: %q", context)
	}
}

func TestBuildKnowledgeContextWithSection(t *testing.T) {
	context := BuildKnowledgeContext([]convention.RetrievedChunk{
		{Text: "内容A", Metadata: map[string]any{"section": "第一章 > 概述"}},
		{Text: "内容B", Metadata: map[string]any{"section": "第二章"}},
		{Text: "内容C"},
	})

	if !strings.Contains(context, "(第一章 > 概述)") {
		t.Fatalf("expected section annotation for chunk 1: %q", context)
	}
	if !strings.Contains(context, "(第二章)") {
		t.Fatalf("expected section annotation for chunk 2: %q", context)
	}
	// 无 section 的 chunk 不加括号标注。
	if !strings.Contains(context, "[3] 内容C") {
		t.Fatalf("expected no section annotation for chunk 3: %q", context)
	}
}

// mockSearcher 实现 corevector.Searcher 用于测试。
type mockSearcher struct {
	hits        []corevector.SearchHit
	keywordHits []corevector.SearchHit
	err         error
	keywordErr  error
}

func (m *mockSearcher) Search(ctx context.Context, request corevector.SearchRequest) ([]corevector.SearchHit, error) {
	return m.hits, m.err
}

func (m *mockSearcher) SearchByKeyword(ctx context.Context, query string, knowledgeBaseIDs []string, topK int) ([]corevector.SearchHit, error) {
	return m.keywordHits, m.keywordErr
}

var _ corevector.Searcher = (*mockSearcher)(nil)

// mockEmbedding 实现 embedding.EmbeddingService 用于测试。
type mockEmbedding struct {
	vector []float32
	err    error
}

func (m *mockEmbedding) Embed(text string) ([]float32, error) {
	return m.vector, m.err
}

func (m *mockEmbedding) EmbedBatch(texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.vector
	}
	return result, m.err
}

func (m *mockEmbedding) EmbedWithModel(text string, modelID string) ([]float32, error) {
	return m.vector, m.err
}

func (m *mockEmbedding) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return m.EmbedBatch(texts)
}

func (m *mockEmbedding) Dimension() int {
	return 768
}

var _ aiembedding.EmbeddingService = (*mockEmbedding)(nil)

func TestRetrieveSemanticModeDefault(t *testing.T) {
	searcher := &mockSearcher{
		hits: []corevector.SearchHit{
			{ChunkID: "c1", Text: "hello", Score: 0.95},
		},
	}
	embedding := &mockEmbedding{vector: []float32{0.1, 0.2}}

	engine := NewEngine(searcher, embedding, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "c1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRetrieveKeywordMode(t *testing.T) {
	searcher := &mockSearcher{
		keywordHits: []corevector.SearchHit{
			{ChunkID: "k1", Text: "keyword result", Score: 1.0},
		},
	}
	engine := NewEngine(searcher, nil, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query:      "keyword test",
		SearchMode: SearchModeKeyword,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "k1" {
		t.Fatalf("unexpected keyword result: %+v", result)
	}
}

func TestResolveSearchModeAuto(t *testing.T) {
	mode := resolveSearchMode(Request{
		Query:      "nginx 404 报错怎么配置",
		SearchMode: SearchModeAuto,
	})
	if mode != SearchModeHybrid {
		t.Fatalf("expected hybrid, got %q", mode)
	}
}

func TestResolveSearchModeEmptyFallsBackToSemanticForConceptQuestion(t *testing.T) {
	mode := resolveSearchMode(Request{
		Query: "什么是RAG",
	})
	if mode != SearchModeSemantic {
		t.Fatalf("expected semantic, got %q", mode)
	}
}

func TestResolveSearchModeInvalidFallsBackToInference(t *testing.T) {
	mode := resolveSearchMode(Request{
		Query:      "标题包含 Kubernetes 的文档",
		SearchMode: "invalid",
	})
	if mode != SearchModeKeyword {
		t.Fatalf("expected keyword, got %q", mode)
	}
}

func TestRetrieveHybridMode(t *testing.T) {
	searcher := &mockSearcher{
		hits: []corevector.SearchHit{
			{ChunkID: "v1", Text: "vector", Score: 0.9},
		},
		keywordHits: []corevector.SearchHit{
			{ChunkID: "k1", Text: "keyword", Score: 1.0},
		},
	}
	embedding := &mockEmbedding{vector: []float32{0.1}}

	engine := NewEngine(searcher, embedding, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query:      "hybrid test",
		SearchMode: SearchModeHybrid,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 两条结果分别来自两个通道。
	if len(result.Chunks) != 2 {
		t.Fatalf("expected 2 chunks from hybrid, got %d: %+v", len(result.Chunks), result.Chunks)
	}
}

func TestRetrieveHybridVectorFailsKeywordOk(t *testing.T) {
	searcher := &mockSearcher{
		err: context.DeadlineExceeded,
		keywordHits: []corevector.SearchHit{
			{ChunkID: "k1", Text: "keyword only", Score: 1.0},
		},
	}
	embedding := &mockEmbedding{vector: []float32{0.1}}

	engine := NewEngine(searcher, embedding, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query:      "test",
		SearchMode: SearchModeHybrid,
	})
	if err != nil {
		t.Fatalf("unexpected error (should fallback): %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "k1" {
		t.Fatalf("expected keyword fallback, got %+v", result)
	}
}

func TestRetrieveEmptyQuery(t *testing.T) {
	engine := NewEngine(nil, nil, nil)
	result, err := engine.Retrieve(context.Background(), Request{Query: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 0 || result.KnowledgeContext != "" {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestRetrieveNilEngine(t *testing.T) {
	var engine *Engine
	_, err := engine.Retrieve(context.Background(), Request{Query: "test"})
	if err == nil {
		t.Fatal("expected error for nil engine")
	}
}
