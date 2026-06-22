package evaluation

import (
	"context"
	"math"
	"strings"
	"testing"
)

type stubQueryEmbedder struct {
	vectors map[string][]float32
}

func (s *stubQueryEmbedder) Embed(text string) ([]float32, error) {
	vectors, err := s.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (s *stubQueryEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if vector, ok := s.vectors[strings.TrimSpace(text)]; ok {
			vectors = append(vectors, append([]float32(nil), vector...))
			continue
		}
		vectors = append(vectors, []float32{0, 0})
	}
	return vectors, nil
}

func TestCosineSimilarityIdenticalVectors(t *testing.T) {
	vector := []float32{1, 2, 3}
	got, err := cosineSimilarity(vector, vector)
	if err != nil {
		t.Fatalf("cosineSimilarity() error = %v", err)
	}
	if math.Abs(got-1) > 1e-6 {
		t.Fatalf("cosineSimilarity() = %v, want 1", got)
	}
}

func TestBuildRewriteEmbeddingOriginalTextIncludesHistory(t *testing.T) {
	text := buildRewriteEmbeddingOriginalText(RewriteSample{
		Query: "那持久化呢",
		History: []RewriteHistoryMessage{
			{Role: "user", Content: "Redis 为什么能那么快"},
			{Role: "assistant", Content: "Redis 基于内存架构。"},
		},
	})
	if !strings.Contains(text, "Redis 为什么能那么快") {
		t.Fatalf("original text = %q, want history content", text)
	}
	if !strings.Contains(text, "user: 那持久化呢") {
		t.Fatalf("original text = %q, want current query", text)
	}
}

func TestEvaluateRewriteSemanticComputesRewriteAndSubScores(t *testing.T) {
	original := "user: Go slice 扩容规则\nuser: 那扩容规则呢"
	rewritten := "Go 1.18 slice 扩容规则是什么"
	subOne := "Go slice 扩容阈值策略"
	subTwo := "Go slice 内存对齐扩容"
	embedder := &stubQueryEmbedder{
		vectors: map[string][]float32{
			original:  {1, 0},
			rewritten: {1, 0},
			subOne:    {0.8, 0.6},
			subTwo:    {0.6, 0.8},
		},
	}

	result, err := EvaluateRewriteSemantic(context.Background(), embedder, RewriteSample{
		Name:  "semantic-sample",
		Query: "那扩容规则呢",
		History: []RewriteHistoryMessage{
			{Role: "user", Content: "Go slice 扩容规则"},
		},
		RewrittenQuery: rewritten,
		SubQuestions:   []string{subOne, subTwo},
	})
	if err != nil {
		t.Fatalf("EvaluateRewriteSemantic() error = %v", err)
	}
	if result.RewriteSimilarity != 1 {
		t.Fatalf("RewriteSimilarity = %v, want 1", result.RewriteSimilarity)
	}
	if len(result.SubQuestionSimilarities) != 2 {
		t.Fatalf("SubQuestionSimilarities len = %d, want 2", len(result.SubQuestionSimilarities))
	}
	if result.SemanticScore <= 0 {
		t.Fatalf("SemanticScore = %v, want > 0", result.SemanticScore)
	}
}
