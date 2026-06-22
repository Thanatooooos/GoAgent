package evaluation

import (
	"context"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestRunSummaryFieldJudge(t *testing.T) {
	judge := &stubJudge{
		results: []JudgeResult{
			{
				Passed: true,
				Score:  1,
				Details: map[string]any{
					"fields": map[string]any{
						"goal":              map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
						"constraints":       map[string]any{"fidelity": 1, "usefulness": 0.5, "reason": "ok"},
						"established_facts": map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
						"recent_progress":   map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
						"open_questions":    map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
					},
				},
			},
		},
	}

	result, err := RunSummaryFieldJudge(context.Background(), judge, SummarySample{
		Name: "summary-sample",
	}, raghistory.StructuredSummary{
		SchemaVersion: 1,
		Goal:          "当前主目标是先做 spec",
		Constraints:   []string{"当前不进入实现"},
	})
	if err != nil {
		t.Fatalf("RunSummaryFieldJudge() error = %v", err)
	}
	if !result.Passed {
		t.Fatal("field judge expected passed")
	}
	if len(result.Fields) != 5 {
		t.Fatalf("field count = %d, want 5", len(result.Fields))
	}
	if result.Fields["constraints"].Usefulness != 0.5 {
		t.Fatalf("constraints usefulness = %v, want 0.5", result.Fields["constraints"].Usefulness)
	}
}

func TestRunSummaryFieldJudgeAcceptsTopLevelFieldDetailsAndStringLists(t *testing.T) {
	judge := &stubJudge{
		results: []JudgeResult{
			{
				Passed: true,
				Score:  0.75,
				Details: map[string]any{
					"goal":              map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
					"constraints":       map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
					"established_facts": map[string]any{"fidelity": 1, "usefulness": 1, "reason": "ok"},
					"recent_progress":   map[string]any{"fidelity": 1, "usefulness": 0.5, "missed_items": "next step, baseline", "reason": "partial"},
					"open_questions":    map[string]any{"fidelity": 0.5, "usefulness": 0.5, "incorrect_claims": "claim-a, claim-b", "reason": "partial"},
				},
			},
		},
	}

	result, err := RunSummaryFieldJudge(context.Background(), judge, SummarySample{
		Name: "summary-sample",
	}, raghistory.StructuredSummary{
		SchemaVersion: 1,
		Goal:          "???????? spec",
		Constraints:   []string{"???????"},
	})
	if err != nil {
		t.Fatalf("RunSummaryFieldJudge() error = %v", err)
	}
	if len(result.Fields) != 5 {
		t.Fatalf("field count = %d, want 5", len(result.Fields))
	}
	if got := result.Fields["recent_progress"].MissedItems; len(got) != 2 || got[0] != "next step" || got[1] != "baseline" {
		t.Fatalf("recent_progress missed_items = %#v, want split strings", got)
	}
	if got := result.Fields["open_questions"].IncorrectClaims; len(got) != 2 || got[0] != "claim-a" || got[1] != "claim-b" {
		t.Fatalf("open_questions incorrect_claims = %#v, want split strings", got)
	}
}
