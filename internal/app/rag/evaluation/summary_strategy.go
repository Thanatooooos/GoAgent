package evaluation

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
)

type summaryStrategyDependencies struct {
	Generator             SummaryGenerator
	Judge                 Judge
	AnswerGenerator       SummaryAnswerGenerator
	ThresholdUnit         SummaryStrategyThresholdUnit
	Thresholds            []int
	Estimator             tokenbudget.Estimator
	MessageOverheadTokens int
}

type SummaryStrategyCheckpointResult struct {
	AfterTurn             int                           `json:"after_turn"`
	RuleEvaluation        SummaryRuleEvaluation         `json:"rule_evaluation"`
	FieldJudge            *SummaryFieldJudgeEvaluation  `json:"field_judge,omitempty"`
	DownstreamEquivalence *SummaryEquivalenceEvaluation `json:"downstream_equivalence,omitempty"`
	TokenBaseline         int                           `json:"token_baseline"`
	TokenStrategy         int                           `json:"token_strategy"`
	TokenSaved            int                           `json:"token_saved"`
	TokenSavedRatio       float64                       `json:"token_saved_ratio"`
}

type SummaryStrategyThresholdResult struct {
	Threshold                  int                               `json:"threshold"`
	ThresholdUnit              SummaryStrategyThresholdUnit      `json:"threshold_unit"`
	SummaryCallCount           int                               `json:"summary_call_count"`
	CheckpointResults          []SummaryStrategyCheckpointResult `json:"checkpoint_results,omitempty"`
	FinalResult                *SummaryStrategyCheckpointResult  `json:"final_result,omitempty"`
	TokenBaseline              int                               `json:"token_baseline"`
	TokenStrategy              int                               `json:"token_strategy"`
	TokenSaved                 int                               `json:"token_saved"`
	TokenSavedRatio            float64                           `json:"token_saved_ratio"`
	SummaryFidelityScore       float64                           `json:"summary_fidelity_score"`
	SummaryUsefulnessScore     float64                           `json:"summary_usefulness_score"`
	DownstreamEquivalenceScore float64                           `json:"downstream_equivalence_score"`
	CriticalFailureCount       int                               `json:"critical_failure_count"`
	DangerousDriftCount        int                               `json:"dangerous_drift_count"`
	Passed                     bool                              `json:"passed"`
}

type SummaryStrategySampleResult struct {
	ThresholdResults []SummaryStrategyThresholdResult `json:"threshold_results"`
	ParetoCandidates []int                            `json:"pareto_candidates,omitempty"`
}

type SummaryStrategyThresholdAggregate struct {
	Threshold                int                          `json:"threshold"`
	ThresholdUnit            SummaryStrategyThresholdUnit `json:"threshold_unit"`
	SampleCount              int                          `json:"sample_count"`
	AvgTokenSavedRatio       float64                      `json:"avg_token_saved_ratio"`
	AvgStructuredFidelity    float64                      `json:"avg_structured_fidelity"`
	AvgStructuredUsefulness  float64                      `json:"avg_structured_usefulness"`
	AvgDownstreamEquivalence float64                      `json:"avg_downstream_equivalence"`
	AvgSummaryCallCount      float64                      `json:"avg_summary_call_count"`
	PassRate                 float64                      `json:"pass_rate"`
	CriticalFailureRate      float64                      `json:"critical_failure_rate"`
	DangerousDriftRate       float64                      `json:"dangerous_drift_rate"`
}

type summaryStrategyCheckpointOutcome struct {
	result               SummaryStrategyCheckpointResult
	fidelityScore        float64
	usefulnessScore      float64
	downstreamScore      float64
	criticalFailureCount int
	dangerousDriftCount  int
	passed               bool
}

