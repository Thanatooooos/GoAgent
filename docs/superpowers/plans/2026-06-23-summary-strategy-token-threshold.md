# Summary Strategy Token Threshold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add token-budget thresholds to summary strategy evaluation while preserving the existing turn-threshold mode and making the selected unit explicit in CLI options, runtime behavior, artifacts, and aggregates.

**Architecture:** Introduce an explicit threshold-unit contract in the summary evaluator runtime options. Keep checkpoint evaluation shared, but route compression timing through either the existing turn scheduler or a new token scheduler that evaluates committed summary plus uncovered messages after every completed turn. Reuse the shared token estimator and one configured message-overhead value for both triggering and reported token savings.

**Tech Stack:** Go, standard `flag` package, existing RAG evaluation framework, shared `internal/app/rag/core/tokenbudget` estimator, table-driven Go tests.

---

## File Map

- Modify `internal/app/rag/evaluation/summary_evaluator.go`
  - define threshold units
  - normalize and validate strategy runtime options
  - pass unit, estimator, and overhead into strategy execution
- Modify `internal/app/rag/evaluation/summary_strategy.go`
  - preserve turn scheduling
  - add token scheduling
  - emit threshold units in threshold and aggregate results
- Modify `internal/app/rag/evaluation/summary_strategy_tokens.go`
  - centralize overhead-aware message and strategy token accounting
- Modify `internal/app/rag/evaluation/summary_strategy_test.go`
  - cover token trigger boundaries and legacy turn compatibility
- Modify `internal/app/rag/evaluation/summary_strategy_tokens_test.go`
  - cover message overhead and consistent token accounting
- Modify `internal/app/rag/evaluation/summary_evaluator_test.go`
  - cover runtime validation and aggregate units
- Modify `internal/app/rag/evaluation/phase1_registry_test.go`
  - prove runtime options survive registry wiring
- Modify `internal/app/rag/evaluation/summary_artifacts_test.go`
  - prove threshold units survive artifact emission
- Modify `cmd/eval-runner/main.go`
  - add `-summary-token-thresholds`
  - resolve token-over-turn precedence
  - wire configured message overhead
- Modify `cmd/eval-runner/main_test.go`
  - cover CLI parsing, precedence, validation, and compatibility
- Modify `openspec/changes/add-token-aware-summary-compression/tasks.md`
  - check the strategy-eval rollout item only after a real token sweep succeeds
- Create runtime artifact `tmp/summary_strategy_token_thresholds_20260623.json`
  - generated evidence only; do not commit unless explicitly requested

## Task 1: Define the Explicit Strategy Runtime Contract

**Files:**

- Modify: `internal/app/rag/evaluation/summary_evaluator.go`
- Modify: `internal/app/rag/evaluation/summary_evaluator_test.go`
- Modify: `internal/app/rag/evaluation/phase1_registry_test.go`

- [ ] **Step 1: Write failing runtime-option tests**

Add tests equivalent to:

```go
func TestSummaryEvaluatorRuntimeOptionsNormalizeTokenStrategy(t *testing.T) {
	options, err := (SummaryEvaluatorRuntimeOptions{
		Mode:                  SummaryEvalModeStrategy,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{1600, 800, 1600, -1},
		MessageOverheadTokens: 4,
	}).normalizedStrategy()
	if err != nil {
		t.Fatalf("normalizedStrategy() error = %v", err)
	}
	if options.ThresholdUnit != SummaryStrategyThresholdTokens {
		t.Fatalf("ThresholdUnit = %q", options.ThresholdUnit)
	}
	if !reflect.DeepEqual(options.Thresholds, []int{800, 1600}) {
		t.Fatalf("Thresholds = %#v", options.Thresholds)
	}
}

func TestSummaryEvaluatorRuntimeOptionsRejectStrategyWithoutThresholds(t *testing.T) {
	_, err := (SummaryEvaluatorRuntimeOptions{
		Mode:          SummaryEvalModeStrategy,
		ThresholdUnit: SummaryStrategyThresholdTokens,
	}).normalizedStrategy()
	if err == nil || !strings.Contains(err.Error(), "strategy thresholds are required") {
		t.Fatalf("error = %v", err)
	}
}

func TestSummaryEvaluatorRuntimeOptionsRejectUnsupportedUnit(t *testing.T) {
	_, err := (SummaryEvaluatorRuntimeOptions{
		Mode:          SummaryEvalModeStrategy,
		ThresholdUnit: "characters",
		Thresholds:    []int{1000},
	}).normalizedStrategy()
	if err == nil || !strings.Contains(err.Error(), "unsupported summary strategy threshold unit") {
		t.Fatalf("error = %v", err)
	}
}
```

