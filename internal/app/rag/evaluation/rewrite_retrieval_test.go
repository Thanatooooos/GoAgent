package evaluation

import (
	"context"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

func TestResolveRewriteKnowledgeBaseIDsPrefersSampleOverride(t *testing.T) {
	sample := RewriteSample{
		RetrievalExpectation: RewriteRetrievalExpectation{
			KnowledgeBaseIDs: []string{"kb-sample"},
		},
	}
	got := resolveRewriteKnowledgeBaseIDs(sample, []string{"kb-default"})
	if len(got) != 1 || got[0] != "kb-sample" {
		t.Fatalf("got %v, want sample override", got)
	}
}

func TestResolveRewriteKnowledgeBaseIDsUsesDefault(t *testing.T) {
	sample := RewriteSample{}
	got := resolveRewriteKnowledgeBaseIDs(sample, []string{"kb-default"})
	if len(got) != 1 || got[0] != "kb-default" {
		t.Fatalf("got %v, want default kb", got)
	}
}

func TestEvaluateRewriteRetrievalRecordsRetrievedIDsAndKnowledgeBaseScope(t *testing.T) {
	retrieve := &queryRetrieveService{
		results: map[string][]ragretrieve.Result{
			"那扩容规则呢": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-baseline", DocumentID: "doc-baseline", Score: 0.5}},
			}},
			"Go 1.18 之后 slice 扩容规则是什么": {{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-candidate", DocumentID: "doc-candidate", Score: 0.9}},
			}},
		},
	}
	sample := RewriteSample{
		Name:           "coref_go_slice_followup",
		Query:          "那扩容规则呢",
		RewrittenQuery: "Go 1.18 之后 slice 扩容规则是什么",
		SubQuestions:   []string{"Go 1.18 之后 slice 扩容规则是什么"},
		RetrievalExpectation: RewriteRetrievalExpectation{
			Target:              "chunk",
			ExpectedIDs:         []string{"chunk-candidate"},
			CriticalExpectedIDs: []string{"chunk-candidate"},
			TopK:                1,
			SearchMode:          "hybrid",
		},
	}

	comparison, err := EvaluateRewriteRetrieval(
		context.Background(),
		sample,
		retrieve,
		[]int{1},
		ragretrieve.SubQuestionOptions{},
		[]string{"kb-eval"},
	)
	if err != nil {
		t.Fatalf("EvaluateRewriteRetrieval() error = %v", err)
	}
	if comparison == nil {
		t.Fatal("expected retrieval comparison")
	}
	if len(comparison.KnowledgeBaseIDs) != 1 || comparison.KnowledgeBaseIDs[0] != "kb-eval" {
		t.Fatalf("KnowledgeBaseIDs = %v, want kb-eval scope", comparison.KnowledgeBaseIDs)
	}
	if len(retrieve.requests) != 2 {
		t.Fatalf("retrieve requests = %d, want baseline + candidate", len(retrieve.requests))
	}
	for _, req := range retrieve.requests {
		if len(req.KnowledgeBaseIDs) != 1 || req.KnowledgeBaseIDs[0] != "kb-eval" {
			t.Fatalf("request kb scope = %v, want kb-eval", req.KnowledgeBaseIDs)
		}
	}
	if len(comparison.BaselineRetrievedIDs) != 1 || comparison.BaselineRetrievedIDs[0] != "chunk-baseline" {
		t.Fatalf("BaselineRetrievedIDs = %v", comparison.BaselineRetrievedIDs)
	}
	if len(comparison.CandidateRetrievedIDs) != 1 || comparison.CandidateRetrievedIDs[0] != "chunk-candidate" {
		t.Fatalf("CandidateRetrievedIDs = %v", comparison.CandidateRetrievedIDs)
	}
	if !comparison.Passed {
		t.Fatalf("comparison = %+v, want pass", comparison)
	}
}