type summaryStrategyThresholdAggregateAccumulator struct {
	sampleCount            int
	passedCount            int
	criticalFailureSamples int
	dangerousDriftSamples  int
	tokenSavedRatios       []float64
	fidelityScores         []float64
	usefulnessScores       []float64
	downstreamScores       []float64
	summaryCallCounts      []float64
}

type summaryStrategyAggregateKey struct {
	unit      SummaryStrategyThresholdUnit
	threshold int
}

func runSummaryStrategySweep(ctx context.Context, deps summaryStrategyDependencies, sample SummarySample) (SummaryStrategySampleResult, error) {
	if deps.Generator == nil {
		return SummaryStrategySampleResult{}, fmt.Errorf("summary generator is required")
	}
	if sample.StrategyEval == nil || len(sample.StrategyEval.Checkpoints) == 0 {
		return SummaryStrategySampleResult{}, fmt.Errorf("strategy_eval checkpoints are required")
	}
	thresholds := normalizeStrategyThresholds(deps.Thresholds)
	if len(thresholds) == 0 {
		return SummaryStrategySampleResult{}, fmt.Errorf("at least one strategy threshold is required")
	}
	if deps.ThresholdUnit == "" {
		deps.ThresholdUnit = SummaryStrategyThresholdTurns
	}
	results := make([]SummaryStrategyThresholdResult, 0, len(thresholds))
	for _, threshold := range thresholds {
		result, err := runSummaryStrategyThreshold(ctx, deps, sample, threshold)
		if err != nil {
			return SummaryStrategySampleResult{}, err
		}
		results = append(results, result)
	}
	return SummaryStrategySampleResult{ThresholdResults: results, ParetoCandidates: buildStrategyParetoCandidates(results)}, nil
}

