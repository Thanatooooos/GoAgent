package evaluation

import (
	"fmt"
	"strings"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

type RewriteHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RewriteExpect struct {
	NeedRetrieval       *bool      `json:"needRetrieval,omitempty"`
	MustKeepTerms       []string   `json:"mustKeepTerms,omitempty"`
	MustKeepAnyGroups   [][]string `json:"mustKeepAnyGroups,omitempty"`
	MustContainAny      [][]string `json:"mustContainAny,omitempty"`
	MustNotStartWith    []string   `json:"mustNotStartWith,omitempty"`
	SubQuestionCountMin int        `json:"subQuestionCountMin,omitempty"`
	SubQuestionCountMax int        `json:"subQuestionCountMax,omitempty"`
	CriticalTerms       []string   `json:"criticalTerms,omitempty"`
	AliasGroups         [][]string `json:"aliasGroups,omitempty"`
	ForbiddenRewrites   []string   `json:"forbiddenRewrites,omitempty"`
}

type RewriteRetrievalExpectation struct {
	Target              string   `json:"target,omitempty"`
	ExpectedIDs         []string `json:"expectedIds,omitempty"`
	CriticalExpectedIDs []string `json:"criticalExpectedIds,omitempty"`
	KnowledgeBaseIDs    []string `json:"knowledgeBaseIds,omitempty"`
	TopK                int      `json:"topK,omitempty"`
	SearchMode          string   `json:"searchMode,omitempty"`
	MustNotRegress      bool     `json:"mustNotRegress,omitempty"`
}

type RewriteSample struct {
	Name           string                  `json:"name"`
	Query          string                  `json:"query"`
	Tags           []string                `json:"tags,omitempty"`
	History        []RewriteHistoryMessage `json:"history,omitempty"`
	Expect         RewriteExpect           `json:"expect"`
	RewrittenQuery string                  `json:"rewrittenQuery,omitempty"`
	SubQuestions   []string                `json:"subQuestions,omitempty"`
	NeedRetrieval  bool                    `json:"needRetrieval,omitempty"`
	Metadata       map[string]any          `json:"metadata,omitempty"`
	RetrievalExpectation RewriteRetrievalExpectation `json:"retrievalExpectation,omitempty"`
}

type RewriteCheckResult struct {
	TermPreservation        bool     `json:"termPreservation"`
	MissingTerms            []string `json:"missingTerms,omitempty"`
	NeedRetrievalMatch      bool     `json:"needRetrievalMatch"`
	NeedRetrievalEvaluated  bool     `json:"needRetrievalEvaluated,omitempty"`
	SubQuestionCountOK      bool     `json:"subQuestionCountOk"`
	MustContainAnyOK        bool     `json:"mustContainAnyOk"`
	MustNotStartWithOK      bool     `json:"mustNotStartWithOk"`
	ConstraintGuardOK       bool     `json:"constraintGuardOk"`
	ConstraintGuardChecked  bool     `json:"constraintGuardChecked,omitempty"`
	TermPreservationChecked bool     `json:"termPreservationChecked,omitempty"`
	CriticalTermsOK         bool     `json:"criticalTermsOk"`
	AliasNormalizationOK    bool     `json:"aliasNormalizationOk"`
	ForbiddenRewritesOK     bool     `json:"forbiddenRewritesOk"`
	Passed                  bool     `json:"passed"`
}

type RewriteSampleResult struct {
	Name           string             `json:"name"`
	Query          string             `json:"query"`
	Tags           []string           `json:"tags,omitempty"`
	RewrittenQuery string             `json:"rewrittenQuery,omitempty"`
	SubQuestions   []string           `json:"subQuestions,omitempty"`
	NeedRetrieval  bool               `json:"needRetrieval,omitempty"`
	Checks         RewriteCheckResult `json:"checks"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
}

type RewriteAggregateMetrics struct {
	SampleCount               int     `json:"sampleCount"`
	PassRate                  float64 `json:"passRate"`
	TermPreservationRate      float64 `json:"termPreservationRate"`
	NeedRetrievalAccuracy     float64 `json:"needRetrievalAccuracy"`
	SubQuestionComplianceRate float64 `json:"subQuestionComplianceRate"`
	ConstraintGuardPassRate   float64 `json:"constraintGuardPassRate"`
}

type RewriteTagSummary struct {
	Tag     string                  `json:"tag"`
	Metrics RewriteAggregateMetrics `json:"metrics"`
}

type RewriteSummary struct {
	Overall RewriteAggregateMetrics `json:"overall"`
	ByTag   []RewriteTagSummary     `json:"byTag"`
	Samples []RewriteSampleResult   `json:"samples"`
}

func ApplyRewriteResult(sample *RewriteSample, result ragrewrite.Result) {
	if sample == nil {
		return
	}
	sample.RewrittenQuery = strings.TrimSpace(result.RewrittenQuestion)
	sample.SubQuestions = append([]string(nil), result.SubQuestions...)
	sample.NeedRetrieval = result.NeedRetrieval
	if len(result.Metadata) > 0 {
		sample.Metadata = cloneAnyMap(result.Metadata)
	}
}

func EvaluateRewriteSamples(samples []RewriteSample) (RewriteSummary, error) {
	results := make([]RewriteSampleResult, 0, len(samples))
	for _, sample := range samples {
		result, err := evaluateRewriteSample(sample)
		if err != nil {
			return RewriteSummary{}, err
		}
		results = append(results, result)
	}
	return RewriteSummary{
		Overall: aggregateRewriteResults(results),
		ByTag:   buildRewriteTagSummaries(results),
		Samples: results,
	}, nil
}

func evaluateRewriteSample(sample RewriteSample) (RewriteSampleResult, error) {
	name := strings.TrimSpace(sample.Name)
	if name == "" {
		return RewriteSampleResult{}, fmt.Errorf("rewrite sample name is required")
	}
	query := strings.TrimSpace(sample.Query)
	if query == "" {
		return RewriteSampleResult{}, fmt.Errorf("rewrite sample %q query is required", name)
	}

	checks := checkRewriteExpect(sample)
	return RewriteSampleResult{
		Name:           name,
		Query:          query,
		Tags:           normalizeTags(sample.Tags),
		RewrittenQuery: strings.TrimSpace(sample.RewrittenQuery),
		SubQuestions:   append([]string(nil), sample.SubQuestions...),
		NeedRetrieval:  sample.NeedRetrieval,
		Checks:         checks,
		Metadata:       cloneAnyMap(sample.Metadata),
	}, nil
}

func checkRewriteExpect(sample RewriteSample) RewriteCheckResult {
	combined := rewriteCandidateText(sample.RewrittenQuery, sample.SubQuestions)
	termChecked := len(sample.Expect.MustKeepTerms) > 0 || len(sample.Expect.MustKeepAnyGroups) > 0
	checks := RewriteCheckResult{
		TermPreservation:         !termChecked,
		TermPreservationChecked:  termChecked,
		NeedRetrievalMatch:       checkNeedRetrieval(sample),
		NeedRetrievalEvaluated:   sample.Expect.NeedRetrieval != nil,
		SubQuestionCountOK:       checkSubQuestionCount(len(sample.SubQuestions), sample.Expect),
		MustContainAnyOK:         checkAnyGroups(combined, sample.Expect.MustContainAny),
		MustNotStartWithOK:       checkMustNotStartWith(sample.RewrittenQuery, sample.SubQuestions, sample.Expect.MustNotStartWith),
		ConstraintGuardOK:        true,
		ConstraintGuardChecked:   hasConstraintGuardMetadata(sample.Metadata),
		CriticalTermsOK:          checkMustKeepTerms(combined, sample.Expect.CriticalTerms),
		AliasNormalizationOK:     checkAnyGroups(combined, sample.Expect.AliasGroups),
		ForbiddenRewritesOK:      checkForbiddenRewrites(combined, sample.Expect.ForbiddenRewrites),
	}
	if termChecked {
		checks.TermPreservation = checkMustKeepTerms(combined, sample.Expect.MustKeepTerms)
		for _, group := range sample.Expect.MustKeepAnyGroups {
			if !anyTermPresent(combined, group) {
				checks.TermPreservation = false
				checks.MissingTerms = append(checks.MissingTerms, strings.Join(group, "|"))
			}
		}
		if !checks.TermPreservation && len(sample.Expect.MustKeepTerms) > 0 {
			checks.MissingTerms = append(checks.MissingTerms, missingTerms(combined, sample.Expect.MustKeepTerms)...)
		}
	}
	if checks.ConstraintGuardChecked {
		checks.ConstraintGuardOK = checkConstraintGuard(sample.Metadata)
	}
	checks.Passed = checks.TermPreservation &&
		checks.NeedRetrievalMatch &&
		checks.SubQuestionCountOK &&
		checks.MustContainAnyOK &&
		checks.MustNotStartWithOK &&
		checks.ConstraintGuardOK &&
		checks.CriticalTermsOK &&
		checks.AliasNormalizationOK &&
		checks.ForbiddenRewritesOK
	return checks
}

func hasConstraintGuardMetadata(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return false
	}
	_, ok := metadata["rewriteValidation"]
	return ok
}

func rewriteCandidateText(rewritten string, subQuestions []string) string {
	parts := make([]string, 0, 1+len(subQuestions))
	if rewritten = strings.TrimSpace(rewritten); rewritten != "" {
		parts = append(parts, rewritten)
	}
	for _, question := range subQuestions {
		if question = strings.TrimSpace(question); question != "" {
			parts = append(parts, question)
		}
	}
	return strings.Join(parts, " ")
}

func checkMustKeepTerms(combined string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	return len(missingTerms(combined, terms)) == 0
}

func missingTerms(combined string, terms []string) []string {
	lowerCombined := strings.ToLower(combined)
	missing := make([]string, 0)
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(combined, term) || strings.Contains(lowerCombined, strings.ToLower(term)) {
			continue
		}
		missing = append(missing, term)
	}
	return missing
}

func anyTermPresent(combined string, terms []string) bool {
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(combined, term) || strings.Contains(strings.ToLower(combined), strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func checkAnyGroups(combined string, groups [][]string) bool {
	if len(groups) == 0 {
		return true
	}
	for _, group := range groups {
		if !anyTermPresent(combined, group) {
			return false
		}
	}
	return true
}

func checkNeedRetrieval(sample RewriteSample) bool {
	if sample.Expect.NeedRetrieval == nil {
		return true
	}
	return sample.NeedRetrieval == *sample.Expect.NeedRetrieval
}

func checkSubQuestionCount(count int, expect RewriteExpect) bool {
	minCount := expect.SubQuestionCountMin
	maxCount := expect.SubQuestionCountMax
	if minCount <= 0 {
		minCount = 1
	}
	if maxCount <= 0 {
		maxCount = 4
	}
	return count >= minCount && count <= maxCount
}

func checkMustNotStartWith(rewritten string, subQuestions []string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	questions := append([]string{strings.TrimSpace(rewritten)}, subQuestions...)
	for _, question := range questions {
		question = strings.TrimSpace(question)
		if question == "" {
			continue
		}
		for _, prefix := range prefixes {
			prefix = strings.TrimSpace(prefix)
			if prefix == "" {
				continue
			}
			if strings.HasPrefix(question, prefix) || strings.HasPrefix(strings.ToLower(question), strings.ToLower(prefix)) {
				return false
			}
		}
	}
	return true
}

func checkForbiddenRewrites(combined string, values []string) bool {
	lowerCombined := strings.ToLower(combined)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.Contains(combined, value) || strings.Contains(lowerCombined, strings.ToLower(value)) {
			return false
		}
	}
	return true
}

func checkConstraintGuard(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return true
	}
	raw, ok := metadata["rewriteValidation"]
	if !ok || raw == nil {
		return true
	}
	switch typed := raw.(type) {
	case map[string]any:
		if accepted, ok := typed["accepted"].(bool); ok {
			return accepted
		}
	case ragrewrite.RewriteValidationReport:
		return typed.Accepted
	}
	return true
}

func aggregateRewriteResults(results []RewriteSampleResult) RewriteAggregateMetrics {
	metrics := RewriteAggregateMetrics{SampleCount: len(results)}
	if len(results) == 0 {
		return metrics
	}

	var passCount int
	var termPass, termDenom int
	var needPass, needDenom int
	var subPass, subDenom int
	var guardPass, guardDenom int

	for _, result := range results {
		if result.Checks.Passed {
			passCount++
		}
		if result.Checks.TermPreservationChecked {
			termDenom++
			if result.Checks.TermPreservation {
				termPass++
			}
		}
		if result.Checks.NeedRetrievalEvaluated {
			needDenom++
			if result.Checks.NeedRetrievalMatch {
				needPass++
			}
		}
		subDenom++
		if result.Checks.SubQuestionCountOK {
			subPass++
		}
		if result.Checks.ConstraintGuardChecked {
			guardDenom++
			if result.Checks.ConstraintGuardOK {
				guardPass++
			}
		}
	}

	metrics.PassRate = float64(passCount) / float64(len(results))
	metrics.TermPreservationRate = rate(termPass, termDenom)
	metrics.NeedRetrievalAccuracy = rate(needPass, needDenom)
	metrics.SubQuestionComplianceRate = rate(subPass, subDenom)
	metrics.ConstraintGuardPassRate = rate(guardPass, guardDenom)
	return metrics
}

func rate(pass, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(pass) / float64(denom)
}

func buildRewriteTagSummaries(results []RewriteSampleResult) []RewriteTagSummary {
	tagResults := map[string][]RewriteSampleResult{}
	for _, result := range results {
		for _, tag := range result.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			tagResults[tag] = append(tagResults[tag], result)
		}
	}
	tags := make([]string, 0, len(tagResults))
	for tag := range tagResults {
		tags = append(tags, tag)
	}
	sortStrings(tags)

	summaries := make([]RewriteTagSummary, 0, len(tags))
	for _, tag := range tags {
		summaries = append(summaries, RewriteTagSummary{
			Tag:     tag,
			Metrics: aggregateRewriteResults(tagResults[tag]),
		})
	}
	return summaries
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func ToChatHistory(messages []RewriteHistoryMessage) []convention.ChatMessage {
	if len(messages) == 0 {
		return nil
	}
	history := make([]convention.ChatMessage, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "user":
			history = append(history, convention.UserMessage(content))
		case "assistant":
			history = append(history, convention.AssistantMessage(content))
		case "system":
			history = append(history, convention.SystemMessage(content))
		}
	}
	return history
}
