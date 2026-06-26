package evaluation

import (
	"context"
	"encoding/json"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestSummaryEvaluatorEmitsArtifactsForDebugging(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"summary-sample-1",
			"tags":["goal_drift"],
			"input":{"source_messages":[{"role":"user","content":"Draft the spec first. Do not implement yet."}]},
			"expected_summary":{
				"goal":{"must_cover":["draft the spec first"]},
				"constraints":{"must_cover":["do not implement yet"]}
			},
			"critical_contract":{"critical_constraints":["do not implement yet"]},
			"next_turn_eval":{"queries":[{"id":"q1","query":"What should happen next?","equivalence_expectations":["draft the spec first"]}]}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	generator := &stubSummaryGenerator{output: SummaryGenerationOutput{
		Structured: raghistory.StructuredSummary{
			SchemaVersion:  1,
			Goal:           "draft the spec first",
			Constraints:    []string{"do not implement yet"},
			RecentProgress: []string{"artifact wiring in progress"},
		},
		Rendered: "rendered summary",
		Raw:      `{"schema_version":1}`,
	}}
	judge := &stubJudge{results: []JudgeResult{
		{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}},
		{Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}},
	}}
	answerGen := &stubSummaryAnswerGenerator{outputs: []SummaryAnswerOutput{{Answer: "draft the spec first"}, {Answer: "draft the spec first"}}}
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

	executions, ok := result.Artifacts["executions"].(map[string]any)
	if !ok {
		t.Fatalf("Artifacts = %#v, want executions map", result.Artifacts)
	}
	sampleArtifacts, ok := executions["summary-sample-1"].(map[string]any)
	if !ok {
		t.Fatalf("executions = %#v, want sample artifact map", executions)
	}
	for _, key := range []string{"generated_summary", "rendered_summary", "raw_summary", "field_judge", "downstream_equivalence"} {
		if _, ok := sampleArtifacts[key]; !ok {
			t.Fatalf("sampleArtifacts = %#v, want key %q", sampleArtifacts, key)
		}
	}
}

func TestBuildSummarySampleArtifactIncludesDiagnosticRuleReasons(t *testing.T) {
	artifact := buildSummarySampleArtifact(
		SummarySample{
			Input: SummaryInput{
				SourceMessages: []SummaryMessage{{Role: "user", Content: "Keep open questions visible for the next step."}},
			},
		},
		SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{
				SchemaVersion:    1,
				Goal:             "stabilize summary evaluation",
				Constraints:      []string{"keep schema stable"},
				EstablishedFacts: []string{"field judge remains the final semantic gate"},
			},
		},
		SummaryRuleEvaluation{
			SchemaValid:             true,
			RequiredFieldsOK:        true,
			ForbiddenClaimsOK:       true,
			CriticalEntitiesOK:      true,
			CriticalItemsOK:         true,
			OpenQuestionsOK:         false,
			CriticalOpenQuestionsOK: false,
			StateOverrideOK:         true,
			Passed:                  true,
		},
		nil,
		nil,
	)

	reasons, ok := artifact["diagnostic_rule_reasons"].([]string)
	if !ok {
		t.Fatalf("artifact = %#v, want diagnostic_rule_reasons", artifact)
	}
	if len(reasons) != 2 {
		t.Fatalf("diagnostic_rule_reasons = %#v, want 2 entries", reasons)
	}
	if reasons[0] != "open questions missing" {
		t.Fatalf("diagnostic_rule_reasons[0] = %q, want open questions missing", reasons[0])
	}
	if reasons[1] != "critical open questions missing" {
		t.Fatalf("diagnostic_rule_reasons[1] = %q, want critical open questions missing", reasons[1])
	}
}

func TestBuildSummaryStrategyArtifactIncludesThresholdResults(t *testing.T) {
	artifact := buildSummaryStrategyArtifact(SummaryStrategySampleResult{
		ThresholdResults: []SummaryStrategyThresholdResult{{
			Threshold:       1200,
			ThresholdUnit:   SummaryStrategyThresholdTokens,
			TokenSavedRatio: 0.5,
			Passed:          true,
		}},
	})
	results, ok := artifact["threshold_results"].([]SummaryStrategyThresholdResult)
	if !ok {
		t.Fatalf("artifact = %#v, want threshold_results", artifact)
	}
	if len(results) != 1 || results[0].Threshold != 1200 {
		t.Fatalf("threshold_results = %#v, want one threshold=1200 result", results)
	}
	if results[0].ThresholdUnit != SummaryStrategyThresholdTokens {
		t.Fatalf("ThresholdUnit = %q, want tokens", results[0].ThresholdUnit)
	}
}
