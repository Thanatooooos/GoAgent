package evaluation

const defaultRewriteJudgeThreshold = 0.65

type RewriteSoftGateResult struct {
	Enabled            bool     `json:"enabled"`
	Passed             bool     `json:"passed"`
	RulePassed         bool     `json:"rule_passed"`
	SemanticScore      float64  `json:"semantic_score,omitempty"`
	JudgeScore         float64  `json:"judge_score,omitempty"`
	PassPath           string   `json:"pass_path,omitempty"`
	OverriddenReasons  []string `json:"overridden_reasons,omitempty"`
	HardBlockedReasons []string `json:"hard_blocked_reasons,omitempty"`
}

func evaluateRewriteSoftGate(
	checks RewriteCheckResult,
	criticalFailures []string,
	failureReasons []string,
	semantic *RewriteSemanticEvaluation,
	judge *RewriteJudgeEvaluation,
) RewriteSoftGateResult {
	result := RewriteSoftGateResult{
		Enabled:    semantic != nil && judge != nil,
		RulePassed: checks.Passed,
	}
	if semantic != nil {
		result.SemanticScore = semantic.SemanticScore
	}
	if judge != nil {
		result.JudgeScore = judge.Score
	}
	if !result.Enabled {
		result.Passed = checks.Passed
		if checks.Passed {
			result.PassPath = "rule"
		}
		return result
	}

	semanticOK := semantic.SemanticScore >= defaultRewriteSemanticThreshold
	judgeOK := judge.Score >= defaultRewriteJudgeThreshold
	softQualityOK := semanticOK && judgeOK

	for _, failure := range append([]string(nil), criticalFailures...) {
		if isRewriteHardCriticalFailure(failure) {
			result.HardBlockedReasons = append(result.HardBlockedReasons, failure)
		}
	}
	for _, reason := range failureReasons {
		if isRewriteHardRuleFailure(reason) {
			result.HardBlockedReasons = append(result.HardBlockedReasons, reason)
		}
	}

	if len(result.HardBlockedReasons) > 0 {
		result.Passed = false
		result.PassPath = "failed"
		return result
	}

	if checks.Passed {
		result.Passed = true
		result.PassPath = "rule"
		return result
	}

	if !softQualityOK {
		result.Passed = false
		result.PassPath = "failed"
		return result
	}

	result.Passed = true
	result.PassPath = "semantic_judge"
	for _, reason := range failureReasons {
		if isRewriteSoftOverridableFailure(reason) {
			result.OverriddenReasons = append(result.OverriddenReasons, reason)
		}
	}
	return result
}

func isRewriteHardCriticalFailure(failure string) bool {
	switch failure {
	case "need_retrieval_mismatch", "critical_terms_missing", "forbidden_rewrite_present", "retrieval_must_not_regress", "critical_expected_ids_missing":
		return true
	default:
		return false
	}
}

func isRewriteHardRuleFailure(reason string) bool {
	switch reason {
	case "sub question count out of range", "constraint guard rejected rewrite":
		return true
	default:
		return false
	}
}

func isRewriteSoftOverridableFailure(reason string) bool {
	switch reason {
	case "required rewrite group missing", "required terms missing", "alias normalization expectation missing", "rewrite starts with forbidden prefix":
		return true
	default:
		return false
	}
}

func buildRewriteDiagnosticScore(ruleQuality float64, softGate RewriteSoftGateResult, retrievalImpact float64, hasRetrieval bool) float64 {
	rewriteComponent := ruleQuality
	if softGate.Enabled {
		rewriteComponent = buildRewriteSemanticQualityScoreFromScores(softGate.SemanticScore, softGate.JudgeScore)
		if softGate.RulePassed && rewriteComponent < ruleQuality {
			rewriteComponent = ruleQuality
		}
	}
	if !hasRetrieval {
		return roundSummaryScore(rewriteComponent)
	}
	return roundSummaryScore((rewriteComponent * 0.45) + (retrievalImpact * 0.55))
}

func buildRewriteSemanticQualityScoreFromScores(semanticScore, judgeScore float64) float64 {
	if semanticScore > 0 && judgeScore > 0 {
		return roundSummaryScore(0.4*semanticScore + 0.6*judgeScore)
	}
	if judgeScore > 0 {
		return judgeScore
	}
	return semanticScore
}
