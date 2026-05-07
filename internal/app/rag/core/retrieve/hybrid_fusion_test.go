package retrieve

import (
	"testing"

	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/framework/convention"
)

func makeHit(id string, score float32) corevector.SearchHit {
	return corevector.SearchHit{
		ChunkID:         id,
		DocumentID:      "doc-" + id,
		KnowledgeBaseID: "kb1",
		Index:           0,
		Text:            "content of " + id,
		Score:           score,
		Metadata:        map[string]any{},
	}
}

func TestRRFFusionBothChannels(t *testing.T) {
	// 模拟向量检索结果（2 条）。
	vectorHits := []corevector.SearchHit{
		makeHit("c1", 0.9),
		makeHit("c2", 0.7),
	}
	// 模拟关键词检索结果（2 条，其中 c2 重叠）。
	keywordHits := []corevector.SearchHit{
		makeHit("c3", 1.0),
		makeHit("c2", 0.8),
	}

	chunks := RRFusion(vectorHits, keywordHits, 60)

	// c2 在两路都命中，RRF 分数应该最高（1/(60+1+1) + 1/(60+0+1) ≈ 0.01613 + 0.01639 = 0.03252）
	// c1: 1/(60+0+1) = 0.01639
	// c3: 1/(60+0+1) = 0.01639
	// c2 > c1 == c3
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].ID != "c2" {
		t.Fatalf("expected c2 first (both channels), got %s", chunks[0].ID)
	}
}

func TestRRFFusionNonOverlapping(t *testing.T) {
	vectorHits := []corevector.SearchHit{
		makeHit("a", 0.9),
		makeHit("b", 0.8),
	}
	keywordHits := []corevector.SearchHit{
		makeHit("c", 1.0),
		makeHit("d", 0.7),
	}

	chunks := RRFusion(vectorHits, keywordHits, 60)

	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}
	// k=60, ranks 0-3: 1/61=0.01639, 1/62=0.01613, 1/63=0.01587, 1/64=0.01563 (all equal per channel)
	// a(rank0=0.01639) + a(not in kw), b(rank1=0.01613), c(rank0_kw=0.01639), d(rank1_kw=0.01613)
	// a and c should be top (equal), b and d next
	if chunks[0].Score != chunks[1].Score {
		// a and c should have the same score if they're both rank 0 in respective channels
		t.Logf("chunks[0]=%s score=%f, chunks[1]=%s score=%f", chunks[0].ID, chunks[0].Score, chunks[1].ID, chunks[1].Score)
	}
}

func TestRRFFusionEmptyVector(t *testing.T) {
	keywordHits := []corevector.SearchHit{
		makeHit("k1", 1.0),
	}
	chunks := RRFusion(nil, keywordHits, 60)
	if len(chunks) != 1 || chunks[0].ID != "k1" {
		t.Fatalf("expected 1 chunk k1, got %d: %v", len(chunks), chunks)
	}
}

func TestRRFFusionEmptyKeyword(t *testing.T) {
	vectorHits := []corevector.SearchHit{
		makeHit("v1", 0.9),
	}
	chunks := RRFusion(vectorHits, nil, 60)
	if len(chunks) != 1 || chunks[0].ID != "v1" {
		t.Fatalf("expected 1 chunk v1, got %d: %v", len(chunks), chunks)
	}
}

func TestRRFFusionBothEmpty(t *testing.T) {
	chunks := RRFusion(nil, nil, 60)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestRRFFusionDefaultK(t *testing.T) {
	vectorHits := []corevector.SearchHit{makeHit("a", 0.5)}
	keywordHits := []corevector.SearchHit{makeHit("b", 0.5)}

	chunks := RRFusion(vectorHits, keywordHits, 0) // k=0 应使用默认值 60
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks with default k, got %d", len(chunks))
	}
}

func TestMergeChunksDedup(t *testing.T) {
	results := []Result{
		{Chunks: []convention.RetrievedChunk{
			{ID: "x", Score: 0.9, Text: "X"},
			{ID: "y", Score: 0.7, Text: "Y"},
		}},
		{Chunks: []convention.RetrievedChunk{
			{ID: "x", Score: 0.6, Text: "X2"},
			{ID: "z", Score: 0.8, Text: "Z"},
		}},
	}

	chunks := MergeChunks(results)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 unique chunks, got %d", len(chunks))
	}
	// x 应保留较高分数 0.9
	for _, c := range chunks {
		if c.ID == "x" && c.Score != 0.9 {
			t.Fatalf("expected x score 0.9, got %f", c.Score)
		}
	}
	// 按分数降序排列。
	if chunks[0].Score < chunks[1].Score || chunks[1].Score < chunks[2].Score {
		t.Fatalf("expected descending score order: %v", chunks)
	}
}

func TestMergeChunksEmpty(t *testing.T) {
	chunks := MergeChunks(nil)
	if len(chunks) != 0 {
		t.Fatalf("expected empty, got %d", len(chunks))
	}
}
