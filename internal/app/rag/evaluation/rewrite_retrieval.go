package evaluation

import (
	"context"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

// DefaultRewriteEvalKnowledgeBaseID scopes rewrite eval retrieval to the
// retrieve-eval corpus when samples omit knowledge_base_ids.
const DefaultRewriteEvalKnowledgeBaseID = "23848386738319617"

type RewriteRetrievalDelta struct {
	HitAtK    map[int]float64 `json:"hit_at_k"`
	RecallAtK map[int]float64 `json:"recall_at_k"`
	NDCGAtK   map[int]float64 `json:"ndcg_at_k"`
	MRR       float64         `json:"mrr"`
}

type RewriteRetrievalComparison struct {
	ExpectedIDs           []string              `json:"expected_ids"`
	CriticalExpectedIDs   []string              `json:"critical_expected_ids,omitempty"`
	KnowledgeBaseIDs      []string              `json:"knowledge_base_ids,omitempty"`
	BaselineRetrievedIDs  []string              `json:"baseline_retrieved_ids,omitempty"`
	CandidateRetrievedIDs []string              `json:"candidate_retrieved_ids,omitempty"`
	Baseline              SampleResult          `json:"baseline"`
	Candidate             SampleResult          `json:"candidate"`
	Delta                 RewriteRetrievalDelta `json:"delta"`
	MustNotRegress        bool                  `json:"must_not_regress"`
	CriticalRegression    bool                  `json:"critical_regression"`
	CriticalIDsOK         bool                  `json:"critical_ids_ok"`
	Passed                bool                  `json:"passed"`
	FailureReasons        []string              `json:"failure_reasons,omitempty"`
	BaselinePipeline      map[string]any        `json:"baseline_pipeline,omitempty"`
	CandidatePipeline     map[string]any        `json:"candidate_pipeline,omitempty"`
}

func EvaluateRewriteRetrieval(
	ctx context.Context,
	sample RewriteSample,
	retrieve ragretrieve.Service,
	ks []int,
	subQuestionOptions ragretrieve.SubQuestionOptions,
	defaultKnowledgeBaseIDs []string,
) (*RewriteRetrievalComparison, error) {
	if retrieve == nil {
		return nil, nil
	}
	if len(sample.RetrievalExpectation.ExpectedIDs) == 0 {
		return nil, nil
	}

	normalizedKs, err := normalizeKs(ks)
	if err != nil {
		return nil, err
	}
	target, err := rewriteRetrievalTarget(sample.RetrievalExpectation.Target)
	if err != nil {
		return nil, err
	}
	knowledgeBaseIDs := resolveRewriteKnowledgeBaseIDs(sample, defaultKnowledgeBaseIDs)

	baselineSample := Sample{
		Name:             sample.Name + ":baseline",
		Query:            sample.Query,
		Tags:             append([]string(nil), sample.Tags...),
		Target:           target,
		ExpectedIDs:      append([]string(nil), sample.RetrievalExpectation.ExpectedIDs...),
		KnowledgeBaseIDs: append([]string(nil), knowledgeBaseIDs...),
		SearchMode:       sample.RetrievalExpectation.SearchMode,
		TopK:             sample.RetrievalExpectation.TopK,
	}
	if err := ExecuteSample(ctx, &baselineSample, ExecuteConfig{
		Retrieve: retrieve,
	}); err != nil {
		return nil, err
	}

	candidateSample, err := executeRewriteRetrievalCandidate(ctx, sample, retrieve, target, knowledgeBaseIDs, subQuestionOptions)
	if err != nil {
		return nil, err
	}

	baselineSummary, err := Evaluate([]Sample{baselineSample}, normalizedKs)
	if err != nil {
		return nil, err
	}
	candidateSummary, err := Evaluate([]Sample{candidateSample}, normalizedKs)
	if err != nil {
		return nil, err
	}

	comparison := &RewriteRetrievalComparison{
		ExpectedIDs:           append([]string(nil), sample.RetrievalExpectation.ExpectedIDs...),
		CriticalExpectedIDs:   append([]string(nil), sample.RetrievalExpectation.CriticalExpectedIDs...),
		KnowledgeBaseIDs:      append([]string(nil), knowledgeBaseIDs...),
		BaselineRetrievedIDs:  retrievedChunkIDs(baselineSample.Retrieved),
		CandidateRetrievedIDs: retrievedChunkIDs(candidateSample.Retrieved),
		Baseline:              baselineSummary.Samples[0],
		Candidate:             candidateSummary.Samples[0],
		BaselinePipeline:      baselineSample.PipelineTrace,
		CandidatePipeline:     candidateSample.PipelineTrace,
		Delta: buildRewriteRetrievalDelta(
			baselineSummary.Samples[0],
			candidateSummary.Samples[0],
			normalizedKs,
		),
		MustNotRegress: sample.RetrievalExpectation.MustNotRegress,
		CriticalIDsOK:  rewriteCriticalExpectedIDsOK(candidateSample, sample.RetrievalExpectation.CriticalExpectedIDs, target),
	}
	comparison.CriticalRegression = rewriteMustNotRegressTriggered(comparison.Baseline, comparison.Candidate, normalizedKs) && comparison.MustNotRegress
	if !comparison.CriticalIDsOK {
		comparison.FailureReasons = append(comparison.FailureReasons, "critical expected ids missing after rewrite retrieval")
	}
	if comparison.CriticalRegression {
		comparison.FailureReasons = append(comparison.FailureReasons, "rewrite retrieval regressed on must_not_regress sample")
	}
	comparison.Passed = comparison.CriticalIDsOK && !comparison.CriticalRegression
	return comparison, nil
}

func executeRewriteRetrievalCandidate(
	ctx context.Context,
	sample RewriteSample,
	retrieve ragretrieve.Service,
	target Target,
	knowledgeBaseIDs []string,
	subQuestionOptions ragretrieve.SubQuestionOptions,
) (Sample, error) {
	topK := sample.RetrievalExpectation.TopK
	if topK <= 0 {
		topK = ragretrieve.DefaultTopK
	}
	prerankTopK := topK
	rerankTopN := 0
	if prerankTopK < evalMinPreRerankCandidates {
		prerankTopK = evalMinPreRerankCandidates
		rerankTopN = topK
	}
	request := ragretrieve.Request{
		Query:            strings.TrimSpace(sample.Query),
		KnowledgeBaseIDs: append([]string(nil), knowledgeBaseIDs...),
		SearchMode:       strings.TrimSpace(sample.RetrievalExpectation.SearchMode),
		TopK:             prerankTopK,
		RerankTopN:       rerankTopN,
	}

	subQuestions := ragretrieve.BuildRetrieveSubQuestions(sample.Query, sample.SubQuestions)
	executor := ragretrieve.NewSubQuestionExecutor(retrieve, subQuestionOptions)
	result, executionMode, _, err := executor.RetrieveMerged(ctx, request, subQuestions, topK)
	if err != nil {
		return Sample{}, err
	}
	return Sample{
		Name:             sample.Name + ":candidate",
		Query:            sample.Query,
		Tags:             append([]string(nil), sample.Tags...),
		Target:           target,
		ExpectedIDs:      append([]string(nil), sample.RetrievalExpectation.ExpectedIDs...),
		Retrieved:        retrievedItemsFromChunks(result.Chunks),
		ChannelRetrieved: channelRetrievedFromResult(result),
		PipelineTrace:    pipelineTraceToMap(result.PipelineTrace),
		SearchMode:       sample.RetrievalExpectation.SearchMode,
		TopK:             topK,
		RewrittenQuery:   sample.RewrittenQuery,
		SubQuestions:     append([]string(nil), sample.SubQuestions...),
		NeedRetrieval:    sample.NeedRetrieval,
		ExecutionMode:    executionMode,
	}, nil
}

func rewriteRetrievalTarget(raw string) (Target, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "document", "document_id", "knowledge_base":
		return TargetDocument, nil
	case "chunk":
		return TargetChunk, nil
	case "document_name":
		return TargetDocumentName, nil
	case "source_file_name":
		return TargetSourceFileName, nil
	case "section":
		return TargetSection, nil
	default:
		return "", fmt.Errorf("unsupported rewrite retrieval target %q", raw)
	}
}

