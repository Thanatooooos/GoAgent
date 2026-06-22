package evaluation

import (
	"encoding/json"
	"testing"
)

func TestSuiteResultRoundTrip(t *testing.T) {
	input := SuiteResult{
		Suite: "summary",
		RunMetadata: RunMetadata{
			RunAt:            "2026-06-19T10:00:00Z",
			Suite:            "summary",
			EvaluatorVersion: "v1",
			SampleSetID:      "summary-sample-set",
			ModelConfig: map[string]any{
				"model": "judge-fixed",
			},
		},
		Samples: []SharedSampleResult{
			{
				Name:             "sample-1",
				Tags:             []string{"goal_drift"},
				Passed:           true,
				CriticalFailures: []string{},
				RuleChecks: map[string]any{
					"schema_valid": true,
				},
				Scores: map[string]any{
					"structured_fidelity": 1.0,
				},
			},
		},
		Aggregate: SharedAggregateResult{
			PassRate:            1,
			CriticalFailureRate: 0,
			ByTag: []TagAggregate{
				{
					Tag: "goal_drift",
					Metrics: map[string]any{
						"pass_rate": 1.0,
					},
				},
			},
			Metrics: map[string]any{
				"structured_fidelity": 1.0,
			},
		},
		Artifacts: map[string]any{
			"raw_summary": map[string]any{"goal": "test"},
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var got SuiteResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if got.Suite != input.Suite {
		t.Fatalf("Suite = %q, want %q", got.Suite, input.Suite)
	}
	if got.RunMetadata.Suite != "summary" {
		t.Fatalf("RunMetadata.Suite = %q, want summary", got.RunMetadata.Suite)
	}
	if len(got.Samples) != 1 {
		t.Fatalf("Samples len = %d, want 1", len(got.Samples))
	}
	if len(got.Aggregate.ByTag) != 1 {
		t.Fatalf("Aggregate.ByTag len = %d, want 1", len(got.Aggregate.ByTag))
	}
}
