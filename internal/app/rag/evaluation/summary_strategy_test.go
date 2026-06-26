package evaluation

import (
	"context"
	"strings"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestRunSummaryStrategySweepEvaluatesConfiguredThresholds(t *testing.T) {
	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{SchemaVersion: 1, Goal: "finish strategy mode"},
			Rendered:   "Goal: finish strategy mode",
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{
			{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}},
			{Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}},
			{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}},
			{Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}},
		},
	}
	answerGen := &stubSummaryAnswerGenerator{
		outputs: []SummaryAnswerOutput{
			{Answer: "finish strategy mode first"},
			{Answer: "finish strategy mode first"},
			{Answer: "finish strategy mode first"},
			{Answer: "finish strategy mode first"},
		},
	}
	sample := SummarySample{
		Name: "strategy-sample",
		Input: SummaryInput{SourceMessages: []SummaryMessage{
			{Role: "user", Content: "Q1"}, {Role: "assistant", Content: "A1"},
			{Role: "user", Content: "Q2"}, {Role: "assistant", Content: "A2"},
			{Role: "user", Content: "Q3"}, {Role: "assistant", Content: "A3"},
			{Role: "user", Content: "Q4"}, {Role: "assistant", Content: "A4"},
		}},
		StrategyEval: &SummaryStrategyEval{Checkpoints: []SummaryStrategyCheckpoint{{
			AfterTurn: 2,
			ExpectedSummary: SummaryExpectedSummary{
				Goal: SummaryExpectedField{MustCover: []string{"finish strategy mode"}},
			},
			CriticalContract: SummaryCriticalContract{},
			NextTurnEval: SummaryNextTurnEval{Queries: []SummaryNextTurnQuery{{
				ID:                     "q1",
				Query:                  "what is next?",
				EquivalenceExpectations: []string{"must mention strategy mode"},
			}}},
		}}},
	}

	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:       generator,
		Judge:           judge,
		AnswerGenerator: answerGen,
		Thresholds:      []int{2, 4},
	}, sample)
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if len(result.ThresholdResults) != 2 {
		t.Fatalf("threshold results len = %d, want 2", len(result.ThresholdResults))
	}
	if result.ThresholdResults[0].Threshold != 2 {
		t.Fatalf("first threshold = %d, want 2", result.ThresholdResults[0].Threshold)
	}
}


func TestRunSummaryStrategySweepUsesSummaryPlusTailContext(t *testing.T) {
	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{SchemaVersion: 1, Goal: "compressed"},
			Rendered:   "Goal: compressed",
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}}, {Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}}},
	}
	answerGen := &stubSummaryAnswerGenerator{outputs: []SummaryAnswerOutput{{Answer: "full"}, {Answer: "strategy"}}}
	sample := SummarySample{
		Name: "tail-sample",
		Input: SummaryInput{SourceMessages: []SummaryMessage{
			{Role: "user", Content: "turn1 user"}, {Role: "assistant", Content: "turn1 assistant"},
			{Role: "user", Content: "turn2 user"}, {Role: "assistant", Content: "turn2 assistant"},
		}},
		StrategyEval: &SummaryStrategyEval{Checkpoints: []SummaryStrategyCheckpoint{{
			AfterTurn: 2,
			ExpectedSummary: SummaryExpectedSummary{Goal: SummaryExpectedField{MustCover: []string{"compressed"}}},
			NextTurnEval: SummaryNextTurnEval{Queries: []SummaryNextTurnQuery{{ID: "q1", Query: "what is next?", EquivalenceExpectations: []string{"must stay aligned"}}}},
		}}},
	}

	_, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:       generator,
		Judge:           judge,
		AnswerGenerator: answerGen,
		Thresholds:      []int{1},
	}, sample)
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if len(answerGen.requests) != 2 {
		t.Fatalf("answer generator calls = %d, want 2", len(answerGen.requests))
	}
	if got := answerGen.requests[1].Context; !strings.Contains(got, "turn2 user") {
		t.Fatalf("strategy context = %q, want unsummarized tail content", got)
	}
}

func TestRunSummaryStrategySweepIncludesFinalEval(t *testing.T) {
	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{SchemaVersion: 1, Goal: "final state"},
			Rendered:   "Goal: final state",
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}}},
	}
	sample := SummarySample{
		Name: "final-sample",
		Input: SummaryInput{SourceMessages: []SummaryMessage{
			{Role: "user", Content: "turn1 user"}, {Role: "assistant", Content: "turn1 assistant"},
			{Role: "user", Content: "turn2 user"}, {Role: "assistant", Content: "turn2 assistant"},
		}},
		StrategyEval: &SummaryStrategyEval{
			Checkpoints: []SummaryStrategyCheckpoint{{AfterTurn: 1, ExpectedSummary: SummaryExpectedSummary{Goal: SummaryExpectedField{MustCover: []string{"final state"}}}}},
			FinalEval:   &SummaryStrategyCheckpoint{ExpectedSummary: SummaryExpectedSummary{Goal: SummaryExpectedField{MustCover: []string{"final state"}}}},
		},
	}

	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:  generator,
		Judge:      judge,
		Thresholds: []int{2},
	}, sample)
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if result.ThresholdResults[0].FinalResult == nil {
		t.Fatal("final_result expected")
	}
}