Update the registry test to pass:

```go
SummaryOptions: SummaryEvaluatorRuntimeOptions{
	Mode:                  SummaryEvalModeStrategy,
	ThresholdUnit:         SummaryStrategyThresholdTokens,
	Thresholds:            []int{800, 1200},
	MessageOverheadTokens: 4,
}
```

and assert all four fields are preserved.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestSummaryEvaluatorRuntimeOptions|TestNewPhase1RegistryForSummaryPassesRuntimeOptions' -count=1
```

Expected: build failure because `SummaryStrategyThresholdTokens`,
`ThresholdUnit`, and `normalizedStrategy` do not exist.

- [ ] **Step 3: Implement the runtime contract**

Add to `summary_evaluator.go`:

```go
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

func (o SummaryEvaluatorRuntimeOptions) normalizedStrategy() (SummaryEvaluatorRuntimeOptions, error) {
	if o.normalizedMode() != SummaryEvalModeStrategy {
		return o, nil
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
```

At the beginning of `SummaryEvaluator.Run`, normalize strategy options once and
return the validation error before executing samples:

```go
runtimeOptions, err := e.runtime.normalizedStrategy()
if err != nil {
	return SuiteResult{}, err
}
```

Use `runtimeOptions` for all later strategy-mode checks and dependencies.

- [ ] **Step 4: Run the tests and verify GREEN**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestSummaryEvaluatorRuntimeOptions|TestNewPhase1RegistryForSummaryPassesRuntimeOptions' -count=1
```

Expected: PASS.

- [ ] **Step 5: Review and stage only Task 1 hunks**

Run:

```powershell
git diff -- internal/app/rag/evaluation/summary_evaluator.go internal/app/rag/evaluation/summary_evaluator_test.go internal/app/rag/evaluation/phase1_registry_test.go
git add -p -- internal/app/rag/evaluation/summary_evaluator.go internal/app/rag/evaluation/summary_evaluator_test.go internal/app/rag/evaluation/phase1_registry_test.go
git diff --cached --check
```

Do not stage unrelated pre-existing evaluation changes. Commit only if the
staged diff contains exclusively this task:

```powershell
git commit -m "refactor: define summary strategy threshold units"
```

## Task 2: Make Token Accounting Overhead-Aware

**Files:**

- Modify: `internal/app/rag/evaluation/summary_strategy_tokens.go`
- Modify: `internal/app/rag/evaluation/summary_strategy_tokens_test.go`

- [ ] **Step 1: Write failing accounting tests**

Add:

```go
func TestEstimateSummaryMessagesTokensAddsMessageOverhead(t *testing.T) {
	got := estimateSummaryMessagesTokens(
		[]SummaryMessage{
			{Role: "user", Content: "a"},
			{Role: "assistant", Content: "b"},
		},
		tokenbudget.FixedEstimator(10),
		4,
	)
	if got != 28 {
		t.Fatalf("tokens = %d, want 28", got)
	}
}

func TestEstimateStrategyTokenUsageUsesSameOverheadForBaselineAndTail(t *testing.T) {
	usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
		FullMessages: []SummaryMessage{
			{Role: "user", Content: "a"},
			{Role: "assistant", Content: "b"},
		},
		SummaryText:          "summary",
		TailMessages:         []SummaryMessage{{Role: "assistant", Content: "b"}},
		Estimator:            tokenbudget.FixedEstimator(10),
		MessageOverheadTokens: 4,
	})
	if usage.BaselineTokens != 28 {
		t.Fatalf("BaselineTokens = %d, want 28", usage.BaselineTokens)
	}
	if usage.StrategyTokens != 24 {
		t.Fatalf("StrategyTokens = %d, want 24", usage.StrategyTokens)
	}
}
```

Import the shared estimator package:

```go
import "local/rag-project/internal/app/rag/core/tokenbudget"
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestEstimateSummaryMessagesTokensAddsMessageOverhead|TestEstimateStrategyTokenUsageUsesSameOverheadForBaselineAndTail' -count=1
```

Expected: build failure because the accounting functions do not accept
`MessageOverheadTokens`.

- [ ] **Step 3: Implement shared-estimator accounting**

Replace the session-recall-specific estimator dependency with:

```go
import "local/rag-project/internal/app/rag/core/tokenbudget"
```

Update the input and helpers:

```go
type strategyTokenUsageInput struct {
	FullMessages          []SummaryMessage
	SummaryText           string
	TailMessages          []SummaryMessage
	Estimator             tokenbudget.Estimator
	MessageOverheadTokens int
}

func estimateSummaryMessagesTokens(
	messages []SummaryMessage,
	estimator tokenbudget.Estimator,
	messageOverheadTokens int,
) int {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	if messageOverheadTokens < 0 {
		messageOverheadTokens = 0
	}
	total := 0
	for _, message := range messages {
		if content := strings.TrimSpace(message.Content); content != "" {
			total += estimator.EstimateTokens(content) + messageOverheadTokens
		}
	}
	return total
}
```

Use the same overhead value for baseline and tail calculations. The rendered
summary receives no message overhead.

- [ ] **Step 4: Update the existing token-usage test**

Pass `MessageOverheadTokens: 4` to the existing
`TestEstimateStrategyTokenUsageCountsSummaryPlusTail` and retain its behavioral
assertions.

- [ ] **Step 5: Run the accounting suite**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestEstimate.*Token' -count=1
```

Expected: PASS.

- [ ] **Step 6: Stage and optionally commit Task 2**

Run:

```powershell
git add -p -- internal/app/rag/evaluation/summary_strategy_tokens.go internal/app/rag/evaluation/summary_strategy_tokens_test.go
git diff --cached --check
```

If the staged diff is isolated:

```powershell
git commit -m "refactor: unify strategy token accounting"
```

## Task 3: Add the Token Trigger Scheduler

**Files:**

- Modify: `internal/app/rag/evaluation/summary_strategy.go`
- Modify: `internal/app/rag/evaluation/summary_strategy_test.go`

- [ ] **Step 1: Add a deterministic recording generator for trigger tests**

In `summary_strategy_test.go`, add:

```go
type recordingStrategyGenerator struct {
	inputs []SummaryGenerationInput
}

func (g *recordingStrategyGenerator) Generate(
	_ context.Context,
	input SummaryGenerationInput,
) (SummaryGenerationOutput, error) {
	g.inputs = append(g.inputs, input)
	call := len(g.inputs)
	return SummaryGenerationOutput{
		Structured: raghistory.StructuredSummary{
			SchemaVersion: 1,
			Goal:          fmt.Sprintf("summary-%d", call),
		},
		Rendered: fmt.Sprintf("summary-%d", call),
	}, nil
}
```

- [ ] **Step 2: Write failing token-trigger boundary tests**

Add a helper sample with two completed turns and one checkpoint after turn 2.
Then add:

```go
func TestRunSummaryStrategyTokenThresholdDoesNotTriggerBelowBudget(t *testing.T) {
	generator := &recordingStrategyGenerator{}
	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:             generator,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{21},
		Estimator:             tokenbudget.FixedEstimator(5),
		MessageOverheadTokens: 0,
	}, twoTurnStrategySample())
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if result.ThresholdResults[0].SummaryCallCount != 0 {
		t.Fatalf("SummaryCallCount = %d, want 0", result.ThresholdResults[0].SummaryCallCount)
	}
}

