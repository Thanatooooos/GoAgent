package evaluation

import (
	"context"
	"fmt"
	"strings"
)

const rewriteQualityJudgeRef = "rewrite.quality.v1"

type RewriteJudgeDimensionScores struct {
	IntentPreservation    float64 `json:"intent_preservation"`
	StandaloneClarity     float64 `json:"standalone_clarity"`
	TermPreservation      float64 `json:"term_preservation"`
	SplitAppropriateness  float64 `json:"split_appropriateness"`
	RetrievalUsefulness   float64 `json:"retrieval_usefulness"`
}

type RewriteJudgeEvaluation struct {
	PromptRef  string                      `json:"prompt_ref"`
	RubricRef  string                      `json:"rubric_ref"`
	Passed     bool                        `json:"passed"`
	Score      float64                     `json:"score"`
	Reason     string                      `json:"reason,omitempty"`
	Dimensions RewriteJudgeDimensionScores `json:"dimensions"`
	MissedItems     []string                `json:"missed_items,omitempty"`
	IncorrectClaims []string                `json:"incorrect_claims,omitempty"`
}

func RunRewriteJudge(ctx context.Context, judge Judge, sample RewriteSample) (RewriteJudgeEvaluation, error) {
	if judge == nil {
		return RewriteJudgeEvaluation{}, fmt.Errorf("judge is required")
	}
	promptRef, rubricRef := resolveRewriteJudgeRefs(sample.Tags)
	result, err := judge.Evaluate(ctx, JudgeRequest{
		PromptRef: promptRef,
		RubricRef: rubricRef,
		Payload:   buildRewriteJudgePayload(sample),
		Config:    fixedRewriteJudgeConfig(),
	})
	if err != nil {
		return RewriteJudgeEvaluation{}, err
	}
	return RewriteJudgeEvaluation{
		PromptRef:       promptRef,
		RubricRef:       rubricRef,
		Passed:          result.Passed,
		Score:           roundSummaryScore(result.Score),
		Reason:          strings.TrimSpace(result.Reason),
		Dimensions:      decodeRewriteJudgeDimensions(result.Details),
		MissedItems:     append([]string(nil), result.MissedItems...),
		IncorrectClaims: append([]string(nil), result.IncorrectClaims...),
	}, nil
}

func resolveRewriteJudgeRefs(tags []string) (string, string) {
	ref := rewriteQualityJudgeRef
	for _, tag := range tags {
		switch strings.TrimSpace(tag) {
		case "coreference", "followup":
			return "rewrite.coref.v1", "rewrite.coref.v1"
		case "multi_intent", "split_required":
			return "rewrite.split.v1", "rewrite.split.v1"
		}
	}
	return ref, ref
}

func buildRewriteJudgePayload(sample RewriteSample) map[string]any {
	payload := map[string]any{
		"sample_name":      sample.Name,
		"tags":             append([]string(nil), sample.Tags...),
		"original_query":   sample.Query,
		"history":          sample.History,
		"rewritten_query":  sample.RewrittenQuery,
		"sub_questions":    append([]string(nil), sample.SubQuestions...),
		"need_retrieval":   sample.NeedRetrieval,
		"rewrite_expectation": map[string]any{
			"need_retrieval":        sample.Expect.NeedRetrieval,
			"must_keep_terms":       sample.Expect.MustKeepTerms,
			"must_keep_any_groups":  sample.Expect.MustKeepAnyGroups,
			"must_contain_any":      sample.Expect.MustContainAny,
			"must_not_start_with":   sample.Expect.MustNotStartWith,
			"critical_terms":        sample.Expect.CriticalTerms,
			"alias_groups":          sample.Expect.AliasGroups,
			"forbidden_rewrites":    sample.Expect.ForbiddenRewrites,
			"sub_question_count_min": sample.Expect.SubQuestionCountMin,
			"sub_question_count_max": sample.Expect.SubQuestionCountMax,
		},
	}
	if len(sample.Metadata) > 0 {
		payload["metadata"] = cloneAnyMap(sample.Metadata)
	}
	return payload
}

func decodeRewriteJudgeDimensions(details map[string]any) RewriteJudgeDimensionScores {
	if len(details) == 0 {
		return RewriteJudgeDimensionScores{}
	}
	raw, ok := details["dimensions"].(map[string]any)
	if !ok {
		raw = details
	}
	return RewriteJudgeDimensionScores{
		IntentPreservation:   readJudgeDimensionScore(raw, "intent_preservation"),
		StandaloneClarity:    readJudgeDimensionScore(raw, "standalone_clarity"),
		TermPreservation:     readJudgeDimensionScore(raw, "term_preservation"),
		SplitAppropriateness: readJudgeDimensionScore(raw, "split_appropriateness"),
		RetrievalUsefulness:  readJudgeDimensionScore(raw, "retrieval_usefulness"),
	}
}

func readJudgeDimensionScore(raw map[string]any, key string) float64 {
	if len(raw) == 0 {
		return 0
	}
	value, ok := raw[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return roundSummaryScore(typed)
	case float32:
		return roundSummaryScore(float64(typed))
	case int:
		return roundSummaryScore(float64(typed))
	case int64:
		return roundSummaryScore(float64(typed))
	default:
		return 0
	}
}

func fixedRewriteJudgeConfig() JudgeConfig {
	return JudgeConfig{
		Temperature: 0,
		MaxTokens:   900,
	}
}

func buildRewriteSemanticQualityScore(semantic *RewriteSemanticEvaluation, judge *RewriteJudgeEvaluation) float64 {
	switch {
	case semantic != nil && judge != nil:
		return roundSummaryScore(0.4*semantic.SemanticScore + 0.6*judge.Score)
	case judge != nil:
		return judge.Score
	case semantic != nil:
		return semantic.SemanticScore
	default:
		return 0
	}
}