func buildRewriteRetrievalDelta(baseline SampleResult, candidate SampleResult, ks []int) RewriteRetrievalDelta {
	delta := RewriteRetrievalDelta{
		HitAtK:    make(map[int]float64, len(ks)),
		RecallAtK: make(map[int]float64, len(ks)),
		NDCGAtK:   make(map[int]float64, len(ks)),
		MRR:       candidate.ReciprocalRank - baseline.ReciprocalRank,
	}
	for _, k := range ks {
		delta.HitAtK[k] = boolToFloat(candidate.HitAtK[k]) - boolToFloat(baseline.HitAtK[k])
		delta.RecallAtK[k] = candidate.RecallAtK[k] - baseline.RecallAtK[k]
		delta.NDCGAtK[k] = candidate.NDCGAtK[k] - baseline.NDCGAtK[k]
	}
	return delta
}

func rewriteMustNotRegressTriggered(baseline SampleResult, candidate SampleResult, ks []int) bool {
	if candidate.ReciprocalRank < baseline.ReciprocalRank {
		return true
	}
	for _, k := range ks {
		if candidate.RecallAtK[k] < baseline.RecallAtK[k] {
			return true
		}
		if boolToFloat(candidate.HitAtK[k]) < boolToFloat(baseline.HitAtK[k]) {
			return true
		}
	}
	return false
}

