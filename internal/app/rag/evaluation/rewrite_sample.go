package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"
)

type rewriteSampleWire struct {
	Name                string                      `json:"name"`
	Tags                []string                    `json:"tags,omitempty"`
	Input               *rewriteSampleInputWire     `json:"input,omitempty"`
	RewriteExpectation  *rewriteExpectationWire     `json:"rewrite_expectation,omitempty"`
	RetrievalExpectation *rewriteRetrievalExpectationWire `json:"retrieval_expectation,omitempty"`
	Metadata            map[string]any              `json:"metadata,omitempty"`

	Query          string                  `json:"query,omitempty"`
	History        []RewriteHistoryMessage `json:"history,omitempty"`
	Expect         *RewriteExpect          `json:"expect,omitempty"`
	RewrittenQuery string                  `json:"rewrittenQuery,omitempty"`
	SubQuestions   []string                `json:"subQuestions,omitempty"`
	NeedRetrieval  *bool                   `json:"needRetrieval,omitempty"`
}

type rewriteSampleInputWire struct {
	Query   string                  `json:"query"`
	History []RewriteHistoryMessage `json:"history,omitempty"`
}

type rewriteExpectationWire struct {
	NeedRetrieval     *bool                 `json:"need_retrieval,omitempty"`
	MustKeepTerms     []string              `json:"must_keep_terms,omitempty"`
	MustKeepAnyGroups [][]string            `json:"must_keep_any_groups,omitempty"`
	MustContainAny    [][]string            `json:"must_contain_any,omitempty"`
	MustNotStartWith  []string              `json:"must_not_start_with,omitempty"`
	CriticalTerms     []string              `json:"critical_terms,omitempty"`
	AliasGroups       [][]string            `json:"alias_groups,omitempty"`
	ForbiddenRewrites []string              `json:"forbidden_rewrites,omitempty"`
	SubQuestionCount  *rewriteCountRangeWire `json:"sub_question_count,omitempty"`
}

type rewriteCountRangeWire struct {
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
}

type rewriteRetrievalExpectationWire struct {
	Target              string   `json:"target,omitempty"`
	ExpectedIDs         []string `json:"expected_ids,omitempty"`
	CriticalExpectedIDs []string `json:"critical_expected_ids,omitempty"`
	KnowledgeBaseIDs    []string `json:"knowledge_base_ids,omitempty"`
	TopK                int      `json:"top_k,omitempty"`
	SearchMode          string   `json:"search_mode,omitempty"`
	MustNotRegress      bool     `json:"must_not_regress,omitempty"`
}

func ParseRewriteSamples(rawSamples []json.RawMessage) ([]RewriteSample, error) {
	samples := make([]RewriteSample, 0, len(rawSamples))
	for i, raw := range rawSamples {
		var wire rewriteSampleWire
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, fmt.Errorf("decode rewrite sample %d: %w", i, err)
		}
		sample := materializeRewriteSample(wire)
		if err := validateRewriteSample(sample); err != nil {
			return nil, fmt.Errorf("validate rewrite sample %d: %w", i, err)
		}
		normalizeRewriteSample(&sample)
		samples = append(samples, sample)
	}
	return samples, nil
}