func TestRunSummaryStrategyTokenThresholdTriggersAtEqualBudget(t *testing.T) {
	generator := &recordingStrategyGenerator{}
	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:             generator,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{10},
		Estimator:             tokenbudget.FixedEstimator(5),
		MessageOverheadTokens: 0,
	}, twoTurnStrategySample())
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if result.ThresholdResults[0].SummaryCallCount != 2 {
		t.Fatalf("SummaryCallCount = %d, want 2", result.ThresholdResults[0].SummaryCallCount)
	}
}
```

Use message contents and fixed estimates so each completed turn contributes
exactly 10 tokens. The checkpoint evaluation call must not be counted as a
committed strategy compression.

- [ ] **Step 3: Write failing summary-plus-tail and overhead tests**

Add:

```go
func TestRunSummaryStrategyTokenThresholdUsesCommittedSummaryPlusTail(t *testing.T) {
	generator := &recordingStrategyGenerator{}
	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:             generator,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{15},
		Estimator:             tokenbudget.FixedEstimator(5),
		MessageOverheadTokens: 0,
	}, threeTurnStrategySample())
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if result.ThresholdResults[0].SummaryCallCount != 2 {
		t.Fatalf("SummaryCallCount = %d, want 2", result.ThresholdResults[0].SummaryCallCount)
	}
}