func runSummaryStrategyThreshold(ctx context.Context, deps summaryStrategyDependencies, sample SummarySample, threshold int) (SummaryStrategyThresholdResult, error) {
	checkpoints := append([]SummaryStrategyCheckpoint(nil), sample.StrategyEval.Checkpoints...)
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].AfterTurn < checkpoints[j].AfterTurn })

	compressedUntilTurn := 0
	observedUntilTurn := 0
	nextTriggerTurn := threshold
	summaryCalls := 0
	turnCount := countSummaryTurns(sample.Input.SourceMessages)
	var committed SummaryGenerationOutput
	haveCommitted := false

	checkpointResults := make([]SummaryStrategyCheckpointResult, 0, len(checkpoints))
	var finalResult *SummaryStrategyCheckpointResult
	totalBaseline := 0
	totalStrategy := 0
	criticalFailureCount := 0
	dangerousDriftCount := 0
	fidelityScores := make([]float64, 0, len(checkpoints)+1)
	usefulnessScores := make([]float64, 0, len(checkpoints)+1)
	downstreamScores := make([]float64, 0, len(checkpoints)+1)
	passed := true

	applyOutcome := func(outcome summaryStrategyCheckpointOutcome) {
		checkpointResults = append(checkpointResults, outcome.result)
		totalBaseline += outcome.result.TokenBaseline
		totalStrategy += outcome.result.TokenStrategy
		criticalFailureCount += outcome.criticalFailureCount
		dangerousDriftCount += outcome.dangerousDriftCount
		fidelityScores = append(fidelityScores, outcome.fidelityScore)
		usefulnessScores = append(usefulnessScores, outcome.usefulnessScore)
		if outcome.result.DownstreamEquivalence != nil {
			downstreamScores = append(downstreamScores, outcome.downstreamScore)
		}
		if !outcome.passed {
			passed = false
		}
	}

	for _, checkpoint := range checkpoints {
		if deps.ThresholdUnit == SummaryStrategyThresholdTokens {
			var err error
			committed, haveCommitted, compressedUntilTurn, observedUntilTurn, summaryCalls, err =
				advanceTokenThresholdStrategy(
					ctx,
					deps,
					sample.Input.SourceMessages,
					threshold,
					checkpoint.AfterTurn,
					committed,
					haveCommitted,
					compressedUntilTurn,
					observedUntilTurn,
					summaryCalls,
				)
			if err != nil {
				return SummaryStrategyThresholdResult{}, err
			}
		} else {
			for nextTriggerTurn > 0 && nextTriggerTurn < checkpoint.AfterTurn {
				generated, err := generateSummarySnapshot(ctx, deps.Generator, sample.Input.SourceMessages, compressedUntilTurn, nextTriggerTurn, committed, haveCommitted)
				if err != nil {
					return SummaryStrategyThresholdResult{}, fmt.Errorf("threshold %d trigger at turn %d: %w", threshold, nextTriggerTurn, err)
				}
				committed = generated
				haveCommitted = true
				compressedUntilTurn = nextTriggerTurn
				nextTriggerTurn += threshold
				summaryCalls++
			}
		}

		outcome, err := evaluateSummaryStrategyCheckpoint(ctx, deps, sample, checkpoint, compressedUntilTurn, committed, haveCommitted)
		if err != nil {
			return SummaryStrategyThresholdResult{}, fmt.Errorf("threshold %d checkpoint %d: %w", threshold, checkpoint.AfterTurn, err)
		}
		applyOutcome(outcome)
	}

	if sample.StrategyEval.FinalEval != nil {
		finalCheckpoint := *sample.StrategyEval.FinalEval
		if finalCheckpoint.AfterTurn <= 0 {
			finalCheckpoint.AfterTurn = turnCount
		}
		if deps.ThresholdUnit == SummaryStrategyThresholdTokens {
			var err error
			committed, haveCommitted, compressedUntilTurn, observedUntilTurn, summaryCalls, err =
				advanceTokenThresholdStrategy(
					ctx,
					deps,
					sample.Input.SourceMessages,
					threshold,
					finalCheckpoint.AfterTurn,
					committed,
					haveCommitted,
					compressedUntilTurn,
					observedUntilTurn,
					summaryCalls,
				)
			if err != nil {
				return SummaryStrategyThresholdResult{}, err
			}
		} else {
			for nextTriggerTurn > 0 && nextTriggerTurn <= finalCheckpoint.AfterTurn {
				generated, err := generateSummarySnapshot(ctx, deps.Generator, sample.Input.SourceMessages, compressedUntilTurn, nextTriggerTurn, committed, haveCommitted)
				if err != nil {
					return SummaryStrategyThresholdResult{}, fmt.Errorf("threshold %d final_eval trigger at turn %d: %w", threshold, nextTriggerTurn, err)
				}
				committed = generated
				haveCommitted = true
				compressedUntilTurn = nextTriggerTurn
				nextTriggerTurn += threshold
				summaryCalls++
			}
		}
		outcome, err := evaluateSummaryStrategyCheckpoint(ctx, deps, sample, finalCheckpoint, compressedUntilTurn, committed, haveCommitted)
		if err != nil {
			return SummaryStrategyThresholdResult{}, fmt.Errorf("threshold %d final_eval: %w", threshold, err)
		}
		resultCopy := outcome.result
		finalResult = &resultCopy
		totalBaseline += outcome.result.TokenBaseline
		totalStrategy += outcome.result.TokenStrategy
		criticalFailureCount += outcome.criticalFailureCount
		dangerousDriftCount += outcome.dangerousDriftCount
		fidelityScores = append(fidelityScores, outcome.fidelityScore)
		usefulnessScores = append(usefulnessScores, outcome.usefulnessScore)
		if outcome.result.DownstreamEquivalence != nil {
			downstreamScores = append(downstreamScores, outcome.downstreamScore)
		}
		if !outcome.passed {
			passed = false
		}
	}

	tokenSaved := totalBaseline - totalStrategy
	return SummaryStrategyThresholdResult{
		Threshold:                  threshold,
		ThresholdUnit:              deps.ThresholdUnit,
		SummaryCallCount:           summaryCalls,
		CheckpointResults:          checkpointResults,
		FinalResult:                finalResult,
		TokenBaseline:              totalBaseline,
		TokenStrategy:              totalStrategy,
		TokenSaved:                 tokenSaved,
		TokenSavedRatio:            ratio(tokenSaved, totalBaseline),
		SummaryFidelityScore:       averageStrategyScore(fidelityScores),
		SummaryUsefulnessScore:     averageStrategyScore(usefulnessScores),
		DownstreamEquivalenceScore: averageStrategyScore(downstreamScores),
		CriticalFailureCount:       criticalFailureCount,
		DangerousDriftCount:        dangerousDriftCount,
		Passed:                     passed,
	}, nil
}

