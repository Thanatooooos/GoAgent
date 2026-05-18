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
	if !strings.Contains(context, "[3] 内容C") {
		t.Fatalf("expected no section annotation for chunk 3: %q", context)
	}
}

type mockSearcher struct {
	hits         []corevector.SearchHit
	keywordHits  []corevector.SearchHit
	metadataHits []corevector.SearchHit
	err          error
	keywordErr   error
	metadataErr  error
}

func (m *mockSearcher) Search(context.Context, corevector.SearchRequest) ([]corevector.SearchHit, error) {
	return m.hits, m.err
}

func (m *mockSearcher) SearchByKeyword(context.Context, string, []string, int) ([]corevector.SearchHit, error) {
	return m.keywordHits, m.keywordErr
}

func (m *mockSearcher) SearchByMetadata(context.Context, string, []string, int) ([]corevector.SearchHit, error) {
	return m.metadataHits, m.metadataErr
}

var _ corevector.Searcher = (*mockSearcher)(nil)

type mockEmbedding struct {
	vector []float32
	err    error
}

func (m *mockEmbedding) Embed(string) ([]float32, error) { return m.vector, m.err }

func (m *mockEmbedding) EmbedBatch(texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.vector
	}
	return result, m.err
}

func (m *mockEmbedding) EmbedWithModel(string, string) ([]float32, error) { return m.vector, m.err }

func (m *mockEmbedding) EmbedBatchWithModel(texts []string, modelID string) ([][]float32, error) {
	return m.EmbedBatch(texts)
}

func (m *mockEmbedding) Dimension() int { return 768 }

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
	if len(result.SearchChannels) != 3 {
		t.Fatalf("unexpected search channels: %+v", result.SearchChannels)
	}
}

func TestRetrieveSemanticModeExplicit(t *testing.T) {
	searcher := &mockSearcher{
		hits: []corevector.SearchHit{
			{ChunkID: "v1", Text: "vector only", Score: 0.95},
		},
		keywordHits: []corevector.SearchHit{
			{ChunkID: "k1", Text: "keyword result", Score: 1.0},
		},
		metadataHits: []corevector.SearchHit{
			{ChunkID: "m1", Text: "metadata result", Score: 1.0},
		},
	}
	embedding := &mockEmbedding{vector: []float32{0.1, 0.2}}

	engine := NewEngine(searcher, embedding, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query:      "semantic test",
		SearchMode: SearchModeSemantic,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "v1" {
		t.Fatalf("unexpected semantic result: %+v", result)
	}
	if len(result.SearchChannels) != 1 || result.SearchChannels[0] != ChannelVectorGlobal {
		t.Fatalf("unexpected semantic search channels: %+v", result.SearchChannels)
	}
	if len(result.ChannelStats) != 1 || result.ChannelStats[0].Name != ChannelVectorGlobal {
		t.Fatalf("unexpected semantic channel stats: %+v", result.ChannelStats)
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
	if len(result.SearchChannels) != 2 {
		t.Fatalf("unexpected search channels: %+v", result.SearchChannels)
	}
}

func TestRetrieveKeywordModeUsesMetadataTitleChannelForFileLookup(t *testing.T) {
	searcher := &mockSearcher{
		metadataHits: []corevector.SearchHit{
			{
				ChunkID: "m1",
				Text:    "trace handlers implementation",
				Score:   1.0,
				Metadata: map[string]any{
					"document_name":    "trace_handlers.go",
					"source_file_name": "trace_handlers.go",
					"section":          "RAG Trace Handler",
				},
			},
		},
	}
	engine := NewEngine(searcher, nil, nil)

	result, err := engine.Retrieve(context.Background(), Request{
		Query:      "查找文件名是 trace_handlers.go 的实现",
		SearchMode: SearchModeAuto,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0].ID != "m1" {
		t.Fatalf("unexpected metadata title result: %+v", result)
	}
	if len(result.SearchChannels) != 2 {
		t.Fatalf("unexpected search channels: %+v", result.SearchChannels)
	}
	foundMetadataTitle := false
	for _, name := range result.SearchChannels {
		if name == ChannelMetadataTitle {
			foundMetadataTitle = true
			break
		}
	}
	if !foundMetadataTitle {
		t.Fatalf("expected metadata title channel in %+v", result.SearchChannels)
	}
	if len(result.ChannelStats) != 2 {
		t.Fatalf("unexpected channel stats: %+v", result.ChannelStats)
	}
	foundMetadataTitle = false
	for _, stat := range result.ChannelStats {
		if stat.Name == ChannelMetadataTitle {
			foundMetadataTitle = true
			break
		}
	}
	if !foundMetadataTitle {
		t.Fatalf("expected metadata title stats in %+v", result.ChannelStats)
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
	if len(result.Chunks) != 2 {
		t.Fatalf("expected 2 chunks from hybrid, got %d: %+v", len(result.Chunks), result.Chunks)
	}
	if len(result.SearchChannels) != 3 {
		t.Fatalf("expected 3 search channels, got %+v", result.SearchChannels)
	}
	if len(result.ChannelStats) != 3 {
		t.Fatalf("expected 3 channel stats, got %+v", result.ChannelStats)
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
	if len(result.ChannelStats) != 3 {
		t.Fatalf("expected failed and successful channel stats, got %+v", result.ChannelStats)
	}
	foundFailed := false
	for _, stat := range result.ChannelStats {
		if stat.Name == ChannelVectorGlobal && stat.Error != "" {
			foundFailed = true
			if stat.Metadata["status"] != "failed" {
				t.Fatalf("expected failed channel status metadata, got %+v", stat.Metadata)
			}
		}
	}
	if !foundFailed {
		t.Fatalf("expected failed vector channel stat, got %+v", result.ChannelStats)
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

func TestMergeResultsAggregatesChannelMetadata(t *testing.T) {
	merged := MergeResults([]Result{
		{
			Chunks: []convention.RetrievedChunk{
				{ID: "c1", Score: 0.9, Text: "A"},
			},
			SearchChannels: []string{ChannelVectorGlobal},
			ChannelStats: []ChannelStat{
				{Name: ChannelVectorGlobal, ChunkCount: 1, LatencyMs: 10},
			},
		},
		{
			Chunks: []convention.RetrievedChunk{
				{ID: "c2", Score: 0.8, Text: "B"},
			},
			SearchChannels: []string{ChannelKeyword},
			ChannelStats: []ChannelStat{
				{Name: ChannelKeyword, ChunkCount: 1, LatencyMs: 5},
			},
		},
	}, 5)

	if len(merged.SearchChannels) != 2 {
		t.Fatalf("expected 2 merged channels, got %+v", merged.SearchChannels)
	}
	if len(merged.ChannelStats) != 2 {
		t.Fatalf("expected 2 merged channel stats, got %+v", merged.ChannelStats)
	}
	if merged.KnowledgeContext == "" {
		t.Fatal("expected merged knowledge context")
	}
}