func TestRunSummaryStrategyTokenThresholdCountsMessageOverhead(t *testing.T) {
	generator := &recordingStrategyGenerator{}
	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:             generator,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{18},
		Estimator:             tokenbudget.FixedEstimator(5),
		MessageOverheadTokens: 4,
	}, oneTurnStrategySample())
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if result.ThresholdResults[0].SummaryCallCount != 1 {
		t.Fatalf("SummaryCallCount = %d, want 1", result.ThresholdResults[0].SummaryCallCount)
	}
}
```

- [ ] **Step 4: Run trigger tests and verify RED**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestRunSummaryStrategyTokenThreshold' -count=1
```

Expected: build failure because `ThresholdUnit` and token scheduling are absent.

- [ ] **Step 5: Extend strategy dependencies and result types**

Use the shared estimator:

```go
type summaryStrategyDependencies struct {
	Generator             SummaryGenerator
	Judge                 Judge
	AnswerGenerator       SummaryAnswerGenerator
	ThresholdUnit         SummaryStrategyThresholdUnit
	Thresholds            []int
	Estimator             tokenbudget.Estimator
	MessageOverheadTokens int
}
```

Add units to output:

```go
ThresholdUnit SummaryStrategyThresholdUnit `json:"threshold_unit"`
```

on both `SummaryStrategyThresholdResult` and
`SummaryStrategyThresholdAggregate`.

- [ ] **Step 6: Implement token scheduling without duplicating checkpoint logic**

Extract the existing turn loop into:

```go
func runTurnThresholdCommits(
	ctx context.Context,
	deps summaryStrategyDependencies,
	messages []SummaryMessage,
	threshold int,
	targetTurn int,
	state *summaryStrategyState,
) error
```

Add:

```go
func runTokenThresholdCommits(
	ctx context.Context,
	deps summaryStrategyDependencies,
	messages []SummaryMessage,
	threshold int,
	targetTurn int,
	state *summaryStrategyState,
) error {
	for turn := state.observedUntilTurn + 1; turn <= targetTurn; turn++ {
		tail := summaryMessagesBetweenTurns(
			messages,
			state.compressedUntilTurn,
			turn,
		)
		usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
			SummaryText:           state.committed.Rendered,
			TailMessages:          tail,
			Estimator:             deps.Estimator,
			MessageOverheadTokens: deps.MessageOverheadTokens,
		})
		if usage.StrategyTokens >= threshold {
			generated, err := generateSummarySnapshot(
				ctx,
				deps.Generator,
				messages,
				state.compressedUntilTurn,
				turn,
				state.committed,
				state.haveCommitted,
			)
			if err != nil {
				return fmt.Errorf(
					"threshold %d tokens trigger at turn %d: %w",
					threshold,
					turn,
					err,
				)
			}
			state.committed = generated
			state.haveCommitted = true
			state.compressedUntilTurn = turn
			state.summaryCalls++
		}
		state.observedUntilTurn = turn
	}
	return nil
}
```

Define the state in one place:

```go
type summaryStrategyState struct {
	compressedUntilTurn int
	observedUntilTurn   int
	summaryCalls        int
	committed           SummaryGenerationOutput
	haveCommitted       bool
}
```

Route through a unit switch:

```go
func advanceSummaryStrategy(
	ctx context.Context,
	deps summaryStrategyDependencies,
	messages []SummaryMessage,
	threshold int,
	targetTurn int,
	state *summaryStrategyState,
) error {
	switch deps.ThresholdUnit {
	case SummaryStrategyThresholdTokens:
		return runTokenThresholdCommits(ctx, deps, messages, threshold, targetTurn, state)
	case SummaryStrategyThresholdTurns:
		return runTurnThresholdCommits(ctx, deps, messages, threshold, targetTurn, state)
	default:
		return fmt.Errorf("unsupported summary strategy threshold unit %q", deps.ThresholdUnit)
	}
}
```