func evaluateSummaryStrategyCheckpoint(
	ctx context.Context,
	deps summaryStrategyDependencies,
	sample SummarySample,
	checkpoint SummaryStrategyCheckpoint,
	compressedUntilTurn int,
	committed SummaryGenerationOutput,
	haveCommitted bool,
) (summaryStrategyCheckpointOutcome, error) {
	snapshot, err := generateSummarySnapshot(ctx, deps.Generator, sample.Input.SourceMessages, compressedUntilTurn, checkpoint.AfterTurn, committed, haveCommitted)
	if err != nil {
		return summaryStrategyCheckpointOutcome{}, err
	}

	checkpointSample := buildCheckpointSample(sample, checkpoint)
	rules := EvaluateSummaryRules(checkpointSample, snapshot.Structured)
	var fieldJudge *SummaryFieldJudgeEvaluation
	var equivalence *SummaryEquivalenceEvaluation
	dangerousDriftCount := 0
	passed := len(rules.CriticalFailures) == 0

	if deps.Judge != nil {
		fieldJudgeResult, err := RunSummaryFieldJudge(ctx, deps.Judge, checkpointSample, snapshot.Structured)
		if err != nil {
			return summaryStrategyCheckpointOutcome{}, fmt.Errorf("field judge: %w", err)
		}
		fieldJudge = &fieldJudgeResult
		if deps.AnswerGenerator != nil && len(checkpoint.NextTurnEval.Queries) > 0 {
			fullContext := renderSourceMessages(messagesThroughTurn(sample.Input.SourceMessages, checkpoint.AfterTurn))
			strategyContext := buildStrategyAnswerContext(committed.Rendered, summaryMessagesBetweenTurns(sample.Input.SourceMessages, compressedUntilTurn, checkpoint.AfterTurn))
			equivalenceResult, err := runSummaryEquivalenceWithContexts(ctx, deps.AnswerGenerator, deps.Judge, checkpoint.NextTurnEval.Queries, fullContext, strategyContext)
			if err != nil {
				return summaryStrategyCheckpointOutcome{}, fmt.Errorf("equivalence: %w", err)
			}
			equivalence = &equivalenceResult
			for _, query := range equivalenceResult.Queries {
				if query.DangerousDrift {
					dangerousDriftCount++
					passed = false
				}
			}
		}
	}

	usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
		FullMessages:          messagesThroughTurn(sample.Input.SourceMessages, checkpoint.AfterTurn),
		SummaryText:           committed.Rendered,
		TailMessages:          summaryMessagesBetweenTurns(sample.Input.SourceMessages, compressedUntilTurn, checkpoint.AfterTurn),
		Estimator:             deps.Estimator,
		MessageOverheadTokens: deps.MessageOverheadTokens,
	})
	tokenSaved := usage.BaselineTokens - usage.StrategyTokens
	result := SummaryStrategyCheckpointResult{
		AfterTurn:             checkpoint.AfterTurn,
		RuleEvaluation:        rules,
		FieldJudge:            fieldJudge,
		DownstreamEquivalence: equivalence,
		TokenBaseline:         usage.BaselineTokens,
		TokenStrategy:         usage.StrategyTokens,
		TokenSaved:            tokenSaved,
		TokenSavedRatio:       ratio(tokenSaved, usage.BaselineTokens),
	}
	return summaryStrategyCheckpointOutcome{
		result:               result,
		fidelityScore:        summaryFidelityScore(rules, fieldJudge),
		usefulnessScore:      summaryUsefulnessScore(rules, fieldJudge),
		downstreamScore:      equivalenceScore(equivalence),
		criticalFailureCount: len(rules.CriticalFailures),
		dangerousDriftCount:  dangerousDriftCount,
		passed:               passed,
	}, nil
}