func materializeRewriteSample(wire rewriteSampleWire) RewriteSample {
	sample := RewriteSample{
		Name:                wire.Name,
		Tags:                append([]string(nil), wire.Tags...),
		Query:               wire.Query,
		History:             append([]RewriteHistoryMessage(nil), wire.History...),
		Metadata:            cloneAnyMap(wire.Metadata),
		RewrittenQuery:      wire.RewrittenQuery,
		SubQuestions:        append([]string(nil), wire.SubQuestions...),
	}
	if wire.NeedRetrieval != nil {
		sample.NeedRetrieval = *wire.NeedRetrieval
	}
	if wire.Expect != nil {
		sample.Expect = *wire.Expect
	}
	if wire.Input != nil {
		sample.Query = wire.Input.Query
		sample.History = append([]RewriteHistoryMessage(nil), wire.Input.History...)
	}
	if wire.RewriteExpectation != nil {
		sample.Expect = RewriteExpect{
			NeedRetrieval:       wire.RewriteExpectation.NeedRetrieval,
			MustKeepTerms:       append([]string(nil), wire.RewriteExpectation.MustKeepTerms...),
			MustKeepAnyGroups:   cloneStringGroups(wire.RewriteExpectation.MustKeepAnyGroups),
			MustContainAny:      cloneStringGroups(wire.RewriteExpectation.MustContainAny),
			MustNotStartWith:    append([]string(nil), wire.RewriteExpectation.MustNotStartWith...),
			CriticalTerms:       append([]string(nil), wire.RewriteExpectation.CriticalTerms...),
			AliasGroups:         cloneStringGroups(wire.RewriteExpectation.AliasGroups),
			ForbiddenRewrites:   append([]string(nil), wire.RewriteExpectation.ForbiddenRewrites...),
		}
		if wire.RewriteExpectation.SubQuestionCount != nil {
			sample.Expect.SubQuestionCountMin = wire.RewriteExpectation.SubQuestionCount.Min
			sample.Expect.SubQuestionCountMax = wire.RewriteExpectation.SubQuestionCount.Max
		}
	}
	if wire.RetrievalExpectation != nil {
		sample.RetrievalExpectation = RewriteRetrievalExpectation{
			Target:              wire.RetrievalExpectation.Target,
			ExpectedIDs:         append([]string(nil), wire.RetrievalExpectation.ExpectedIDs...),
			CriticalExpectedIDs: append([]string(nil), wire.RetrievalExpectation.CriticalExpectedIDs...),
			KnowledgeBaseIDs:    append([]string(nil), wire.RetrievalExpectation.KnowledgeBaseIDs...),
			TopK:                wire.RetrievalExpectation.TopK,
			SearchMode:          wire.RetrievalExpectation.SearchMode,
			MustNotRegress:      wire.RetrievalExpectation.MustNotRegress,
		}
	}
	return sample
}

func validateRewriteSample(sample RewriteSample) error {
	if strings.TrimSpace(sample.Name) == "" {
		return fmt.Errorf("sample name is required")
	}
	if strings.TrimSpace(sample.Query) == "" {
		return fmt.Errorf("sample %q query is required", sample.Name)
	}
	return nil
}

func normalizeRewriteSample(sample *RewriteSample) {
	if sample == nil {
		return
	}
	sample.Name = strings.TrimSpace(sample.Name)
	sample.Query = strings.TrimSpace(sample.Query)
	sample.Tags = normalizeTags(sample.Tags)
	for i := range sample.History {
		sample.History[i].Role = strings.TrimSpace(sample.History[i].Role)
		sample.History[i].Content = strings.TrimSpace(sample.History[i].Content)
	}
	sample.Expect.MustKeepTerms = normalizeSummaryValues(sample.Expect.MustKeepTerms)
	sample.Expect.MustKeepAnyGroups = normalizeStringGroups(sample.Expect.MustKeepAnyGroups)
	sample.Expect.MustContainAny = normalizeStringGroups(sample.Expect.MustContainAny)
	sample.Expect.MustNotStartWith = normalizeSummaryValues(sample.Expect.MustNotStartWith)
	sample.Expect.CriticalTerms = normalizeSummaryValues(sample.Expect.CriticalTerms)
	sample.Expect.AliasGroups = normalizeStringGroups(sample.Expect.AliasGroups)
	sample.Expect.ForbiddenRewrites = normalizeSummaryValues(sample.Expect.ForbiddenRewrites)
	sample.RetrievalExpectation.Target = strings.TrimSpace(sample.RetrievalExpectation.Target)
	sample.RetrievalExpectation.ExpectedIDs = normalizeSummaryValues(sample.RetrievalExpectation.ExpectedIDs)
	sample.RetrievalExpectation.CriticalExpectedIDs = normalizeSummaryValues(sample.RetrievalExpectation.CriticalExpectedIDs)
	sample.RetrievalExpectation.KnowledgeBaseIDs = normalizeSummaryValues(sample.RetrievalExpectation.KnowledgeBaseIDs)
	sample.RetrievalExpectation.SearchMode = strings.TrimSpace(sample.RetrievalExpectation.SearchMode)
	sample.RewrittenQuery = strings.TrimSpace(sample.RewrittenQuery)
	sample.SubQuestions = normalizeSummaryValues(sample.SubQuestions)
}

func cloneStringGroups(groups [][]string) [][]string {
	if len(groups) == 0 {
		return nil
	}
	cloned := make([][]string, 0, len(groups))
	for _, group := range groups {
		cloned = append(cloned, append([]string(nil), group...))
	}
	return cloned
}

func normalizeStringGroups(groups [][]string) [][]string {
	if len(groups) == 0 {
		return nil
	}
	normalized := make([][]string, 0, len(groups))
	for _, group := range groups {
		values := normalizeSummaryValues(group)
		if len(values) > 0 {
			normalized = append(normalized, values)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