Call `advanceSummaryStrategy` before each checkpoint and before final evaluation.
For ordinary checkpoints, preserve current behavior by advancing only through
turns strictly before `checkpoint.AfterTurn`; for `final_eval`, advance through
the final turn inclusively. This keeps existing checkpoint expectations stable.

- [ ] **Step 7: Pass overhead into checkpoint token totals**

Update `evaluateSummaryStrategyCheckpoint`:

```go
usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
	FullMessages:          messagesThroughTurn(sample.Input.SourceMessages, checkpoint.AfterTurn),
	SummaryText:           committed.Rendered,
	TailMessages:          summaryMessagesBetweenTurns(sample.Input.SourceMessages, compressedUntilTurn, checkpoint.AfterTurn),
	Estimator:             deps.Estimator,
	MessageOverheadTokens: deps.MessageOverheadTokens,
})
```

- [ ] **Step 8: Run token and legacy strategy tests**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestRunSummaryStrategy(TokenThreshold|Sweep)' -count=1
```

Expected: PASS, including all pre-existing turn-mode tests after explicitly
setting `ThresholdUnit: SummaryStrategyThresholdTurns` in those tests.

- [ ] **Step 9: Stage and optionally commit Task 3**

Run:

```powershell
git add -p -- internal/app/rag/evaluation/summary_strategy.go internal/app/rag/evaluation/summary_strategy_test.go
git diff --cached --check
```

If isolated:

```powershell
git commit -m "feat: evaluate summary strategy by token budget"
```

## Task 4: Propagate Threshold Units Through Results and Artifacts

**Files:**

- Modify: `internal/app/rag/evaluation/summary_strategy.go`
- Modify: `internal/app/rag/evaluation/summary_artifacts_test.go`
- Modify: `internal/app/rag/evaluation/summary_evaluator_test.go`

- [ ] **Step 1: Write failing output tests**

Update the artifact test:

```go
artifact := buildSummaryStrategyArtifact(SummaryStrategySampleResult{
	ThresholdResults: []SummaryStrategyThresholdResult{{
		Threshold:     1200,
		ThresholdUnit: SummaryStrategyThresholdTokens,
		TokenSavedRatio: 0.5,
		Passed:        true,
	}},
})
```

Assert `results[0].ThresholdUnit == SummaryStrategyThresholdTokens`.

In the evaluator aggregate test, decode:

```go
aggregates, ok := result.Aggregate.Metrics["threshold_aggregates"].([]SummaryStrategyThresholdAggregate)
if !ok {
	t.Fatalf("threshold_aggregates = %#v", result.Aggregate.Metrics["threshold_aggregates"])
}
if aggregates[0].ThresholdUnit != SummaryStrategyThresholdTokens {
	t.Fatalf("ThresholdUnit = %q", aggregates[0].ThresholdUnit)
}
```

Add an assertion that the shared sample result contains:

```go
if got := result.Samples[0].RuleChecks["threshold_unit"]; got != "tokens" {
	t.Fatalf("threshold_unit = %#v", got)
}
```

- [ ] **Step 2: Run output tests and verify RED**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -run 'TestBuildSummaryStrategyArtifact|TestSummaryEvaluatorRunStrategyModeEmitsThresholdAggregates' -count=1
```

Expected: assertions fail because units are not yet populated.

- [ ] **Step 3: Populate unit fields**

When building each threshold result:

```go
ThresholdUnit: deps.ThresholdUnit,
```

When aggregating, key by both unit and threshold to prevent accidental mixing:

```go
type summaryStrategyAggregateKey struct {
	unit      SummaryStrategyThresholdUnit
	threshold int
}
```

Populate:

```go
ThresholdUnit: key.unit,
```

In `buildStrategySharedSampleResult`, derive the single unit from the first
threshold result and emit:

```go
"threshold_unit": string(thresholdUnit),
```

Keep Pareto candidates as integer values.

