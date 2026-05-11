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
	if len(result.SearchChannels) != 1 || result.SearchChannels[0] != ChannelVectorGlobal {
		t.Fatalf("unexpected search channels: %+v", result.SearchChannels)
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
	if len(result.SearchChannels) != 1 || result.SearchChannels[0] != ChannelKeyword {
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

func TestAnalyzeSearchModeExplicitWins(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "什么是 RAG",
		SearchMode: SearchModeKeyword,
	})
	if decision.ResolvedMode != SearchModeKeyword {
		t.Fatalf("expected explicit keyword mode, got %q", decision.ResolvedMode)
	}
	if decision.Source != modeSourceExplicit {
		t.Fatalf("expected explicit source, got %q", decision.Source)
	}
}

func TestAnalyzeSearchModeAutoSemantic(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "什么是 RAG 检索增强生成",
		SearchMode: SearchModeAuto,
	})
	if decision.ResolvedMode != SearchModeSemantic {
		t.Fatalf("expected semantic, got %q", decision.ResolvedMode)
	}
	if decision.Source != modeSourceAuto {
		t.Fatalf("expected auto source, got %q", decision.Source)
	}
	if len(decision.Signals) == 0 {
		t.Fatal("expected semantic decision signals")
	}
}

func TestAnalyzeSearchModeAutoKeyword(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "标题包含 \"Kubernetes\" 的文档",
		SearchMode: SearchModeAuto,
	})
	if decision.ResolvedMode != SearchModeKeyword {
		t.Fatalf("expected keyword, got %q", decision.ResolvedMode)
	}
}

func TestAnalyzeSearchModeAutoKeywordForIdentifierLookup(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "搜索包含 fallback_to_general_model 的节点",
		SearchMode: SearchModeAuto,
	})
	if decision.ResolvedMode != SearchModeKeyword {
		t.Fatalf("expected keyword, got %q", decision.ResolvedMode)
	}
}

func TestAnalyzeSearchModeAutoKeywordForSectionLookup(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "查找第一章 概述 讲了什么",
		SearchMode: SearchModeAuto,
	})
	if decision.ResolvedMode != SearchModeKeyword {
		t.Fatalf("expected keyword, got %q", decision.ResolvedMode)
	}
}

func TestAnalyzeSearchModeAutoHybrid(t *testing.T) {
	decision := AnalyzeSearchMode(Request{
		Query:      "nginx 404 报错怎么排查",
		SearchMode: SearchModeAuto,
	})
	if decision.ResolvedMode != SearchModeHybrid {
		t.Fatalf("expected hybrid, got %q", decision.ResolvedMode)
	}
}

func TestAnalyzeSearchModeSamples(t *testing.T) {
	testCases := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "concept question",
			query: "什么是 RAG 检索增强生成",
			want:  SearchModeSemantic,
		},
		{
			name:  "difference question",
			query: "向量检索和关键词检索有什么区别",
			want:  SearchModeSemantic,
		},
		{
			name:  "title contains exact phrase",
			query: "标题包含 \"Kubernetes\" 的文档",
			want:  SearchModeKeyword,
		},
		{
			name:  "named lookup",
			query: "有没有名称叫 GoAgent 的知识库文档",
			want:  SearchModeKeyword,
		},
		{
			name:  "error troubleshooting",
			query: "nginx 404 报错怎么排查",
			want:  SearchModeHybrid,
		},
		{
			name:  "api locator",
			query: "chat 接口的 timeout 参数在哪里配置",
			want:  SearchModeHybrid,
		},
		{
			name:  "code symbol lookup",
			query: "RagChatService.runRetrieveStage 是怎么工作的",
			want:  SearchModeHybrid,
		},
		{
			name:  "path lookup",
			query: "internal/app/rag/core/retrieve/service.go 里做了什么",
			want:  SearchModeHybrid,
		},
		{
			name:  "natural language how question",
			query: "如何理解 rewrite 和 retrieve 的关系",
			want:  SearchModeSemantic,
		},
		{
			name:  "architecture flow question",
			query: "retrieve 主链路整体流程是什么",
			want:  SearchModeSemantic,
		},
		{
			name:  "exact phrase with quotes",
			query: "搜索包含 \"tool_workflow\" 的 trace 节点",
			want:  SearchModeKeyword,
		},
		{
			name:  "identifier lookup without quotes",
			query: "搜索包含 fallback_to_general_model 的节点",
			want:  SearchModeKeyword,
		},
		{
			name:  "file name lookup",
			query: "查找文件名是 trace_handlers.go 的实现",
			want:  SearchModeKeyword,
		},
		{
			name:  "section lookup",
			query: "查找第一章 概述 讲了什么",
			want:  SearchModeKeyword,
		},
		{
			name:  "chapter title lookup",
			query: "搜索章节标题包含 概述 的文档",
			want:  SearchModeKeyword,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			decision := AnalyzeSearchMode(Request{
				Query:      tc.query,
				SearchMode: SearchModeAuto,
			})
			if decision.ResolvedMode != tc.want {
				t.Fatalf("query %q: expected %q, got %q (reason=%q, signals=%v)", tc.query, tc.want, decision.ResolvedMode, decision.Reason, decision.Signals)
			}
			if decision.Source != modeSourceAuto {
				t.Fatalf("query %q: expected auto source, got %q", tc.query, decision.Source)
			}
			if len(decision.Signals) == 0 {
				t.Fatalf("query %q: expected non-empty decision signals", tc.query)
			}
		})
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
	if len(result.SearchChannels) != 2 {
		t.Fatalf("expected 2 search channels, got %+v", result.SearchChannels)
	}
	if len(result.ChannelStats) != 2 {
		t.Fatalf("expected 2 channel stats, got %+v", result.ChannelStats)
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
	if len(result.ChannelStats) != 2 {
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
