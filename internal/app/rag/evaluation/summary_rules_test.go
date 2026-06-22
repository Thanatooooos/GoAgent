package evaluation

import (
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

func TestEvaluateSummaryRulesDetectsCriticalFailures(t *testing.T) {
	sample := SummarySample{
		Name: "critical-failure-sample",
		ExpectedSummary: SummaryExpectedSummary{
			Goal: SummaryExpectedField{MustCover: []string{"draft spec first"}},
			Constraints: SummaryExpectedField{
				MustCover:    []string{"no implementation yet"},
				MustNotClaim: []string{"implementation already started"},
			},
		},
		CriticalContract: SummaryCriticalContract{
			CriticalEntities:    []string{"summary"},
			CriticalConstraints: []string{"no implementation yet"},
			ForbiddenClaims:     []string{"implementation already started"},
		},
		Input: SummaryInput{
			SourceMessages: []SummaryMessage{{Role: "user", Content: "Draft the summary spec first."}},
		},
	}
	generated := raghistory.StructuredSummary{
		SchemaVersion:  1,
		Goal:           "implementation already started",
		Constraints:    []string{"implementation already started"},
		RecentProgress: []string{"coding now"},
	}

	result := EvaluateSummaryRules(sample, generated)

	if !result.SchemaValid {
		t.Fatal("SchemaValid expected true because summary still satisfies base schema validation")
	}
	if result.ForbiddenClaimsOK {
		t.Fatal("ForbiddenClaimsOK expected false")
	}
	if result.CriticalEntitiesOK {
		t.Fatal("CriticalEntitiesOK expected false")
	}
	if result.Passed {
		t.Fatal("Passed expected false")
	}
	if len(result.CriticalFailures) == 0 {
		t.Fatal("CriticalFailures expected non-empty")
	}
}

func TestEvaluateSummaryRulesPassesValidSummary(t *testing.T) {
	sample := SummarySample{
		Name: "valid-summary",
		ExpectedSummary: SummaryExpectedSummary{
			Goal:             SummaryExpectedField{MustCover: []string{"finish summary eval design"}},
			Constraints:      SummaryExpectedField{MustCover: []string{"do not implement yet"}},
			EstablishedFacts: SummaryExpectedField{MustCover: []string{"phase one covers summary and rewrite"}},
		},
		CriticalContract: SummaryCriticalContract{
			CriticalEntities:    []string{"summary"},
			CriticalConstraints: []string{"do not implement yet"},
			CriticalFacts:       []string{"phase one covers summary and rewrite"},
			ForbiddenClaims:     []string{"implementation already started"},
		},
		Input: SummaryInput{
			SourceMessages: []SummaryMessage{{Role: "user", Content: "Design summary and rewrite evaluation first."}},
		},
	}
	generated := raghistory.StructuredSummary{
		SchemaVersion:    1,
		Goal:             "finish summary eval design",
		Constraints:      []string{"do not implement yet"},
		EstablishedFacts: []string{"phase one covers summary and rewrite", "samples are still in progress"},
		RecentProgress:   []string{"sample conventions drafted"},
	}

	result := EvaluateSummaryRules(sample, generated)

	if !result.SchemaValid {
		t.Fatal("SchemaValid expected true")
	}
	if !result.RequiredFieldsOK {
		t.Fatal("RequiredFieldsOK expected true")
	}
	if !result.ForbiddenClaimsOK {
		t.Fatal("ForbiddenClaimsOK expected true")
	}
	if !result.CriticalEntitiesOK {
		t.Fatal("CriticalEntitiesOK expected true")
	}
	if !result.StateOverrideOK {
		t.Fatal("StateOverrideOK expected true")
	}
	if !result.Passed {
		t.Fatal("Passed expected true")
	}
}

func TestEvaluateSummaryRulesTreatsMissingOpenQuestionsAsNonFatal(t *testing.T) {
	sample := SummarySample{
		Name: "missing-open-questions-is-soft",
		ExpectedSummary: SummaryExpectedSummary{
			Goal:             SummaryExpectedField{MustCover: []string{"stabilize summary evaluation"}},
			Constraints:      SummaryExpectedField{MustCover: []string{"keep existing summary schema"}},
			EstablishedFacts: SummaryExpectedField{MustCover: []string{"summary judge already returns field-level diagnostics"}},
			OpenQuestions:    SummaryExpectedField{MustCover: []string{"whether open_questions should block pass/fail is still undecided"}},
		},
		CriticalContract: SummaryCriticalContract{
			CriticalEntities:      []string{"summary"},
			CriticalConstraints:   []string{"keep existing summary schema"},
			CriticalFacts:         []string{"summary judge already returns field-level diagnostics"},
			CriticalOpenQuestions: []string{"whether open_questions should block pass/fail is still undecided"},
		},
		Input: SummaryInput{
			SourceMessages: []SummaryMessage{{Role: "user", Content: "Keep summary schema stable while we soften open-question gating."}},
		},
	}
	generated := raghistory.StructuredSummary{
		SchemaVersion:    1,
		Goal:             "stabilize summary evaluation",
		Constraints:      []string{"keep existing summary schema"},
		EstablishedFacts: []string{"summary judge already returns field-level diagnostics"},
		RecentProgress:   []string{"goal, constraints, and facts already score correctly"},
	}

	result := EvaluateSummaryRules(sample, generated)

	if !result.RequiredFieldsOK {
		t.Fatal("RequiredFieldsOK expected true because open_questions should not hard-fail required fields")
	}
	if !result.CriticalItemsOK {
		t.Fatal("CriticalItemsOK expected true because critical_open_questions should be diagnostic only")
	}
	if result.OpenQuestionsOK {
		t.Fatal("OpenQuestionsOK expected false when open_questions are missing")
	}
	if result.CriticalOpenQuestionsOK {
		t.Fatal("CriticalOpenQuestionsOK expected false when critical_open_questions are missing")
	}
	if !result.Passed {
		t.Fatal("Passed expected true because missing open_questions should be non-fatal")
	}
	if len(result.CriticalFailures) != 0 {
		t.Fatalf("CriticalFailures = %#v, want none", result.CriticalFailures)
	}
	if len(result.FailureReasons) != 0 {
		t.Fatalf("FailureReasons = %#v, want none", result.FailureReasons)
	}
}

func TestBuildSummaryScoreUsesWeightingModel(t *testing.T) {
	score := buildSummaryScore(
		SummaryRuleEvaluation{
			SchemaValid:        true,
			RequiredFieldsOK:   true,
			ForbiddenClaimsOK:  true,
			CriticalEntitiesOK: true,
			CriticalItemsOK:    true,
			StateOverrideOK:    true,
			Passed:             true,
		},
		&SummaryFieldJudgeEvaluation{
			Fields: map[string]SummaryFieldJudgeResult{
				"goal":              {Fidelity: 1, Usefulness: 1},
				"constraints":       {Fidelity: 0.5, Usefulness: 0.5},
				"established_facts": {Fidelity: 1, Usefulness: 0.5},
				"recent_progress":   {Fidelity: 0.5, Usefulness: 1},
				"open_questions":    {Fidelity: 1, Usefulness: 0.5},
			},
		},
		&SummaryEquivalenceEvaluation{
			Passed: true,
			Score:  0.5,
		},
	)

	if score["structured_fidelity"] != 0.8 {
		t.Fatalf("structured_fidelity = %v, want 0.8", score["structured_fidelity"])
	}
	if score["structured_usefulness"] != 0.7 {
		t.Fatalf("structured_usefulness = %v, want 0.7", score["structured_usefulness"])
	}
	if score["downstream_equivalence"] != 0.5 {
		t.Fatalf("downstream_equivalence = %v, want 0.5", score["downstream_equivalence"])
	}
	if score["diagnostic_score"] != 0.685 {
		t.Fatalf("diagnostic_score = %v, want 0.685", score["diagnostic_score"])
	}
	if score["gate_blocked"] != false {
		t.Fatalf("gate_blocked = %v, want false", score["gate_blocked"])
	}
}