- [ ] **Step 4: Run output and full evaluation tests**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/evaluation -count=1
```

Expected: PASS.

- [ ] **Step 5: Stage and optionally commit Task 4**

Run:

```powershell
git add -p -- internal/app/rag/evaluation/summary_strategy.go internal/app/rag/evaluation/summary_artifacts_test.go internal/app/rag/evaluation/summary_evaluator_test.go
git diff --cached --check
```

If isolated:

```powershell
git commit -m "feat: report summary strategy threshold units"
```

## Task 5: Add CLI Token Thresholds and Compatibility Resolution

**Files:**

- Modify: `cmd/eval-runner/main.go`
- Modify: `cmd/eval-runner/main_test.go`

- [ ] **Step 1: Write failing CLI parsing and precedence tests**

Add:

```go
func TestRunWithDepsPrefersSummaryTokenThresholds(t *testing.T) {
	var captured summaryEvalOptions
	exitCode := runWithDeps(
		[]string{
			"-suite", "summary",
			"-input", "summary.json",
			"-summary-mode", "strategy",
			"-summary-thresholds", "4,8",
			"-summary-token-thresholds", "800,1200",
		},
		io.Discard,
		io.Discard,
		evalRunnerDeps{
			buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
				return &ragbootstrap.Runtime{LLMChat: summaryOnlyLLMService{}}, nil
			},
			buildRegistry: func(
				_ *ragbootstrap.Runtime,
				_ rageval.SuiteName,
				_ []string,
				options summaryEvalOptions,
			) (*rageval.Registry, error) {
				captured = options
				return strategyTestRegistry(t), nil
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d", exitCode)
	}
	if captured.thresholdUnit != rageval.SummaryStrategyThresholdTokens {
		t.Fatalf("thresholdUnit = %q", captured.thresholdUnit)
	}
	if !reflect.DeepEqual(captured.thresholds, []int{800, 1200}) {
		t.Fatalf("thresholds = %#v", captured.thresholds)
	}
}
```

Also add:

- legacy-only flags resolve to `turns`
- strategy mode without either threshold flag exits 2 and prints
  `summary strategy thresholds are required`
- invalid token value exits 2 before `buildRuntime` is called
- standard mode with no thresholds still succeeds

- [ ] **Step 2: Run CLI tests and verify RED**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./cmd/eval-runner -run 'TestRunWithDeps.*Summary|TestRunRejectsInvalidSummaryTokenThreshold' -count=1
```

Expected: build failure because the token flag and `thresholdUnit` do not exist.

- [ ] **Step 3: Extend CLI options**

Change:

```go
type summaryEvalOptions struct {
	mode                  rageval.SummaryEvalMode
	thresholdUnit         rageval.SummaryStrategyThresholdUnit
	thresholds            []int
	messageOverheadTokens int
}
```

Add the flag:

```go
summaryTokenThresholds := fs.String(
	"summary-token-thresholds",
	"",
	"comma-separated summary strategy token thresholds, e.g. 800,1200,1600",
)
```

Rename the parser to a reusable positive-integer parser:

```go
func parsePositiveIntList(raw string, label string) ([]int, error)
```

Resolve options:

```go
func resolveSummaryEvalOptions(
	mode rageval.SummaryEvalMode,
	turnThresholds []int,
	tokenThresholds []int,
) (summaryEvalOptions, error)
```

Resolution:

```go
if mode != rageval.SummaryEvalModeStrategy {
	return summaryEvalOptions{mode: mode}, nil
}
if len(tokenThresholds) > 0 {
	return summaryEvalOptions{
		mode:          mode,
		thresholdUnit: rageval.SummaryStrategyThresholdTokens,
		thresholds:    tokenThresholds,
	}, nil
}
if len(turnThresholds) > 0 {
	return summaryEvalOptions{
		mode:          mode,
		thresholdUnit: rageval.SummaryStrategyThresholdTurns,
		thresholds:    turnThresholds,
	}, nil
}
return summaryEvalOptions{}, fmt.Errorf("summary strategy thresholds are required")
```

- [ ] **Step 4: Wire configured message overhead**

In `buildPhase1Registry`, use a fallback of 4:

```go
messageOverheadTokens := 4
if cfg := config.Get(); cfg != nil {
	messageOverheadTokens = cfg.Rag.Memory.SummaryToken.MessageOverheadTokens
}
```

Build:

```go
SummaryOptions: rageval.SummaryEvaluatorRuntimeOptions{
	Mode:                  summaryOpts.mode,
	ThresholdUnit:         summaryOpts.thresholdUnit,
	Thresholds:            append([]int(nil), summaryOpts.thresholds...),
	MessageOverheadTokens: messageOverheadTokens,
},
```

Do not make CLI users specify overhead in this phase.

- [ ] **Step 5: Run CLI and registry tests**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./cmd/eval-runner ./internal/app/rag/evaluation -count=1
```

Expected: PASS.

- [ ] **Step 6: Stage and optionally commit Task 5**

Run:

```powershell
git add -p -- cmd/eval-runner/main.go cmd/eval-runner/main_test.go
git diff --cached --check
```

If isolated:

```powershell
git commit -m "feat: add summary token threshold CLI"
```

## Task 6: Verify the Formal Entry Point and Run the Candidate Sweep

**Files:**

- Modify: `openspec/changes/add-token-aware-summary-compression/tasks.md`
- Generate: `tmp/summary_strategy_token_thresholds_20260623.json`

- [ ] **Step 1: Run focused automated verification**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./cmd/eval-runner ./internal/app/rag/evaluation -count=1
```

Expected: PASS.

- [ ] **Step 2: Run broader token-aware summary regression tests**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/tokenbudget ./internal/app/rag/core/history ./internal/app/rag/evaluation ./internal/bootstrap/rag -count=1
```

Expected: PASS.

- [ ] **Step 3: Validate the OpenSpec change**

Run:

```powershell
openspec validate add-token-aware-summary-compression --strict
```

Expected:

```text
Change 'add-token-aware-summary-compression' is valid
```

- [ ] **Step 4: Execute the real token-threshold sweep**

Run from the repository root:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go run ./cmd/eval-runner `
  -suite summary `
  -input testdata/evals/summary/strategy_samples.json `
  -summary-mode strategy `
  -summary-token-thresholds 800,1200,1600 `
  -output tmp/summary_strategy_token_thresholds_20260623.json
```

Expected: exit code 0 and a JSON result file containing threshold rows for 800,
1200, and 1600 with `threshold_unit: "tokens"`.

This command may require network access and configured model credentials. If it
cannot run because the external model service is unavailable, leave the
OpenSpec rollout checkbox unchecked and record the exact environmental blocker.

- [ ] **Step 5: Inspect the sweep result**

Run:

```powershell
$result = Get-Content -Raw 'tmp\summary_strategy_token_thresholds_20260623.json' | ConvertFrom-Json
$result.aggregate.metrics.threshold_aggregates |
  Select-Object threshold, threshold_unit, avg_token_saved_ratio, avg_structured_fidelity, avg_structured_usefulness, avg_downstream_equivalence, avg_summary_call_count, pass_rate |
  Format-Table
```

Expected:

- three rows
- unit `tokens` on every row
- no missing quality or token-saving fields

- [ ] **Step 6: Update OpenSpec only when evidence exists**

If Step 4 succeeds, change:

```markdown
- [ ] 将 summary strategy eval 的候选阈值从轮数口径升级为 token budget 口径并执行比较。
```

to:

```markdown
- [x] 将 summary strategy eval 的候选阈值从轮数口径升级为 token budget 口径并执行比较。
```

Add a verification note containing:

- command date
- thresholds `800/1200/1600`
- output artifact path
- a concise comparison of token savings, fidelity, equivalence, and call count

If the sweep is externally blocked, keep `[ ]` and add the blocker instead.

- [ ] **Step 7: Run final verification**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./cmd/eval-runner ./internal/app/rag/evaluation ./internal/app/rag/core/tokenbudget ./internal/app/rag/core/history ./internal/bootstrap/rag -count=1
openspec validate add-token-aware-summary-compression --strict
git diff --check
```

Expected:

- all focused tests pass
- OpenSpec strict validation passes
- `git diff --check` reports no errors

- [ ] **Step 8: Review repository-wide known blockers without changing them**

Run:

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./... -count=1
```

Record failures separately. Do not fix unrelated root temporary programs,
multiple `scripts` package `main` declarations, or the pre-existing config
default-model assertion as part of this feature.

- [ ] **Step 9: Prepare a selective implementation commit**

Because the target files already overlap other uncommitted work, inspect every
staged hunk:

```powershell
git status --short
git diff --cached
git diff --cached --check
```

Commit only if the staged diff contains the token-threshold evaluation work and
its OpenSpec evidence:

```powershell
git commit -m "feat: evaluate summary compression by token threshold"
```

If clean selective staging is not possible, leave the implementation
uncommitted and report the exact overlapping files rather than including
unrelated user changes.
