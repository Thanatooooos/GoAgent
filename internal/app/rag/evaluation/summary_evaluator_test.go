package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

type stubSummaryGenerator struct {
	output SummaryGenerationOutput
	calls  int
	last   SummaryGenerationInput
}

func (g *stubSummaryGenerator) Generate(_ context.Context, input SummaryGenerationInput) (SummaryGenerationOutput, error) {
	g.calls++
	g.last = input
	return g.output, nil
}

type stubJudge struct {
	calls    int
	requests []JudgeRequest
	results  []JudgeResult
}

func (j *stubJudge) Evaluate(_ context.Context, req JudgeRequest) (JudgeResult, error) {
	j.calls++
	j.requests = append(j.requests, req)
	if len(j.results) == 0 {
		return JudgeResult{}, nil
	}
	result := j.results[0]
	j.results = j.results[1:]
	return result, nil
}

type stubSummaryAnswerGenerator struct {
	calls    int
	requests []SummaryAnswerInput
	outputs  []SummaryAnswerOutput
}

func (g *stubSummaryAnswerGenerator) Answer(_ context.Context, input SummaryAnswerInput) (SummaryAnswerOutput, error) {
	g.calls++
	g.requests = append(g.requests, input)
	if len(g.outputs) == 0 {
		return SummaryAnswerOutput{}, nil
	}
	output := g.outputs[0]
	g.outputs = g.outputs[1:]
	return output, nil
}

func TestSummaryEvaluatorRun(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"summary-sample-1",
			"tags":["goal_drift"],
			"input":{"source_messages":[{"role":"user","content":"先做 spec，不进入实现"}]},
			"expected_summary":{
				"goal":{"must_cover":["当前主目标是先做 spec"]},
				"constraints":{"must_cover":["当前不进入实现"]}
			},
			"critical_contract":{
				"critical_constraints":["当前不进入实现"],
				"forbidden_claims":["已经开始实现"]
			},
			"next_turn_eval":{"queries":[{"id":"q1","query":"下一步做什么？","equivalence_expectations":["必须说明先做 spec"]}]}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{
				SchemaVersion: 1,
				Goal:          "当前主目标是先做 spec",
				Constraints:   []string{"当前不进入实现"},
				RecentProgress: []string{
					"正在整理评测规范",
				},
			},
			Rendered: `{"schema_version":1}`,
		},
	}
	evaluator := NewSummaryEvaluator(generator)

	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteSummary,
		InputPath:  "testdata/evals/summary/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
	if result.Suite != string(SuiteSummary) {
		t.Fatalf("result suite = %q, want %q", result.Suite, SuiteSummary)
	}
	if len(result.Samples) != 1 {
		t.Fatalf("result samples len = %d, want 1", len(result.Samples))
	}
	if result.Samples[0].Name != "summary-sample-1" {
		t.Fatalf("sample name = %q, want summary-sample-1", result.Samples[0].Name)
	}
	if !result.Samples[0].Passed {
		t.Fatal("sample expected passed")
	}
}

func TestSummaryEvaluatorRunsJudgeAndEquivalenceDespiteRuleFailure(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"summary-sample-1",
			"tags":["goal_drift"],
			"input":{"source_messages":[{"role":"user","content":"先做 spec，不进入实现。"}]},
			"expected_summary":{
				"goal":{"must_cover":["当前主目标是先做 spec"]},
				"constraints":{"must_cover":["当前不进入实现"],"must_not_claim":["已经开始实现"]}
			},
			"critical_contract":{
				"critical_constraints":["当前不进入实现"],
				"forbidden_claims":["已经开始实现"]
			},
			"next_turn_eval":{"queries":[{"id":"q1","query":"下一步做什么？","equivalence_expectations":["必须说明先做 spec"]}]}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{
				SchemaVersion: 1,
				Goal:          "当前主目标是直接写代码",
				Constraints:   []string{"已经开始实现"},
				RecentProgress: []string{
					"开始编码",
				},
			},
			Rendered: "渲染摘要",
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{
			{
				Passed: true,
				Score:  1,
				Details: map[string]any{
					"fields": map[string]any{
						"goal": map[string]any{"fidelity": 1, "usefulness": 1},
						"constraints": map[string]any{"fidelity": 1, "usefulness": 1},
						"established_facts": map[string]any{"fidelity": 1, "usefulness": 1},
						"recent_progress": map[string]any{"fidelity": 1, "usefulness": 1},
						"open_questions": map[string]any{"fidelity": 1, "usefulness": 1},
					},
				},
			},
			{
				Passed: true,
				Score:  1,
				Details: map[string]any{
					"dangerous_drift": false,
				},
			},
		},
	}
	answerGen := &stubSummaryAnswerGenerator{
		outputs: []SummaryAnswerOutput{
			{Answer: "下一步先做 spec"},
			{Answer: "下一步先做 spec"},
		},
	}
	evaluator := NewSummaryEvaluator(generator, WithSummaryJudge(judge), WithSummaryAnswerGenerator(answerGen))

	result, err := evaluator.Run(context.Background(), RunInput{
		Suite:      SuiteSummary,
		InputPath:  "testdata/evals/summary/samples.json",
		RawPayload: rawPayload,
		RawSamples: rawSamples,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if judge.calls != 2 {
		t.Fatalf("judge calls = %d, want 2", judge.calls)
	}
	if answerGen.calls != 2 {
		t.Fatalf("answer generator calls = %d, want 2", answerGen.calls)
	}
	if result.Samples[0].Passed {
		t.Fatal("sample should remain failed because rule checks already failed")
	}
}
