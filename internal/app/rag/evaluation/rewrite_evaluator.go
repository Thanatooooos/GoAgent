package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
)

type RewriteEvaluator struct {
	rewrite                 ragrewrite.Service
	retrieve                ragretrieve.Service
	embedding               QueryEmbedder
	judge                   Judge
	retrievalKs             []int
	subQuestionOptions      ragretrieve.SubQuestionOptions
	defaultKnowledgeBaseIDs []string
}

type RewriteEvaluatorOption func(*RewriteEvaluator)

func WithRewriteRetrieveService(retrieve ragretrieve.Service) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.retrieve = retrieve
	}
}

func WithRewriteRetrievalKs(ks []int) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.retrievalKs = append([]int(nil), ks...)
	}
}

func WithRewriteSubQuestionOptions(opts ragretrieve.SubQuestionOptions) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.subQuestionOptions = opts
	}
}

func WithRewriteDefaultKnowledgeBaseIDs(ids []string) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.defaultKnowledgeBaseIDs = append([]string(nil), ids...)
	}
}

func WithRewriteQueryEmbedder(embedder QueryEmbedder) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.embedding = embedder
	}
}

func WithRewriteJudge(judge Judge) RewriteEvaluatorOption {
	return func(e *RewriteEvaluator) {
		e.judge = judge
	}
}

func NewRewriteEvaluator(rewrite ragrewrite.Service, opts ...RewriteEvaluatorOption) *RewriteEvaluator {
	evaluator := &RewriteEvaluator{
		rewrite:     rewrite,
		retrievalKs: []int{1, 3, 5},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(evaluator)
		}
	}
	return evaluator
}

func (e *RewriteEvaluator) Suite() SuiteName {
	return SuiteRewrite
}