func advanceTokenThresholdStrategy(
	ctx context.Context,
	deps summaryStrategyDependencies,
	messages []SummaryMessage,
	threshold int,
	targetTurn int,
	committed SummaryGenerationOutput,
	haveCommitted bool,
	compressedUntilTurn int,
	observedUntilTurn int,
	summaryCalls int,
) (SummaryGenerationOutput, bool, int, int, int, error) {
	for turn := observedUntilTurn + 1; turn <= targetTurn; turn++ {
		usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
			SummaryText:           committed.Rendered,
			TailMessages:          summaryMessagesBetweenTurns(messages, compressedUntilTurn, turn),
			Estimator:             deps.Estimator,
			MessageOverheadTokens: deps.MessageOverheadTokens,
		})
		if usage.StrategyTokens >= threshold {
			generated, err := generateSummarySnapshot(
				ctx,
				deps.Generator,
				messages,
				compressedUntilTurn,
				turn,
				committed,
				haveCommitted,
			)
			if err != nil {
				return committed, haveCommitted, compressedUntilTurn, observedUntilTurn, summaryCalls,
					fmt.Errorf("threshold %d tokens trigger at turn %d: %w", threshold, turn, err)
			}
			committed = generated
			haveCommitted = true
			compressedUntilTurn = turn
			summaryCalls++
		}
		observedUntilTurn = turn
	}
	return committed, haveCommitted, compressedUntilTurn, observedUntilTurn, summaryCalls, nil
}

func equivalenceScore(equivalence *SummaryEquivalenceEvaluation) float64 {
	if equivalence == nil {
		return 0
	}
	return equivalence.Score
}

func buildStrategyAnswerContext(summaryText string, tail []SummaryMessage) string {
	parts := make([]string, 0, 2)
	if summaryText = strings.TrimSpace(summaryText); summaryText != "" {
		parts = append(parts, summaryText)
	}
	if tailContext := strings.TrimSpace(renderSourceMessages(tail)); tailContext != "" {
		parts = append(parts, tailContext)
	}
	return strings.Join(parts, "\n")
}

func generateSummarySnapshot(ctx context.Context, generator SummaryGenerator, messages []SummaryMessage, compressedUntilTurn int, targetTurn int, committed SummaryGenerationOutput, haveCommitted bool) (SummaryGenerationOutput, error) {
	if haveCommitted && compressedUntilTurn == targetTurn {
		return committed, nil
	}
	input := SummaryGenerationInput{
		SourceMessages: summaryMessagesBetweenTurns(messages, compressedUntilTurn, targetTurn),
	}
	if haveCommitted {
		previous := committed.Structured
		input.PreviousSummary = &previous
	}
	return generator.Generate(ctx, input)
}

func buildCheckpointSample(sample SummarySample, checkpoint SummaryStrategyCheckpoint) SummarySample {
	return SummarySample{
		Name:             sample.Name,
		Tags:             append([]string(nil), sample.Tags...),
		Input:            SummaryInput{SourceMessages: messagesThroughTurn(sample.Input.SourceMessages, checkpoint.AfterTurn)},
		ExpectedSummary:  checkpoint.ExpectedSummary,
		CriticalContract: checkpoint.CriticalContract,
		NextTurnEval:     checkpoint.NextTurnEval,
	}
}

