package retrieve

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestBuildRetrieveSubQuestionsKeepsOriginalFirst(t *testing.T) {
	subs := BuildRetrieveSubQuestions("SIGURG 异步抢占", []string{"Go 异步抢占原理", "SIGURG 信号作用"})
	if len(subs) != 3 {
		t.Fatalf("expected 3 sub questions, got %v", subs)
	}
	if subs[0] != "SIGURG 异步抢占" {
		t.Fatalf("expected original first, got %q", subs[0])
	}
}

func TestBuildRetrieveSubQuestionsSkipsDependencyRiskOriginalWhenRewritePresent(t *testing.T) {
	subs := BuildRetrieveSubQuestions("那扩容规则呢", []string{"Go 1.18 之后 slice 扩容规则是什么"})
	if len(subs) != 1 {
		t.Fatalf("expected 1 sub question, got %v", subs)
	}
	if subs[0] != "Go 1.18 之后 slice 扩容规则是什么" {
		t.Fatalf("expected rewrite-only retrieval, got %q", subs[0])
	}
}

func TestBuildRetrieveSubQuestionsDedupesAndCaps(t *testing.T) {
	subs := BuildRetrieveSubQuestions("q1", []string{"q1", "q2", "q3", "q4", "q5"})
	if len(subs) != 4 {
		t.Fatalf("expected cap at 4, got %d: %v", len(subs), subs)
	}
}

func TestMergeResultsPrefersOriginalSubQuestionWithRRF(t *testing.T) {
	merged := MergeResults([]Result{
		{
			Chunks: []convention.RetrievedChunk{
				{ID: "target", Score: 0.01},
				{ID: "noise-a", Score: 0.02},
			},
		},
		{
			Chunks: []convention.RetrievedChunk{
				{ID: "noise-b", Score: 0.99},
				{ID: "noise-c", Score: 0.98},
			},
		},
	}, 5)

	if len(merged.Chunks) == 0 {
		t.Fatal("expected merged chunks")
	}
	if merged.Chunks[0].ID != "target" {
		t.Fatalf("expected original-query hit first, got %+v", merged.Chunks)
	}
}
