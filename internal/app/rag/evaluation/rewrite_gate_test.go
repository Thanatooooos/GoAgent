package evaluation

import "testing"

func TestEvaluateRewriteSoftGateRulePass(t *testing.T) {
	result := evaluateRewriteSoftGate(
		RewriteCheckResult{Passed: true},
		nil,
		nil,
		&RewriteSemanticEvaluation{SemanticScore: 0.9},
		&RewriteJudgeEvaluation{Score: 0.9},
	)
	if !result.Passed || result.PassPath != "rule" {
		t.Fatalf("result = %#v, want rule pass path", result)
	}
}

func TestEvaluateRewriteSoftGateOverridesMustContainAny(t *testing.T) {
	result := evaluateRewriteSoftGate(
		RewriteCheckResult{
			Passed:           false,
			MustContainAnyOK: false,
		},
		nil,
		[]string{"required rewrite group missing"},
		&RewriteSemanticEvaluation{SemanticScore: 0.85},
		&RewriteJudgeEvaluation{Score: 0.85},
	)
	if !result.Passed || result.PassPath != "semantic_judge" {
		t.Fatalf("result = %#v, want semantic_judge pass", result)
	}
	if len(result.OverriddenReasons) != 1 {
		t.Fatalf("OverriddenReasons = %#v, want one override", result.OverriddenReasons)
	}
}

func TestEvaluateRewriteSoftGateDoesNotOverrideSubQuestionCount(t *testing.T) {
	result := evaluateRewriteSoftGate(
		RewriteCheckResult{
			Passed:             false,
			SubQuestionCountOK: false,
		},
		nil,
		[]string{"sub question count out of range"},
		&RewriteSemanticEvaluation{SemanticScore: 0.95},
		&RewriteJudgeEvaluation{Score: 0.95},
	)
	if result.Passed {
		t.Fatalf("result = %#v, want hard block on sub question count", result)
	}
}

func TestEvaluateRewriteSoftGateDoesNotOverrideCriticalTerms(t *testing.T) {
	result := evaluateRewriteSoftGate(
		RewriteCheckResult{
			Passed:          false,
			CriticalTermsOK: false,
		},
		[]string{"critical_terms_missing"},
		[]string{"critical terms missing"},
		&RewriteSemanticEvaluation{SemanticScore: 0.95},
		&RewriteJudgeEvaluation{Score: 0.95},
	)
	if result.Passed {
		t.Fatalf("result = %#v, want hard block on critical terms", result)
	}
}

func TestEvaluateRewriteSoftGateRequiresBothSemanticAndJudge(t *testing.T) {
	result := evaluateRewriteSoftGate(
		RewriteCheckResult{Passed: false, MustContainAnyOK: false},
		nil,
		[]string{"required rewrite group missing"},
		&RewriteSemanticEvaluation{SemanticScore: 0.95},
		&RewriteJudgeEvaluation{Score: 0.4},
	)
	if result.Passed {
		t.Fatalf("result = %#v, want fail when judge below threshold", result)
	}
}
