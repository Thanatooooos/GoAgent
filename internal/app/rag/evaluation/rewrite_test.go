package evaluation

import (
	"testing"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

type rewriteStub struct {
	result ragrewrite.Result
}

func (s rewriteStub) Rewrite(question string) string {
	return s.result.RewrittenQuestion
}

func (s rewriteStub) RewriteWithSplit(string) ragrewrite.Result {
	return s.result
}

func (s rewriteStub) RewriteWithHistory(string, []convention.ChatMessage) ragrewrite.Result {
	return s.result
}

func TestEvaluateRewriteSamplesOffline(t *testing.T) {
	needTrue := true
	needFalse := false
	samples := []RewriteSample{
		{
			Name:           "keep_sigurg",
			Query:          "Go 异步抢占 SIGURG 是什么机制",
			Tags:           []string{"keyword_preserve"},
			RewrittenQuery: "Go 调度 SIGURG 异步抢占机制",
			SubQuestions:   []string{"Go 调度 SIGURG 异步抢占机制", "SIGURG 作用"},
			NeedRetrieval:  true,
			Expect: RewriteExpect{
				NeedRetrieval:     &needTrue,
				MustKeepTerms:     []string{"SIGURG"},
				SubQuestionCountMax: 4,
			},
		},
		{
			Name:           "drop_sigurg",
			Query:          "Go 异步抢占 SIGURG 是什么机制",
			Tags:           []string{"keyword_preserve"},
			RewrittenQuery: "Go 异步抢占原理",
			SubQuestions:   []string{"Go 异步抢占原理"},
			NeedRetrieval:  true,
			Expect: RewriteExpect{
				MustKeepTerms: []string{"SIGURG"},
			},
		},
		{
			Name:           "skip_hello",
			Query:          "你好",
			Tags:           []string{"skip_retrieval"},
			RewrittenQuery: "你好",
			SubQuestions:   []string{"你好"},
			NeedRetrieval:  false,
			Expect: RewriteExpect{
				NeedRetrieval: &needFalse,
			},
		},
	}

	summary, err := EvaluateRewriteSamples(samples)
	if err != nil {
		t.Fatalf("EvaluateRewriteSamples returned error: %v", err)
	}
	if summary.Overall.SampleCount != 3 {
		t.Fatalf("expected 3 samples, got %d", summary.Overall.SampleCount)
	}
	if summary.Overall.PassRate < 0.66 || summary.Overall.PassRate > 0.67 {
		t.Fatalf("expected pass rate 2/3, got %v", summary.Overall.PassRate)
	}
	if summary.Overall.TermPreservationRate != 0.5 {
		t.Fatalf("expected term preservation 0.5, got %v", summary.Overall.TermPreservationRate)
	}
	if summary.Overall.NeedRetrievalAccuracy != 1 {
		t.Fatalf("expected need retrieval accuracy 1, got %v", summary.Overall.NeedRetrievalAccuracy)
	}
}

func TestCheckMustNotStartWith(t *testing.T) {
	sample := RewriteSample{
		RewrittenQuery: "向量数据库应用场景",
		SubQuestions:   []string{"向量数据库有哪些用途"},
		Expect: RewriteExpect{
			MustNotStartWith: []string{"它", "这"},
		},
	}
	checks := checkRewriteExpect(sample)
	if !checks.MustNotStartWithOK {
		t.Fatal("expected pronoun check to pass")
	}
}

func TestExecuteRewriteSampleUsesHistory(t *testing.T) {
	stub := rewriteStub{
		result: ragrewrite.Result{
			RewrittenQuestion: "向量数据库有哪些应用场景",
			SubQuestions:      []string{"向量数据库有哪些应用场景"},
			NeedRetrieval:     true,
		},
	}
	sample := RewriteSample{
		Name:  "coref_vector_db",
		Query: "它有哪些应用场景",
		History: []RewriteHistoryMessage{
			{Role: "user", Content: "什么是向量数据库"},
			{Role: "assistant", Content: "向量数据库用于语义检索"},
		},
	}
	// rewriteStub ignores history but path should not error
	if err := ExecuteRewriteSample(t.Context(), &sample, stub); err != nil {
		t.Fatalf("ExecuteRewriteSample returned error: %v", err)
	}
	if sample.RewrittenQuery == "" {
		t.Fatal("expected rewritten query to be populated")
	}
}
