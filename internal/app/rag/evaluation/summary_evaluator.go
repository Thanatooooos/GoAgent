package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type SummaryEvalMode string

const (
	SummaryEvalModeStandard SummaryEvalMode = "standard"
	SummaryEvalModeStrategy SummaryEvalMode = "strategy"
)

type SummaryStrategyThresholdUnit string

const (
	SummaryStrategyThresholdTokens SummaryStrategyThresholdUnit = "tokens"
	SummaryStrategyThresholdTurns  SummaryStrategyThresholdUnit = "turns"
)

const defaultSummaryStrategyMessageOverheadTokens = 4

type SummaryEvaluatorRuntimeOptions struct {
	Mode                  SummaryEvalMode
	ThresholdUnit         SummaryStrategyThresholdUnit
	Thresholds            []int
	MessageOverheadTokens int
}

func (o SummaryEvaluatorRuntimeOptions) normalizedMode() SummaryEvalMode {
	if o.Mode == SummaryEvalModeStrategy {
		return SummaryEvalModeStrategy
	}
	return SummaryEvalModeStandard
}

func (o SummaryEvaluatorRuntimeOptions) normalizedStrategy() (SummaryEvaluatorRuntimeOptions, error) {
	if o.normalizedMode() != SummaryEvalModeStrategy {
		return o, nil
	}
	if o.ThresholdUnit == "" {
		o.ThresholdUnit = SummaryStrategyThresholdTurns
	}
	switch o.ThresholdUnit {
	case SummaryStrategyThresholdTokens, SummaryStrategyThresholdTurns:
	default:
		return SummaryEvaluatorRuntimeOptions{}, fmt.Errorf(
			"unsupported summary strategy threshold unit %q",
			o.ThresholdUnit,
		)
	}
	o.Thresholds = normalizeStrategyThresholds(o.Thresholds)
	if len(o.Thresholds) == 0 {
		return SummaryEvaluatorRuntimeOptions{}, fmt.Errorf("summary strategy thresholds are required")
	}
	if o.MessageOverheadTokens < 0 {
		o.MessageOverheadTokens = 0
	}
	if o.MessageOverheadTokens == 0 {
		o.MessageOverheadTokens = defaultSummaryStrategyMessageOverheadTokens
	}
	return o, nil
}

type SummaryEvaluator struct {
	generator       SummaryGenerator
	judge           Judge
	answerGenerator SummaryAnswerGenerator
	runtime         SummaryEvaluatorRuntimeOptions
}

type SummaryEvaluatorOption func(*SummaryEvaluator)

func WithSummaryJudge(judge Judge) SummaryEvaluatorOption {
	return func(e *SummaryEvaluator) {
		e.judge = judge
	}
}

func WithSummaryAnswerGenerator(generator SummaryAnswerGenerator) SummaryEvaluatorOption {
	return func(e *SummaryEvaluator) {
		e.answerGenerator = generator
	}
}

func WithSummaryRuntimeOptions(options SummaryEvaluatorRuntimeOptions) SummaryEvaluatorOption {
	return func(e *SummaryEvaluator) {
		e.runtime = options
	}
}

func NewSummaryEvaluator(generator SummaryGenerator, opts ...SummaryEvaluatorOption) *SummaryEvaluator {
	evaluator := &SummaryEvaluator{generator: generator}
	for _, opt := range opts {
		if opt != nil {
			opt(evaluator)
		}
	}
	return evaluator
}

func (e *SummaryEvaluator) Suite() SuiteName {
	return SuiteSummary
}

