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
