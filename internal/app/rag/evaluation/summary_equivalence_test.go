package evaluation

import (
	"context"
	"testing"
)

func TestRunSummaryEquivalence(t *testing.T) {
	answerGen := &stubSummaryAnswerGenerator{
		outputs: []SummaryAnswerOutput{
			{Answer: "完整上下文答案"},
			{Answer: "摘要上下文答案"},
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{
			{
				Passed: true,
				Score:  1,
				Details: map[string]any{
					"dangerous_drift": false,
				},
			},
		},
	}

	result, err := RunSummaryEquivalence(context.Background(), answerGen, judge, SummarySample{
		Name: "summary-sample",
		Input: SummaryInput{
			SourceMessages: []SummaryMessage{
				{Role: "user", Content: "先做 spec，不进入实现。"},
			},
		},
		NextTurnEval: SummaryNextTurnEval{
			Queries: []SummaryNextTurnQuery{
				{
					ID:                     "q1",
					Query:                  "下一步做什么？",
					EquivalenceExpectations: []string{"必须说明先做 spec"},
				},
			},
		},
	}, SummaryGenerationOutput{
		Rendered: "目标：先做 spec",
	})
	if err != nil {
		t.Fatalf("RunSummaryEquivalence() error = %v", err)
	}
	if answerGen.calls != 2 {
		t.Fatalf("answer generator calls = %d, want 2", answerGen.calls)
	}
	if judge.calls != 1 {
		t.Fatalf("judge calls = %d, want 1", judge.calls)
	}
	if !result.Passed {
		t.Fatal("equivalence expected passed")
	}
	if len(result.Queries) != 1 {
		t.Fatalf("query result len = %d, want 1", len(result.Queries))
	}
	if answerGen.requests[0].Config.EnableTools {
		t.Fatal("full-context answer config should disable tools")
	}
	if answerGen.requests[0].Config.EnableRetrieval {
		t.Fatal("full-context answer config should disable retrieval")
	}
	if answerGen.requests[0].Config.EnableExternalCompensation {
		t.Fatal("full-context answer config should disable external compensation")
	}
}
