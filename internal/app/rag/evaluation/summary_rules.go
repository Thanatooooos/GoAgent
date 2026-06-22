package evaluation

import (
	"math"
	"strings"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

type SummaryRuleEvaluation struct {
	SchemaValid             bool     `json:"schema_valid"`
	RequiredFieldsOK        bool     `json:"required_fields_ok"`
	ForbiddenClaimsOK       bool     `json:"forbidden_claims_ok"`
	CriticalEntitiesOK      bool     `json:"critical_entities_ok"`
	CriticalItemsOK         bool     `json:"critical_items_ok"`
	OpenQuestionsOK         bool     `json:"open_questions_ok"`
	CriticalOpenQuestionsOK bool     `json:"critical_open_questions_ok"`
	StateOverrideOK         bool     `json:"state_override_ok"`
	Passed                  bool     `json:"passed"`
	CriticalFailures        []string `json:"critical_failures,omitempty"`
	FailureReasons          []string `json:"failure_reasons,omitempty"`
}

func EvaluateSummaryRules(sample SummarySample, generated raghistory.StructuredSummary) SummaryRuleEvaluation {
	generated.Normalize()
	result := SummaryRuleEvaluation{
		SchemaValid:             evaluateSummarySchema(sample, generated),
		RequiredFieldsOK:        evaluateRequiredFields(sample, generated),
		ForbiddenClaimsOK:       evaluateForbiddenClaims(sample, generated),
		CriticalEntitiesOK:      evaluateCriticalEntities(sample, generated),
		CriticalItemsOK:         evaluateCriticalItems(sample, generated),
		OpenQuestionsOK:         evaluateOpenQuestions(sample, generated),
		CriticalOpenQuestionsOK: evaluateCriticalOpenQuestions(sample, generated),
		StateOverrideOK:         evaluateStateOverride(sample, generated),
	}

	if !result.SchemaValid {
		result.FailureReasons = append(result.FailureReasons, "schema validation failed")
		result.CriticalFailures = append(result.CriticalFailures, "schema_validation_failed")
	}
	if !result.RequiredFieldsOK {
		result.FailureReasons = append(result.FailureReasons, "required summary fields missing (diagnostic only; judge owns final verdict)")
	}
	if !result.ForbiddenClaimsOK {
		result.FailureReasons = append(result.FailureReasons, "forbidden claims present")
		result.CriticalFailures = append(result.CriticalFailures, "forbidden_claim_present")
	}
	if !result.CriticalEntitiesOK {
		result.FailureReasons = append(result.FailureReasons, "critical entities missing")
		result.CriticalFailures = append(result.CriticalFailures, "critical_entities_missing")
	}
	if !result.CriticalItemsOK {
		result.FailureReasons = append(result.FailureReasons, "critical contract items missing (diagnostic only; judge owns final verdict)")
	}
	if !result.StateOverrideOK {
		result.FailureReasons = append(result.FailureReasons, "stale superseded state retained")
		result.CriticalFailures = append(result.CriticalFailures, "stale_state_retained")
	}

	// CriticalItemsOK and RequiredFieldsOK are no longer rule-level gates.
	// Critical items coverage and field completeness are now evaluated by
	// the field-level judge, which checks semantic quality rather than
	// exact substring match or binary non-empty checks.
	result.Passed = result.SchemaValid &&
		result.ForbiddenClaimsOK &&
		result.CriticalEntitiesOK &&
		result.StateOverrideOK
	return result
}

func evaluateSummarySchema(sample SummarySample, generated raghistory.StructuredSummary) bool {
	validation := raghistory.ValidateStructuredSummary(generated, sample.ToDomainMessages())
	return validation.Accepted
}

func evaluateRequiredFields(sample SummarySample, generated raghistory.StructuredSummary) bool {
	checks := []struct {
		field    SummaryExpectedField
		hasValue bool
	}{
		{field: sample.ExpectedSummary.Goal, hasValue: strings.TrimSpace(generated.Goal) != ""},
		{field: sample.ExpectedSummary.UserPreferences, hasValue: len(generated.UserPreferences) > 0},
		{field: sample.ExpectedSummary.Constraints, hasValue: len(generated.Constraints) > 0},
		{field: sample.ExpectedSummary.EstablishedFacts, hasValue: len(generated.EstablishedFacts) > 0},
		{field: sample.ExpectedSummary.RecentProgress, hasValue: len(generated.RecentProgress) > 0},
	}

	for _, check := range checks {
		if len(check.field.MustCover) > 0 && !check.hasValue {
			return false
		}
	}
	return true
}

func evaluateForbiddenClaims(sample SummarySample, generated raghistory.StructuredSummary) bool {
	text := renderSummarySearchText(generated)
	for _, claim := range summaryForbiddenClaims(sample) {
		if containsNormalized(text, claim) {
			return false
		}
	}
	return true
}

func evaluateCriticalEntities(sample SummarySample, generated raghistory.StructuredSummary) bool {
	text := renderSummarySearchText(generated)
	for _, entity := range sample.CriticalContract.CriticalEntities {
		if !containsNormalized(text, entity) {
			return false
		}
	}
	return true
}

func evaluateCriticalItems(sample SummarySample, generated raghistory.StructuredSummary) bool {
	text := renderSummarySearchText(generated)
	criticalValues := [][]string{
		sample.CriticalContract.CriticalConstraints,
		sample.CriticalContract.CriticalFacts,
		sample.CriticalContract.CriticalProgress,
	}
	for _, values := range criticalValues {
		for _, value := range values {
			if !containsNormalized(text, value) {
				return false
			}
		}
	}
	return true
}

func evaluateOpenQuestions(sample SummarySample, generated raghistory.StructuredSummary) bool {
	if len(sample.ExpectedSummary.OpenQuestions.MustCover) == 0 {
		return true
	}
	return len(generated.OpenQuestions) > 0
}

func evaluateCriticalOpenQuestions(sample SummarySample, generated raghistory.StructuredSummary) bool {
	text := renderSummarySearchText(generated)
	for _, value := range sample.CriticalContract.CriticalOpenQuestions {
		if !containsNormalized(text, value) {
			return false
		}
	}
	return true
}

func evaluateStateOverride(sample SummarySample, generated raghistory.StructuredSummary) bool {
	if sample.Input.PreviousSummary == nil {
		return true
	}
	previous := *sample.Input.PreviousSummary
	previous.Normalize()

	checks := []bool{
		evaluateFieldOverride([]string{previous.Goal}, sample.ExpectedSummary.Goal.MustCover, []string{generated.Goal}),
		evaluateFieldOverride(previous.UserPreferences, sample.ExpectedSummary.UserPreferences.MustCover, generated.UserPreferences),
		evaluateFieldOverride(previous.Constraints, sample.ExpectedSummary.Constraints.MustCover, generated.Constraints),
		evaluateFieldOverride(previous.EstablishedFacts, sample.ExpectedSummary.EstablishedFacts.MustCover, generated.EstablishedFacts),
		evaluateFieldOverride(previous.RecentProgress, sample.ExpectedSummary.RecentProgress.MustCover, generated.RecentProgress),
		evaluateFieldOverride(previous.OpenQuestions, sample.ExpectedSummary.OpenQuestions.MustCover, generated.OpenQuestions),
	}
	for _, ok := range checks {
		if !ok {
			return false
		}
	}
	return true
}

func evaluateFieldOverride(previousValues, requiredCurrentValues, generatedValues []string) bool {
	previousValues = normalizeSummaryValues(previousValues)
	requiredCurrentValues = normalizeSummaryValues(requiredCurrentValues)
	generatedValues = normalizeSummaryValues(generatedValues)
	if len(previousValues) == 0 || len(requiredCurrentValues) == 0 {
		return true
	}
	for _, previous := range previousValues {
		if containsValue(requiredCurrentValues, previous) {
			continue
		}
		if containsValue(generatedValues, previous) {
			return false
		}
	}
	return true
}

func containsValue(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, value := range values {
		if containsNormalized(strings.ToLower(value), target) {
			return true
		}
	}
	return false
}

func renderSummarySearchText(summary raghistory.StructuredSummary) string {
	parts := []string{summary.Goal}
	parts = append(parts, summary.ActivePriorities...)
	parts = append(parts, summary.UserPreferences...)
	parts = append(parts, summary.Constraints...)
	parts = append(parts, summary.EstablishedFacts...)
	parts = append(parts, summary.RecentProgress...)
	parts = append(parts, summary.OpenQuestions...)
	parts = append(parts, summary.BackgroundIssues...)
	return strings.ToLower(strings.Join(parts, "\n"))
}

func containsNormalized(text string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	return strings.Contains(text, value)
}

func summaryForbiddenClaims(sample SummarySample) []string {
	values := []string{}
	values = append(values, sample.ExpectedSummary.Goal.MustNotClaim...)
	values = append(values, sample.ExpectedSummary.UserPreferences.MustNotClaim...)
	values = append(values, sample.ExpectedSummary.Constraints.MustNotClaim...)
	values = append(values, sample.ExpectedSummary.EstablishedFacts.MustNotClaim...)
	values = append(values, sample.ExpectedSummary.RecentProgress.MustNotClaim...)
	values = append(values, sample.ExpectedSummary.OpenQuestions.MustNotClaim...)
	values = append(values, sample.CriticalContract.ForbiddenClaims...)
	return normalizeSummaryValues(values)
}

func buildSummaryScore(
	ruleResult SummaryRuleEvaluation,
	fieldJudge *SummaryFieldJudgeEvaluation,
	equivalence *SummaryEquivalenceEvaluation,
) map[string]any {
	structuredFidelity := summaryFidelityScore(ruleResult, fieldJudge)
	structuredUsefulness := summaryUsefulnessScore(ruleResult, fieldJudge)
	downstreamEquivalence := 0.0
	if equivalence != nil {
		downstreamEquivalence = clampSummaryScore(equivalence.Score)
	}
	diagnosticScore := roundSummaryScore(
		(structuredFidelity * 0.45) +
			(structuredUsefulness * 0.25) +
			(downstreamEquivalence * 0.30),
	)

	return map[string]any{
		"structured_fidelity":              structuredFidelity,
		"structured_usefulness":            structuredUsefulness,
		"downstream_equivalence":           downstreamEquivalence,
		"field_judge_available":            fieldJudge != nil,
		"downstream_equivalence_available": equivalence != nil,
		"diagnostic_score":                 diagnosticScore,
		"gate_blocked":                     len(ruleResult.CriticalFailures) > 0,
	}
}

func summaryFidelityScore(ruleResult SummaryRuleEvaluation, fieldJudge *SummaryFieldJudgeEvaluation) float64 {
	if !ruleResult.SchemaValid || !ruleResult.ForbiddenClaimsOK || !ruleResult.CriticalEntitiesOK || !ruleResult.StateOverrideOK {
		return 0
	}
	if fieldJudge == nil {
		return 1
	}
	return averageSummaryFieldScore(fieldJudge.Fields, func(result SummaryFieldJudgeResult) float64 {
		return result.Fidelity
	})
}

func summaryUsefulnessScore(ruleResult SummaryRuleEvaluation, fieldJudge *SummaryFieldJudgeEvaluation) float64 {
	// RequiredFieldsOK is now a diagnostic signal, not a score zeroing condition.
	// The judge evaluates whether the summary is useful regardless of field presence.
	if fieldJudge == nil {
		return 1
	}
	return averageSummaryFieldScore(fieldJudge.Fields, func(result SummaryFieldJudgeResult) float64 {
		return result.Usefulness
	})
}

func averageSummaryFieldScore(
	fields map[string]SummaryFieldJudgeResult,
	project func(SummaryFieldJudgeResult) float64,
) float64 {
	if len(fields) == 0 {
		return 0
	}
	total := 0.0
	count := 0
	for _, field := range fields {
		total += clampSummaryScore(project(field))
		count++
	}
	if count == 0 {
		return 0
	}
	return roundSummaryScore(total / float64(count))
}

func clampSummaryScore(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return roundSummaryScore(value)
	}
}

func roundSummaryScore(value float64) float64 {
	return math.Round(value*1000) / 1000
}