func (e *RewriteEvaluator) LoadSamples(_ context.Context, path string) (json.RawMessage, error) {
	raw, err := LoadRawSampleFile(path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (e *RewriteEvaluator) Run(ctx context.Context, input RunInput) (SuiteResult, error) {
	if e == nil || e.rewrite == nil {
		return SuiteResult{}, fmt.Errorf("rewrite service is required")
	}
	samples, err := ParseRewriteSamples(input.RawSamples)
	if err != nil {
		return SuiteResult{}, err
	}

	results := make([]SharedSampleResult, 0, len(samples))
	tagStats := map[string]*tagSummaryAccumulator{}
	criticalFailureCount := 0
	artifacts := map[string]any{}
	baselineRetrievalResults := make([]SampleResult, 0, len(samples))
	candidateRetrievalResults := make([]SampleResult, 0, len(samples))
	retrievalRegressionCount := 0
	var semanticScoreTotal float64
	var semanticScoreCount int
	var judgeScoreTotal float64
	var judgeScoreCount int
	var softGateOverrideCount int

	for _, sample := range samples {
		if err := ExecuteRewriteSample(ctx, &sample, e.rewrite); err != nil {
			return SuiteResult{}, fmt.Errorf("execute rewrite sample %q: %w", sample.Name, err)
		}
		checks := checkRewriteExpect(sample)
		criticalFailures, failureReasons := collectRewriteFailures(checks)
		rewriteQuality := buildRewriteQualityScore(checks)
		retrievalImpact := 0.0
		scores := map[string]any{
			"rewrite_quality": rewriteQuality,
		}
		judgeChecks := map[string]any{}
		var semanticEval *RewriteSemanticEvaluation
		if e.embedding != nil {
			evaluation, semanticErr := EvaluateRewriteSemantic(ctx, e.embedding, sample)
			if semanticErr != nil {
				judgeChecks["semantic"] = map[string]any{"error": semanticErr.Error()}
			} else {
				semanticEval = &evaluation
				judgeChecks["semantic"] = evaluation
				scores["semantic_score"] = evaluation.SemanticScore
				scores["rewrite_similarity"] = evaluation.RewriteSimilarity
				semanticScoreTotal += evaluation.SemanticScore
				semanticScoreCount++
			}
		}
		var judgeEval *RewriteJudgeEvaluation
		if e.judge != nil {
			evaluation, judgeErr := RunRewriteJudge(ctx, e.judge, sample)
			if judgeErr != nil {
				judgeChecks["quality"] = map[string]any{"error": judgeErr.Error()}
			} else {
				judgeEval = &evaluation
				judgeChecks["quality"] = evaluation
				scores["judge_score"] = evaluation.Score
				judgeScoreTotal += evaluation.Score
				judgeScoreCount++
			}
		}
		if semanticEval != nil || judgeEval != nil {
			scores["rewrite_semantic_quality"] = buildRewriteSemanticQualityScore(semanticEval, judgeEval)
		}
		retrievalComparison, err := EvaluateRewriteRetrieval(ctx, sample, e.retrieve, e.retrievalKs, e.subQuestionOptions, e.defaultKnowledgeBaseIDs)
		if err != nil {
			return SuiteResult{}, fmt.Errorf("evaluate rewrite retrieval for sample %q: %w", sample.Name, err)
		}
		if retrievalComparison != nil {
			retrievalImpact = buildRewriteRetrievalImpactScore(retrievalComparison, e.retrievalKs)
			scores["retrieval_impact"] = retrievalImpact
			baselineRetrievalResults = append(baselineRetrievalResults, retrievalComparison.Baseline)
			candidateRetrievalResults = append(candidateRetrievalResults, retrievalComparison.Candidate)
			if retrievalComparison.CriticalRegression {
				retrievalRegressionCount++
				criticalFailures = append(criticalFailures, "retrieval_must_not_regress")
			}
			if !retrievalComparison.CriticalIDsOK {
				criticalFailures = append(criticalFailures, "critical_expected_ids_missing")
			}
			failureReasons = append(failureReasons, retrievalComparison.FailureReasons...)
		} else {
			scores["retrieval_impact_pending"] = true
		}

		softGate := evaluateRewriteSoftGate(checks, criticalFailures, failureReasons, semanticEval, judgeEval)
		if len(judgeChecks) > 0 {
			judgeChecks["soft_gate"] = softGate
		}
		scores["semantic_soft_gate_ok"] = softGate.Passed
		if softGate.PassPath != "" {
			scores["pass_path"] = softGate.PassPath
		}
		if softGate.PassPath == "semantic_judge" {
			softGateOverrideCount++
		}

		passed := softGate.Passed
		if retrievalComparison != nil {
			passed = passed && retrievalComparison.Passed
			scores["diagnostic_score"] = buildRewriteDiagnosticScore(rewriteQuality, softGate, retrievalImpact, true)
		} else {
			scores["diagnostic_score"] = buildRewriteDiagnosticScore(rewriteQuality, softGate, 0, false)
		}
		scores["gate_blocked"] = !passed

		sampleResult := SharedSampleResult{
			Name:             sample.Name,
			Tags:             sample.Tags,
			Passed:           passed,
			CriticalFailures: criticalFailures,
			RuleChecks: map[string]any{
				"term_preservation_ok":     checks.TermPreservation,
				"need_retrieval_ok":        checks.NeedRetrievalMatch,
				"sub_question_count_ok":    checks.SubQuestionCountOK,
				"must_contain_any_ok":      checks.MustContainAnyOK,
				"must_not_start_with_ok":   checks.MustNotStartWithOK,
				"constraint_guard_ok":      checks.ConstraintGuardOK,
				"critical_terms_ok":        checks.CriticalTermsOK,
				"alias_normalization_ok":   checks.AliasNormalizationOK,
				"forbidden_rewrites_ok":    checks.ForbiddenRewritesOK,
				"rule_passed":              checks.Passed,
				"semantic_soft_gate_ok":    softGate.Passed,
			},
			Scores:         scores,
			FailureReasons: failureReasons,
		}
		if len(judgeChecks) > 0 {
			sampleResult.JudgeChecks = judgeChecks
		}
		results = append(results, sampleResult)
		artifact := map[string]any{
			"query":            sample.Query,
			"rewritten_query":  sample.RewrittenQuery,
			"sub_questions":    append([]string(nil), sample.SubQuestions...),
			"need_retrieval":   sample.NeedRetrieval,
			"retrieval_expectation": sample.RetrievalExpectation,
		}
		if retrievalComparison != nil {
			artifact["retrieval_comparison"] = retrievalComparison
		}
		if semanticEval != nil {
			artifact["semantic_evaluation"] = semanticEval
		} else if semanticErrArtifact, ok := judgeChecks["semantic"]; ok && semanticEval == nil {
			artifact["semantic_evaluation"] = semanticErrArtifact
		}
		if judgeEval != nil {
			artifact["judge_evaluation"] = judgeEval
		}
		artifacts[sample.Name] = artifact

		if len(criticalFailures) > 0 {
			criticalFailureCount++
		}
		for _, tag := range sample.Tags {
			acc := tagStats[tag]
			if acc == nil {
				acc = &tagSummaryAccumulator{}
				tagStats[tag] = acc
			}
			acc.total++
			if passed {
				acc.passed++
			}
			if len(criticalFailures) > 0 {
				acc.criticalFailures++
			}
		}
	}

	aggregateMetrics := map[string]any{
		"sample_count": len(results),
	}
	if len(baselineRetrievalResults) > 0 {
		baselineAggregate := aggregate(baselineRetrievalResults, e.retrievalKs)
		candidateAggregate := aggregate(candidateRetrievalResults, e.retrievalKs)
		aggregateMetrics["baseline_mrr"] = baselineAggregate.MRR
		aggregateMetrics["candidate_mrr"] = candidateAggregate.MRR
		aggregateMetrics["baseline_hit_at_k"] = baselineAggregate.HitRateAtK
		aggregateMetrics["candidate_hit_at_k"] = candidateAggregate.HitRateAtK
		aggregateMetrics["baseline_recall_at_k"] = baselineAggregate.AverageRecallAtK
		aggregateMetrics["candidate_recall_at_k"] = candidateAggregate.AverageRecallAtK
		aggregateMetrics["baseline_ndcg_at_k"] = baselineAggregate.AverageNDCGAtK
		aggregateMetrics["candidate_ndcg_at_k"] = candidateAggregate.AverageNDCGAtK
		aggregateMetrics["retrieval_regression_count"] = retrievalRegressionCount
		aggregateMetrics["mrr_uplift"] = candidateAggregate.MRR - baselineAggregate.MRR
		aggregateMetrics["hit_at_k_uplift"] = aggregateMetricDelta(candidateAggregate.HitRateAtK, baselineAggregate.HitRateAtK, e.retrievalKs)
		aggregateMetrics["recall_at_k_uplift"] = aggregateMetricDelta(candidateAggregate.AverageRecallAtK, baselineAggregate.AverageRecallAtK, e.retrievalKs)
		aggregateMetrics["ndcg_at_k_uplift"] = aggregateMetricDelta(candidateAggregate.AverageNDCGAtK, baselineAggregate.AverageNDCGAtK, e.retrievalKs)
	}
	if semanticScoreCount > 0 {
		aggregateMetrics["avg_semantic_score"] = roundSummaryScore(semanticScoreTotal / float64(semanticScoreCount))
	}
	if judgeScoreCount > 0 {
		aggregateMetrics["avg_judge_score"] = roundSummaryScore(judgeScoreTotal / float64(judgeScoreCount))
	}
	if softGateOverrideCount > 0 {
		aggregateMetrics["semantic_judge_override_count"] = softGateOverrideCount
	}

	return SuiteResult{
		Suite: string(SuiteRewrite),
		RunMetadata: RunMetadata{
			RunAt:       time.Now().UTC().Format(time.RFC3339),
			Suite:       string(SuiteRewrite),
			SampleSetID: input.InputPath,
		},
		Samples: results,
		Aggregate: SharedAggregateResult{
			PassRate:            rate(countPassed(results), len(results)),
			CriticalFailureRate: rate(criticalFailureCount, len(results)),
			ByTag:               buildTagAggregates(tagStats),
			Metrics:             aggregateMetrics,
		},
		Artifacts: map[string]any{
			"executions": artifacts,
		},
	}, nil
}

func collectRewriteFailures(checks RewriteCheckResult) ([]string, []string) {
	criticalFailures := []string{}
	failureReasons := []string{}

	if !checks.TermPreservation {
		failureReasons = append(failureReasons, "required terms missing")
	}
	if !checks.NeedRetrievalMatch {
		criticalFailures = append(criticalFailures, "need_retrieval_mismatch")
		failureReasons = append(failureReasons, "need_retrieval mismatch")
	}
	if !checks.SubQuestionCountOK {
		failureReasons = append(failureReasons, "sub question count out of range")
	}
	if !checks.MustContainAnyOK {
		failureReasons = append(failureReasons, "required rewrite group missing")
	}
	if !checks.MustNotStartWithOK {
		failureReasons = append(failureReasons, "rewrite starts with forbidden prefix")
	}
	if !checks.ConstraintGuardOK {
		failureReasons = append(failureReasons, "constraint guard rejected rewrite")
	}
	if !checks.CriticalTermsOK {
		criticalFailures = append(criticalFailures, "critical_terms_missing")
		failureReasons = append(failureReasons, "critical terms missing")
	}
	if !checks.AliasNormalizationOK {
		failureReasons = append(failureReasons, "alias normalization expectation missing")
	}
	if !checks.ForbiddenRewritesOK {
		criticalFailures = append(criticalFailures, "forbidden_rewrite_present")
		failureReasons = append(failureReasons, "forbidden rewrite present")
	}

	return criticalFailures, failureReasons
}

func buildRewriteQualityScore(checks RewriteCheckResult) float64 {
	total := 0.0
	count := 0.0
	values := []bool{
		checks.TermPreservation,
		checks.NeedRetrievalMatch,
		checks.SubQuestionCountOK,
		checks.MustContainAnyOK,
		checks.MustNotStartWithOK,
		checks.ConstraintGuardOK,
		checks.CriticalTermsOK,
		checks.AliasNormalizationOK,
		checks.ForbiddenRewritesOK,
	}
	for _, ok := range values {
		count++
		if ok {
			total++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}