func messagesThroughTurn(messages []SummaryMessage, afterTurn int) []SummaryMessage {
	if afterTurn <= 0 {
		return nil
	}
	limit := afterTurn * 2
	if limit > len(messages) {
		limit = len(messages)
	}
	return append([]SummaryMessage(nil), messages[:limit]...)
}

func summaryMessagesBetweenTurns(messages []SummaryMessage, fromTurnExclusive int, toTurnInclusive int) []SummaryMessage {
	if toTurnInclusive <= fromTurnExclusive {
		return nil
	}
	start := fromTurnExclusive * 2
	end := toTurnInclusive * 2
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	if end > len(messages) {
		end = len(messages)
	}
	if start >= end {
		return nil
	}
	return append([]SummaryMessage(nil), messages[start:end]...)
}

func normalizeStrategyThresholds(thresholds []int) []int {
	if len(thresholds) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	normalized := make([]int, 0, len(thresholds))
	for _, threshold := range thresholds {
		if threshold <= 0 {
			continue
		}
		if _, exists := seen[threshold]; exists {
			continue
		}
		seen[threshold] = struct{}{}
		normalized = append(normalized, threshold)
	}
	sort.Ints(normalized)
	return normalized
}

func averageStrategyScore(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return roundSummaryScore(total / float64(len(values)))
}

func ratio(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return roundSummaryScore(float64(numerator) / float64(denominator))
}

func buildStrategyParetoCandidates(results []SummaryStrategyThresholdResult) []int {
	if len(results) == 0 {
		return nil
	}
	candidates := make([]int, 0, len(results))
	for i, candidate := range results {
		dominated := false
		for j, other := range results {
			if i == j {
				continue
			}
			if other.TokenSavedRatio >= candidate.TokenSavedRatio &&
				other.SummaryFidelityScore >= candidate.SummaryFidelityScore &&
				other.SummaryUsefulnessScore >= candidate.SummaryUsefulnessScore &&
				other.DownstreamEquivalenceScore >= candidate.DownstreamEquivalenceScore &&
				(other.TokenSavedRatio > candidate.TokenSavedRatio ||
					other.SummaryFidelityScore > candidate.SummaryFidelityScore ||
					other.SummaryUsefulnessScore > candidate.SummaryUsefulnessScore ||
					other.DownstreamEquivalenceScore > candidate.DownstreamEquivalenceScore) {
				dominated = true
				break
			}
		}
		if !dominated {
			candidates = append(candidates, candidate.Threshold)
		}
	}
	sort.Ints(candidates)
	return candidates
}