func rewriteCriticalExpectedIDsOK(sample Sample, criticalExpectedIDs []string, target Target) bool {
	if len(criticalExpectedIDs) == 0 {
		return true
	}
	seen := map[string]struct{}{}
	for _, item := range sample.Retrieved {
		id := strings.TrimSpace(extractTargetID(item, target))
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, expected := range criticalExpectedIDs {
		expected = strings.TrimSpace(expected)
		if expected == "" {
			continue
		}
		if _, ok := seen[expected]; !ok {
			return false
		}
	}
	return true
}

func buildRewriteRetrievalImpactScore(comparison *RewriteRetrievalComparison, ks []int) float64 {
	if comparison == nil {
		return 0
	}
	if len(ks) == 0 {
		return 0
	}

	hitSum := 0.0
	recallSum := 0.0
	ndcgSum := 0.0
	for _, k := range ks {
		hitSum += boolToFloat(comparison.Candidate.HitAtK[k])
		recallSum += comparison.Candidate.RecallAtK[k]
		ndcgSum += comparison.Candidate.NDCGAtK[k]
	}
	divisor := float64(len(ks))
	return roundSummaryScore(
		(comparison.Candidate.ReciprocalRank +
			(hitSum/divisor) +
			(recallSum/divisor) +
			(ndcgSum/divisor)) / 4,
	)
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func aggregateMetricDelta(candidate, baseline map[int]float64, ks []int) map[int]float64 {
	if len(ks) == 0 {
		return nil
	}
	delta := make(map[int]float64, len(ks))
	for _, k := range ks {
		delta[k] = candidate[k] - baseline[k]
	}
	return delta
}

func resolveRewriteKnowledgeBaseIDs(sample RewriteSample, defaults []string) []string {
	if len(sample.RetrievalExpectation.KnowledgeBaseIDs) > 0 {
		return append([]string(nil), sample.RetrievalExpectation.KnowledgeBaseIDs...)
	}
	if len(defaults) > 0 {
		return append([]string(nil), defaults...)
	}
	return nil
}

func retrievedChunkIDs(items []RetrievedItem) []string {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ChunkID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}
