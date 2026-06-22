package evaluation

import (
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestEvaluateSummaryRulesDetectsStateOverrideFailure(t *testing.T) {
	sample := SummarySample{
		Name: "state-override",
		Input: SummaryInput{
			PreviousSummary: &raghistory.StructuredSummary{
				SchemaVersion: 1,
				Goal:          "implement immediately",
				Constraints:   []string{"implementation is allowed now"},
			},
			SourceMessages: []SummaryMessage{{Role: "user", Content: "Draft the spec first. Do not implement yet."}},
		},
		ExpectedSummary: SummaryExpectedSummary{
			Goal:        SummaryExpectedField{MustCover: []string{"draft the spec first"}},
			Constraints: SummaryExpectedField{MustCover: []string{"do not implement yet"}},
		},
		CriticalContract: SummaryCriticalContract{
			CriticalConstraints: []string{"do not implement yet"},
		},
	}
	generated := raghistory.StructuredSummary{
		SchemaVersion:  1,
		Goal:           "implement immediately",
		Constraints:    []string{"do not implement yet", "implementation is allowed now"},
		RecentProgress: []string{"sample prep complete"},
	}

	result := EvaluateSummaryRules(sample, generated)

	if result.StateOverrideOK {
		t.Fatal("StateOverrideOK expected false")
	}
	if result.Passed {
		t.Fatal("Passed expected false when stale prior state is retained")
	}
	found := false
	for _, failure := range result.CriticalFailures {
		if failure == "stale_state_retained" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("CriticalFailures = %#v, want stale_state_retained", result.CriticalFailures)
	}
}
