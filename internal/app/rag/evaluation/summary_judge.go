package evaluation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	raghistory "local/rag-project/internal/app/rag/core/history"
)

type SummaryFieldJudgeResult struct {
	Fidelity        float64  `json:"fidelity"`
	Usefulness      float64  `json:"usefulness"`
	MissedItems     []string `json:"missed_items,omitempty"`
	IncorrectClaims []string `json:"incorrect_claims,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type SummaryFieldJudgeEvaluation struct {
	Fields           map[string]SummaryFieldJudgeResult `json:"fields"`
	FidelityPassed   bool                               `json:"fidelity_passed"`
	UsefulnessPassed bool                               `json:"usefulness_passed"`
	Passed           bool                               `json:"passed"`
	Score            float64                            `json:"score"`
	Reason           string                             `json:"reason,omitempty"`
}

func RunSummaryFieldJudge(ctx context.Context, judge Judge, sample SummarySample, generated raghistory.StructuredSummary) (SummaryFieldJudgeEvaluation, error) {
	if judge == nil {
		return SummaryFieldJudgeEvaluation{}, fmt.Errorf("judge is required")
	}
	result, err := judge.Evaluate(ctx, JudgeRequest{
		PromptRef: "summary.field.v1",
		RubricRef: "summary.field.v1",
		Payload: map[string]any{
			"sample_name":       sample.Name,
			"expected_summary":  sample.ExpectedSummary,
			"critical_contract": sample.CriticalContract,
			"generated_summary": generated,
		},
		Config: fixedSummaryFieldJudgeConfig(),
	})
	if err != nil {
		return SummaryFieldJudgeEvaluation{}, err
	}

	fields := decodeSummaryFieldJudgeResults(result.Details)
	evaluation := SummaryFieldJudgeEvaluation{
		Fields:           fields,
		FidelityPassed:   true,
		UsefulnessPassed: true,
		Passed:           result.Passed,
		Score:            result.Score,
		Reason:           result.Reason,
	}
	if len(fields) == 0 {
		return evaluation, nil
	}
	for _, field := range fields {
		if field.Fidelity < 0.5 {
			evaluation.FidelityPassed = false
		}
		if field.Usefulness < 0.5 {
			evaluation.UsefulnessPassed = false
		}
	}
	evaluation.Passed = evaluation.Passed && evaluation.FidelityPassed && evaluation.UsefulnessPassed
	return evaluation, nil
}

func decodeSummaryFieldJudgeResults(details map[string]any) map[string]SummaryFieldJudgeResult {
	if len(details) == 0 {
		return nil
	}
	rawFields := extractSummaryFieldDetails(details)
	if len(rawFields) == 0 {
		return nil
	}
	fields := make(map[string]SummaryFieldJudgeResult, len(rawFields))
	for name, raw := range rawFields {
		fieldMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fields[name] = SummaryFieldJudgeResult{
			Fidelity:        toFloat64(fieldMap["fidelity"]),
			Usefulness:      toFloat64(fieldMap["usefulness"]),
			MissedItems:     toStringSlice(fieldMap["missed_items"]),
			IncorrectClaims: toStringSlice(fieldMap["incorrect_claims"]),
			Reason:          toString(fieldMap["reason"]),
		}
	}
	return fields
}

func extractSummaryFieldDetails(details map[string]any) map[string]any {
	if rawFields, ok := details["fields"].(map[string]any); ok && len(rawFields) > 0 {
		return rawFields
	}

	candidateNames := []string{"goal", "constraints", "established_facts", "recent_progress", "open_questions"}
	fields := make(map[string]any)
	for _, name := range candidateNames {
		if value, ok := details[name]; ok {
			fields[name] = value
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func toFloat64(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := toString(item); text != "" {
				values = append(values, text)
			}
		}
		return values
	case string:
		return splitDelimitedText(typed)
	default:
		return nil
	}
}

func splitDelimitedText(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