func buildSummaryStrategyThresholdAggregates(results []SummaryStrategySampleResult) []SummaryStrategyThresholdAggregate {
	if len(results) == 0 {
		return nil
	}
	accumulators := map[summaryStrategyAggregateKey]*summaryStrategyThresholdAggregateAccumulator{}
	for _, sampleResult := range results {
		for _, thresholdResult := range sampleResult.ThresholdResults {
			key := summaryStrategyAggregateKey{
				unit:      thresholdResult.ThresholdUnit,
				threshold: thresholdResult.Threshold,
			}
			acc := accumulators[key]
			if acc == nil {
				acc = &summaryStrategyThresholdAggregateAccumulator{}
				accumulators[key] = acc
			}
			acc.sampleCount++
			if thresholdResult.Passed {
				acc.passedCount++
			}
			if thresholdResult.CriticalFailureCount > 0 {
				acc.criticalFailureSamples++
			}
			if thresholdResult.DangerousDriftCount > 0 {
				acc.dangerousDriftSamples++
			}
			acc.tokenSavedRatios = append(acc.tokenSavedRatios, thresholdResult.TokenSavedRatio)
			acc.fidelityScores = append(acc.fidelityScores, thresholdResult.SummaryFidelityScore)
			acc.usefulnessScores = append(acc.usefulnessScores, thresholdResult.SummaryUsefulnessScore)
			acc.downstreamScores = append(acc.downstreamScores, thresholdResult.DownstreamEquivalenceScore)
			acc.summaryCallCounts = append(acc.summaryCallCounts, float64(thresholdResult.SummaryCallCount))
		}
	}
	keys := make([]summaryStrategyAggregateKey, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].unit != keys[j].unit {
			return keys[i].unit < keys[j].unit
		}
		return keys[i].threshold < keys[j].threshold
	})
	aggregates := make([]SummaryStrategyThresholdAggregate, 0, len(keys))
	for _, key := range keys {
		acc := accumulators[key]
		aggregates = append(aggregates, SummaryStrategyThresholdAggregate{
			Threshold:                key.threshold,
			ThresholdUnit:            key.unit,
			SampleCount:              acc.sampleCount,
			AvgTokenSavedRatio:       averageStrategyScore(acc.tokenSavedRatios),
			AvgStructuredFidelity:    averageStrategyScore(acc.fidelityScores),
			AvgStructuredUsefulness:  averageStrategyScore(acc.usefulnessScores),
			AvgDownstreamEquivalence: averageStrategyScore(acc.downstreamScores),
			AvgSummaryCallCount:      averageStrategyScore(acc.summaryCallCounts),
			PassRate:                 rate(acc.passedCount, acc.sampleCount),
			CriticalFailureRate:      rate(acc.criticalFailureSamples, acc.sampleCount),
			DangerousDriftRate:       rate(acc.dangerousDriftSamples, acc.sampleCount),
		})
	}
	return aggregates
}

func buildStrategySharedSampleResult(sample SummarySample, result SummaryStrategySampleResult) SharedSampleResult {
	passed := true
	criticalFailures := make([]string, 0)
	failureReasons := make([]string, 0)
	tokenSavedRatios := make([]float64, 0, len(result.ThresholdResults))
	fidelityScores := make([]float64, 0, len(result.ThresholdResults))
	usefulnessScores := make([]float64, 0, len(result.ThresholdResults))
	downstreamScores := make([]float64, 0, len(result.ThresholdResults))
	var thresholdUnit SummaryStrategyThresholdUnit
	for _, thresholdResult := range result.ThresholdResults {
		if thresholdUnit == "" {
			thresholdUnit = thresholdResult.ThresholdUnit
		}
		tokenSavedRatios = append(tokenSavedRatios, thresholdResult.TokenSavedRatio)
		fidelityScores = append(fidelityScores, thresholdResult.SummaryFidelityScore)
		usefulnessScores = append(usefulnessScores, thresholdResult.SummaryUsefulnessScore)
		downstreamScores = append(downstreamScores, thresholdResult.DownstreamEquivalenceScore)
		if !thresholdResult.Passed {
			passed = false
			criticalFailures = append(criticalFailures, fmt.Sprintf("strategy_threshold_failed:%d", thresholdResult.Threshold))
			failureReasons = append(failureReasons, fmt.Sprintf("strategy threshold %d failed", thresholdResult.Threshold))
		}
	}
	return SharedSampleResult{
		Name:             sample.Name,
		Tags:             sample.Tags,
		Passed:           passed,
		CriticalFailures: criticalFailures,
		RuleChecks: map[string]any{
			"strategy_mode":     true,
			"threshold_unit":    string(thresholdUnit),
			"threshold_count":   len(result.ThresholdResults),
			"pareto_candidates": append([]int(nil), result.ParetoCandidates...),
		},
		Scores: map[string]any{
			"avg_token_saved_ratio":      averageStrategyScore(tokenSavedRatios),
			"avg_structured_fidelity":    averageStrategyScore(fidelityScores),
			"avg_structured_usefulness":  averageStrategyScore(usefulnessScores),
			"avg_downstream_equivalence": averageStrategyScore(downstreamScores),
		},
		FailureReasons: failureReasons,
	}
}