func (e *SummaryEvaluator) LoadSamples(_ context.Context, path string) (json.RawMessage, error) {
	raw, err := LoadRawSampleFile(path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (e *SummaryEvaluator) Run(ctx context.Context, input RunInput) (SuiteResult, error) {
	if e == nil || e.generator == nil {
		return SuiteResult{}, fmt.Errorf("summary generator is required")
	}
	runtimeOptions, err := e.runtime.normalizedStrategy()
	if err != nil {
		return SuiteResult{}, err
	}
	samples, err := ParseSummarySamples(input.RawSamples)
	if err != nil {
		return SuiteResult{}, err
	}

	results := make([]SharedSampleResult, 0, len(samples))
	strategySampleResults := make([]SummaryStrategySampleResult, 0, len(samples))
	tagStats := map[string]*tagSummaryAccumulator{}
	criticalFailureCount := 0
	artifacts := map[string]any{}

	for _, sample := range samples {
		if runtimeOptions.normalizedMode() == SummaryEvalModeStrategy {
			strategyResult, err := runSummaryStrategySweep(ctx, summaryStrategyDependencies{
				Generator:             e.generator,
				Judge:                 e.judge,
				AnswerGenerator:       e.answerGenerator,
				ThresholdUnit:         runtimeOptions.ThresholdUnit,
				Thresholds:            runtimeOptions.Thresholds,
				MessageOverheadTokens: runtimeOptions.MessageOverheadTokens,
			}, sample)
			if err != nil {
				return SuiteResult{}, fmt.Errorf("run strategy sample %q: %w", sample.Name, err)
			}
			strategySampleResults = append(strategySampleResults, strategyResult)
			sampleResult := buildStrategySharedSampleResult(sample, strategyResult)
			results = append(results, sampleResult)
			artifacts[sample.Name] = buildSummaryStrategyArtifact(strategyResult)
			if len(sampleResult.CriticalFailures) > 0 {
				criticalFailureCount++
			}
			for _, tag := range sample.Tags {
				acc := tagStats[tag]
				if acc == nil {
					acc = &tagSummaryAccumulator{}
					tagStats[tag] = acc
				}
				acc.total++
				if sampleResult.Passed {
					acc.passed++
				}
				if len(sampleResult.CriticalFailures) > 0 {
					acc.criticalFailures++
				}
			}
			continue
		}

		generated, err := e.generator.Generate(ctx, SummaryGenerationInput{
			SourceMessages:  append([]SummaryMessage(nil), sample.Input.SourceMessages...),
			PreviousSummary: sample.Input.PreviousSummary,
		})
		if err != nil {
			return SuiteResult{}, fmt.Errorf("generate summary for sample %q: %w", sample.Name, err)
		}
		rules := EvaluateSummaryRules(sample, generated.Structured)
		var fieldJudge *SummaryFieldJudgeEvaluation
		var equivalence *SummaryEquivalenceEvaluation
		var judgeChecks map[string]any
		if e.judge != nil {
			fieldJudgeResult, err := RunSummaryFieldJudge(ctx, e.judge, sample, generated.Structured)
			if err != nil {
				return SuiteResult{}, fmt.Errorf("field judge for sample %q: %w", sample.Name, err)
			}
			fieldJudge = &fieldJudgeResult
			judgeChecks = map[string]any{
				"field_level": fieldJudgeResult,
			}
			rules.Passed = rules.Passed && fieldJudgeResult.Passed
			if e.answerGenerator != nil && len(sample.NextTurnEval.Queries) > 0 {
				equivalenceResult, err := RunSummaryEquivalence(ctx, e.answerGenerator, e.judge, sample, generated)
				if err != nil {
					return SuiteResult{}, fmt.Errorf("equivalence judge for sample %q: %w", sample.Name, err)
				}
				equivalence = &equivalenceResult
				judgeChecks["downstream_equivalence"] = equivalenceResult
				if !equivalenceResult.Passed {
					rules.Passed = false
				}
				for _, query := range equivalenceResult.Queries {
					if !query.DangerousDrift {
						continue
					}
					rules.CriticalFailures = append(rules.CriticalFailures, "dangerous_downstream_drift:"+query.ID)
					rules.FailureReasons = append(rules.FailureReasons, "dangerous downstream drift on "+query.ID)
				}
			}
		}
		sampleResult := SharedSampleResult{
			Name:             sample.Name,
			Tags:             sample.Tags,
			Passed:           rules.Passed,
			CriticalFailures: append([]string(nil), rules.CriticalFailures...),
			RuleChecks: map[string]any{
				"schema_valid":               rules.SchemaValid,
				"required_fields_ok":         rules.RequiredFieldsOK,
				"forbidden_claims_ok":        rules.ForbiddenClaimsOK,
				"critical_entities_ok":       rules.CriticalEntitiesOK,
				"critical_items_ok":          rules.CriticalItemsOK,
				"open_questions_ok":          rules.OpenQuestionsOK,
				"critical_open_questions_ok": rules.CriticalOpenQuestionsOK,
				"state_override_ok":          rules.StateOverrideOK,
			},
			JudgeChecks:    judgeChecks,
			Scores:         buildSummaryScore(rules, fieldJudge, equivalence),
			FailureReasons: append([]string(nil), rules.FailureReasons...),
		}
		results = append(results, sampleResult)
		artifacts[sample.Name] = buildSummarySampleArtifact(sample, generated, rules, fieldJudge, equivalence)
		if len(rules.CriticalFailures) > 0 {
			criticalFailureCount++
		}
		for _, tag := range sample.Tags {
			acc := tagStats[tag]
			if acc == nil {
				acc = &tagSummaryAccumulator{}
				tagStats[tag] = acc
			}
			acc.total++
			if sampleResult.Passed {
				acc.passed++
			}
			if len(rules.CriticalFailures) > 0 {
				acc.criticalFailures++
			}
		}
	}

	metrics := map[string]any{
		"sample_count": len(results),
	}
	if runtimeOptions.normalizedMode() == SummaryEvalModeStrategy {
		metrics["threshold_aggregates"] = buildSummaryStrategyThresholdAggregates(strategySampleResults)
	}
	aggregate := SharedAggregateResult{
		PassRate:            rate(countPassed(results), len(results)),
		CriticalFailureRate: rate(criticalFailureCount, len(results)),
		ByTag:               buildTagAggregates(tagStats),
		Metrics:             metrics,
	}

	return SuiteResult{
		Suite: string(SuiteSummary),
		RunMetadata: RunMetadata{
			RunAt:       time.Now().UTC().Format(time.RFC3339),
			Suite:       string(SuiteSummary),
			SampleSetID: input.InputPath,
		},
		Samples:   results,
		Aggregate: aggregate,
		Artifacts: map[string]any{"executions": artifacts},
	}, nil
}

type tagSummaryAccumulator struct {
	total            int
	passed           int
	criticalFailures int
}

func countPassed(results []SharedSampleResult) int {
	total := 0
	for _, result := range results {
		if result.Passed {
			total++
		}
	}
	return total
}

func buildTagAggregates(stats map[string]*tagSummaryAccumulator) []TagAggregate {
	if len(stats) == 0 {
		return nil
	}
	tags := make([]string, 0, len(stats))
	for tag := range stats {
		tags = append(tags, tag)
	}
	sortStrings(tags)

	aggregates := make([]TagAggregate, 0, len(tags))
	for _, tag := range tags {
		stat := stats[tag]
		aggregates = append(aggregates, TagAggregate{
			Tag: tag,
			Metrics: map[string]any{
				"sample_count":          stat.total,
				"pass_rate":             rate(stat.passed, stat.total),
				"critical_failure_rate": rate(stat.criticalFailures, stat.total),
			},
		})
	}
	return aggregates
}
