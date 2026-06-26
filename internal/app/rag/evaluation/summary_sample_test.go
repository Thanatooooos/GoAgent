package evaluation

import (
	"encoding/json"
	"testing"
)

func TestParseSummarySamples(t *testing.T) {
	rawSamples := []json.RawMessage{
		json.RawMessage(`{
			"name":"summary-sample-1",
			"tags":["goal_drift"],
			"input":{"source_messages":[{"role":"user","content":"先做 spec，不进实现"}]},
			"expected_summary":{
				"goal":{"must_cover":["当前主目标是先做 spec"]},
				"constraints":{"must_cover":["当前不进入实现"]}
			},
			"critical_contract":{
				"critical_constraints":["当前不进入实现"],
				"forbidden_claims":["已经开始实现"]
			},
			"next_turn_eval":{"queries":[{"id":"q1","query":"下一步做什么？","equivalence_expectations":["必须说明先做 spec"]}]},
			"metadata":{"author":"manual"}
		}`),
	}

	samples, err := ParseSummarySamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseSummarySamples() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("ParseSummarySamples() len = %d, want 1", len(samples))
	}
	if samples[0].Name != "summary-sample-1" {
		t.Fatalf("sample name = %q, want summary-sample-1", samples[0].Name)
	}
	if len(samples[0].Input.SourceMessages) != 1 {
		t.Fatalf("source messages len = %d, want 1", len(samples[0].Input.SourceMessages))
	}
}

func TestParseSummarySamplesRejectsMissingName(t *testing.T) {
	rawSamples := []json.RawMessage{
		json.RawMessage(`{
			"input":{"source_messages":[{"role":"user","content":"x"}]},
			"expected_summary":{"goal":{"must_cover":["x"]}},
			"critical_contract":{"forbidden_claims":["y"]}
		}`),
	}

	if _, err := ParseSummarySamples(rawSamples); err == nil {
		t.Fatal("ParseSummarySamples() expected error for missing name")
	}
}

func TestParseSummarySamplesSupportsStrategyEval(t *testing.T) {
	rawSamples, err := ExtractSampleArray(json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"},
				{"role":"user","content":"Q2"},
				{"role":"assistant","content":"A2"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":1,
					"expected_summary":{"goal":{"must_cover":["current goal"]}},
					"critical_contract":{},
					"next_turn_eval":{"queries":[{"id":"q1","query":"what is next?","equivalence_expectations":["must mention current goal"]}]}
				}]
			}
		}
	]`))
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	samples, err := ParseSummarySamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseSummarySamples() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples len = %d, want 1", len(samples))
	}
	if samples[0].StrategyEval == nil || len(samples[0].StrategyEval.Checkpoints) != 1 {
		t.Fatalf("unexpected strategy eval: %#v", samples[0].StrategyEval)
	}
	if samples[0].StrategyEval.Checkpoints[0].AfterTurn != 1 {
		t.Fatalf("after_turn = %d, want 1", samples[0].StrategyEval.Checkpoints[0].AfterTurn)
	}
}

func TestParseSummarySamplesNormalizesFinalEvalToConversationEnd(t *testing.T) {
	rawSamples, err := ExtractSampleArray(json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"},
				{"role":"user","content":"Q2"},
				{"role":"assistant","content":"A2"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":1,
					"expected_summary":{"goal":{"must_cover":["current goal"]}},
					"critical_contract":{},
					"next_turn_eval":{}
				}],
				"final_eval":{
					"expected_summary":{"goal":{"must_cover":["final goal"]}},
					"critical_contract":{},
					"next_turn_eval":{}
				}
			}
		}
	]`))
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	samples, err := ParseSummarySamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseSummarySamples() error = %v", err)
	}
	if samples[0].StrategyEval == nil || samples[0].StrategyEval.FinalEval == nil {
		t.Fatalf("unexpected strategy eval: %#v", samples[0].StrategyEval)
	}
	if samples[0].StrategyEval.FinalEval.AfterTurn != 2 {
		t.Fatalf("final_eval.after_turn = %d, want 2", samples[0].StrategyEval.FinalEval.AfterTurn)
	}
}

func TestParseSummarySamplesRejectsCheckpointPastConversationLength(t *testing.T) {
	rawSamples, err := ExtractSampleArray(json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":2,
					"expected_summary":{"goal":{"must_cover":["current goal"]}},
					"critical_contract":{},
					"next_turn_eval":{}
				}]
			}
		}
	]`))
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	if _, err := ParseSummarySamples(rawSamples); err == nil {
		t.Fatal("ParseSummarySamples() expected checkpoint validation error")
	}
}
