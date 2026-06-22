package evaluation

func buildSummarySampleArtifact(
	sample SummarySample,
	generated SummaryGenerationOutput,
	rules SummaryRuleEvaluation,
	fieldJudge *SummaryFieldJudgeEvaluation,
	equivalence *SummaryEquivalenceEvaluation,
) map[string]any {
	artifact := map[string]any{
		"generated_summary":    generated.Structured,
		"rendered_summary":     generated.Rendered,
		"raw_summary":          generated.Raw,
		"rule_evaluation":      rules,
		"source_message_count": len(sample.Input.SourceMessages),
	}
	if diagnosticReasons := buildSummaryDiagnosticRuleReasons(rules); len(diagnosticReasons) > 0 {
		artifact["diagnostic_rule_reasons"] = diagnosticReasons
	}
	if sample.Input.PreviousSummary != nil {
		artifact["previous_summary"] = *sample.Input.PreviousSummary
	}
	if fieldJudge != nil {
		artifact["field_judge"] = *fieldJudge
	}
	if equivalence != nil {
		artifact["downstream_equivalence"] = *equivalence
	}
	return artifact
}

func buildSummaryDiagnosticRuleReasons(rules SummaryRuleEvaluation) []string {
	reasons := make([]string, 0, 4)
	if !rules.RequiredFieldsOK {
		reasons = append(reasons, "required summary fields missing")
	}
	if !rules.CriticalItemsOK {
		reasons = append(reasons, "critical contract items missing")
	}
	if !rules.OpenQuestionsOK {
		reasons = append(reasons, "open questions missing")
	}
	if !rules.CriticalOpenQuestionsOK {
		reasons = append(reasons, "critical open questions missing")
	}
	return reasons
}