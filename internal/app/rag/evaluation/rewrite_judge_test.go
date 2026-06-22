package evaluation

import (
	"context"
	"testing"
)

func TestResolveRewriteJudgeRefsUsesCorefPromptForFollowup(t *testing.T) {
	promptRef, rubricRef := resolveRewriteJudgeRefs([]string{"coreference", "followup"})
	if promptRef != "rewrite.coref.v1" || rubricRef != "rewrite.coref.v1" {
		t.Fatalf("resolveRewriteJudgeRefs() = (%q, %q), want rewrite.coref.v1", promptRef, rubricRef)
	}
}

func TestRunRewriteJudgeUsesStubJudge(t *testing.T) {
	judge := &stubJudge{
		results: []JudgeResult{{
			Passed: true,
			Score:  0.92,
			Reason: "rewrite is standalone and preserves intent",
			Details: map[string]any{
				"dimensions": map[string]any{
					"intent_preservation":    0.95,
					"standalone_clarity":     0.9,
					"term_preservation":      0.85,
					"split_appropriateness":  1.0,
					"retrieval_usefulness":   0.88,
				},
			},
		}},
	}
	result, err := RunRewriteJudge(context.Background(), judge, RewriteSample{
		Name:           "coref-sample",
		Tags:           []string{"coreference", "followup"},
		Query:          "那持久化呢",
		RewrittenQuery: "Redis 持久化 AOF 和 RDB 机制",
		SubQuestions:   []string{"Redis 持久化 AOF 和 RDB 机制"},
		NeedRetrieval:  true,
	})
	if err != nil {
		t.Fatalf("RunRewriteJudge() error = %v", err)
	}
	if judge.calls != 1 {
		t.Fatalf("judge calls = %d, want 1", judge.calls)
	}
	if judge.requests[0].PromptRef != "rewrite.coref.v1" {
		t.Fatalf("PromptRef = %q, want rewrite.coref.v1", judge.requests[0].PromptRef)
	}
	if result.Score != 0.92 {
		t.Fatalf("Score = %v, want 0.92", result.Score)
	}
	if result.Dimensions.StandaloneClarity != 0.9 {
		t.Fatalf("StandaloneClarity = %v, want 0.9", result.Dimensions.StandaloneClarity)
	}
}

func TestBuildRewriteSemanticQualityScorePrefersJudgeWhenBothPresent(t *testing.T) {
	semantic := &RewriteSemanticEvaluation{SemanticScore: 0.8}
	judge := &RewriteJudgeEvaluation{Score: 0.9}
	got := buildRewriteSemanticQualityScore(semantic, judge)
	want := roundSummaryScore(0.4*0.8 + 0.6*0.9)
	if got != want {
		t.Fatalf("buildRewriteSemanticQualityScore() = %v, want %v", got, want)
	}
}
