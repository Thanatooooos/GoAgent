# Summary Strategy Evaluation Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing `summary` offline evaluator with a formal `strategy` mode that replays long multi-turn samples under a sweep of global compression thresholds, then compares token savings, structured summary quality, and downstream answer carry-over quality.

**Architecture:** Keep the current `summary` evaluator as the single suite entrypoint, then add a mode switch inside evaluator execution rather than creating a separate evaluator. Strategy mode will reuse the real incremental summary generation path, run checkpoint-based evaluation contracts from an optional `strategy_eval` sample block, estimate token costs with the existing rough token estimator, and emit threshold-level sample + aggregate results including Pareto candidates.

**Tech Stack:** Go, `cmd/eval-runner`, existing `internal/app/rag/evaluation` summary evaluator/judge/artifact code, existing `internal/app/rag/core/history` structured summary generation, existing `sessionrecall.RoughTokenEstimator`, `go test`

---

## File Map

- Modify: `cmd/eval-runner/main.go`
  - add `-summary-mode` and `-summary-thresholds` flags and thread summary strategy options into registry construction
- Modify: `cmd/eval-runner/main_test.go`
  - cover new summary CLI flags and registry wiring expectations
- Modify: `internal/app/rag/evaluation/phase1_registry.go`
  - pass summary evaluator options through the phase-1 registry
- Modify: `internal/app/rag/evaluation/summary_evaluator.go`
  - branch between existing `standard` flow and new `strategy` flow
- Modify: `internal/app/rag/evaluation/summary_sample.go`
  - add optional `strategy_eval` sample contract and validation helpers
- Modify: `internal/app/rag/evaluation/summary_artifacts.go`
  - emit threshold-level strategy artifacts
- Create: `internal/app/rag/evaluation/summary_strategy.go`
  - repeated-compression replay engine, checkpoint execution, threshold aggregation
- Create: `internal/app/rag/evaluation/summary_strategy_test.go`
  - focused replay / checkpoint / aggregate / Pareto tests
- Create: `internal/app/rag/evaluation/summary_strategy_tokens.go`
  - token accounting helpers built on the existing rough estimator
- Create: `internal/app/rag/evaluation/summary_strategy_tokens_test.go`
  - focused token baseline-vs-strategy tests
- Create: `testdata/evals/summary/strategy_samples.json`
  - initial dense long-dialog samples for strategy mode
- Modify: `testdata/evals/summary/README.md`
  - document `strategy_eval`, checkpoints, and threshold-sweep authoring rules

### Task 1: Extend summary sample and evaluator option contracts

**Files:**
- Modify: `internal/app/rag/evaluation/summary_sample.go`
- Modify: `internal/app/rag/evaluation/summary_evaluator.go`
- Modify: `internal/app/rag/evaluation/phase1_registry.go`
- Test: `internal/app/rag/evaluation/summary_sample_test.go`
- Test: `internal/app/rag/evaluation/summary_evaluator_test.go`
- Test: `internal/app/rag/evaluation/phase1_registry_test.go`

- [ ] **Step 1: Write the failing sample-contract tests for `strategy_eval` parsing and validation**

```go
func TestParseSummarySamplesSupportsStrategyEval(t *testing.T) {
	rawSamples, err := ExtractSampleArray(json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"},
				{"role":"user","content":"Q2"},
				{"role":"assistant","content":"A2"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":1,
					"expected_summary":{"goal":{"must_cover":["????"]}},
					"critical_contract":{},
					"next_turn_eval":{"queries":[{"id":"q1","query":"???????","equivalence_expectations":["????????"]}]}
				}]
			}
		}
	]`))
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	samples, err := ParseSummarySamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseSummarySamples() error = %v", err)
	}
	if len(samples) != 1 || len(samples[0].StrategyEval.Checkpoints) != 1 {
		t.Fatalf("unexpected checkpoints: %#v", samples)
	}
	if samples[0].StrategyEval.Checkpoints[0].AfterTurn != 1 {
		t.Fatalf("AfterTurn = %d, want 1", samples[0].StrategyEval.Checkpoints[0].AfterTurn)
	}
}

func TestParseSummarySamplesRejectsCheckpointPastConversationLength(t *testing.T) {
	rawSamples, err := ExtractSampleArray(json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":2,
					"expected_summary":{"goal":{"must_cover":["????"]}},
					"critical_contract":{},
					"next_turn_eval":{}
				}]
			}
		}
	]`))
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}

	if _, err := ParseSummarySamples(rawSamples); err == nil {
		t.Fatal("ParseSummarySamples() expected checkpoint validation error")
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./internal/app/rag/evaluation -run "TestParseSummarySamplesSupportsStrategyEval|TestParseSummarySamplesRejectsCheckpointPastConversationLength" -v`

Expected: FAIL with unknown `StrategyEval` fields or missing validation logic.

- [ ] **Step 3: Implement the minimal `strategy_eval` sample types and validation helpers**

```go
type SummaryStrategyCheckpoint struct {
	AfterTurn        int                     `json:"after_turn"`
	ExpectedSummary  SummaryExpectedSummary  `json:"expected_summary"`
	CriticalContract SummaryCriticalContract `json:"critical_contract"`
	NextTurnEval     SummaryNextTurnEval     `json:"next_turn_eval"`
}

type SummaryStrategyEval struct {
	Checkpoints []SummaryStrategyCheckpoint `json:"checkpoints,omitempty"`
	FinalEval   *SummaryStrategyCheckpoint  `json:"final_eval,omitempty"`
}

type SummarySample struct {
	Name             string                  `json:"name"`
	Tags             []string                `json:"tags,omitempty"`
	Input            SummaryInput            `json:"input"`
	ExpectedSummary  SummaryExpectedSummary  `json:"expected_summary"`
	CriticalContract SummaryCriticalContract `json:"critical_contract"`
	NextTurnEval     SummaryNextTurnEval     `json:"next_turn_eval,omitempty"`
	StrategyEval     *SummaryStrategyEval    `json:"strategy_eval,omitempty"`
	Metadata         map[string]any          `json:"metadata,omitempty"`
}

func countSummaryTurns(messages []SummaryMessage) int {
	turns := 0
	for i := 0; i+1 < len(messages); i += 2 {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") && strings.EqualFold(strings.TrimSpace(messages[i+1].Role), "assistant") {
			turns++
		}
	}
	return turns
}
```

- [ ] **Step 4: Add failing evaluator-option tests for strategy-mode wiring**

```go
func TestPhase1RegistryPassesSummaryStrategyOptions(t *testing.T) {
	generator := &stubSummaryGenerator{}
	registry, err := NewPhase1RegistryForSuite(SuiteSummary, Phase1RegistryDependencies{
		SummaryGenerator: generator,
		SummaryOptions: SummaryEvaluatorRuntimeOptions{
			Mode:       SummaryEvalModeStrategy,
			Thresholds: []int{4, 8, 12},
		},
	})
	if err != nil {
		t.Fatalf("NewPhase1RegistryForSuite() error = %v", err)
	}
	evaluator, ok := registry.Get(SuiteSummary)
	if !ok {
		t.Fatal("summary evaluator expected registered")
	}
	typed, ok := evaluator.(*SummaryEvaluator)
	if !ok {
		t.Fatalf("unexpected evaluator type %T", evaluator)
	}
	if typed.runtime.Mode != SummaryEvalModeStrategy {
		t.Fatalf("Mode = %q, want %q", typed.runtime.Mode, SummaryEvalModeStrategy)
	}
}
```

- [ ] **Step 5: Run the focused evaluator-option tests to verify they fail**

Run: `go test ./internal/app/rag/evaluation -run "TestPhase1RegistryPassesSummaryStrategyOptions" -v`

Expected: FAIL because `SummaryOptions` or `runtime` configuration does not exist yet.

- [ ] **Step 6: Implement the minimal evaluator runtime option plumbing**

```go
type SummaryEvalMode string

const (
	SummaryEvalModeStandard SummaryEvalMode = "standard"
	SummaryEvalModeStrategy SummaryEvalMode = "strategy"
)

type SummaryEvaluatorRuntimeOptions struct {
	Mode       SummaryEvalMode
	Thresholds []int
}

type SummaryEvaluator struct {
	generator       SummaryGenerator
	judge           Judge
	answerGenerator SummaryAnswerGenerator
	runtime         SummaryEvaluatorRuntimeOptions
}

func WithSummaryRuntimeOptions(options SummaryEvaluatorRuntimeOptions) SummaryEvaluatorOption {
	return func(e *SummaryEvaluator) {
		e.runtime = options
	}
}
```

- [ ] **Step 7: Run the focused tests to verify the contract layer passes**

Run: `go test ./internal/app/rag/evaluation -run "TestParseSummarySamples|TestPhase1RegistryPassesSummaryStrategyOptions" -v`

Expected: PASS

### Task 2: Build the repeated-compression strategy engine and token accounting helpers

**Files:**
- Create: `internal/app/rag/evaluation/summary_strategy.go`
- Create: `internal/app/rag/evaluation/summary_strategy_tokens.go`
- Test: `internal/app/rag/evaluation/summary_strategy_test.go`
- Test: `internal/app/rag/evaluation/summary_strategy_tokens_test.go`
- Read only: `internal/app/rag/evaluation/summary_equivalence.go`
- Read only: `internal/app/rag/service/sessionrecall/tokens.go`

- [ ] **Step 1: Write the failing token-accounting tests**

```go
func TestEstimateStrategyTokenUsageCountsSummaryPlusTail(t *testing.T) {
	estimator := sessionrecall.RoughTokenEstimator{}
	usage := estimateStrategyTokenUsage(strategyTokenUsageInput{
		SummaryText: "?????????",
		TailMessages: []SummaryMessage{
			{Role: "user", Content: "?????? sweep"},
			{Role: "assistant", Content: "???? token ???"},
		},
		Estimator: estimator,
	})
	if usage.StrategyTokens <= 0 {
		t.Fatalf("StrategyTokens = %d, want > 0", usage.StrategyTokens)
	}
	if usage.BaselineTokens <= usage.StrategyTokens {
		t.Fatalf("BaselineTokens = %d, StrategyTokens = %d, want baseline > strategy", usage.BaselineTokens, usage.StrategyTokens)
	}
}
```

- [ ] **Step 2: Run the token tests to verify they fail**

Run: `go test ./internal/app/rag/evaluation -run "TestEstimateStrategyTokenUsageCountsSummaryPlusTail" -v`

Expected: FAIL with undefined token-accounting helpers.

- [ ] **Step 3: Write the failing replay and checkpoint tests**

```go
func TestRunSummaryStrategySweepEvaluatesConfiguredThresholds(t *testing.T) {
	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{SchemaVersion: 1, Goal: "??????? strategy mode"},
			Rendered:   "?????????? strategy mode",
		},
	}
	judge := &stubJudge{
		results: []JudgeResult{
			{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}},
			{Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}},
			{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}},
			{Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}},
		},
	}
	answerGen := &stubSummaryAnswerGenerator{outputs: []SummaryAnswerOutput{{Answer: "??? strategy mode"}, {Answer: "??? strategy mode"}, {Answer: "??? strategy mode"}, {Answer: "??? strategy mode"}}}
	sample := SummarySample{
		Name: "strategy-sample",
		Input: SummaryInput{SourceMessages: []SummaryMessage{{Role: "user", Content: "Q1"}, {Role: "assistant", Content: "A1"}, {Role: "user", Content: "Q2"}, {Role: "assistant", Content: "A2"}, {Role: "user", Content: "Q3"}, {Role: "assistant", Content: "A3"}, {Role: "user", Content: "Q4"}, {Role: "assistant", Content: "A4"}}},
		StrategyEval: &SummaryStrategyEval{Checkpoints: []SummaryStrategyCheckpoint{{AfterTurn: 2, ExpectedSummary: SummaryExpectedSummary{Goal: SummaryExpectedField{MustCover: []string{"?? strategy mode"}}}, NextTurnEval: SummaryNextTurnEval{Queries: []SummaryNextTurnQuery{{ID: "q1", Query: "???????", EquivalenceExpectations: []string{"???? strategy mode"}}}}}}},
	}

	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:       generator,
		Judge:           judge,
		AnswerGenerator: answerGen,
		Thresholds:      []int{2, 4},
	}, sample)
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	if len(result.ThresholdResults) != 2 {
		t.Fatalf("threshold results len = %d, want 2", len(result.ThresholdResults))
	}
}
```

- [ ] **Step 4: Run the replay tests to verify they fail**

Run: `go test ./internal/app/rag/evaluation -run "TestRunSummaryStrategySweepEvaluatesConfiguredThresholds" -v`

Expected: FAIL with undefined strategy engine types and functions.

- [ ] **Step 5: Implement the minimal token helpers and strategy replay engine**

```go
type strategyTokenUsageInput struct {
	SummaryText  string
	TailMessages []SummaryMessage
	Estimator    sessionrecall.TokenEstimator
}

type strategyTokenUsage struct {
	BaselineTokens int
	StrategyTokens int
}

type SummaryStrategyCheckpointResult struct {
	AfterTurn                int                         `json:"after_turn"`
	RuleEvaluation           SummaryRuleEvaluation       `json:"rule_evaluation"`
	FieldJudge               *SummaryFieldJudgeEvaluation `json:"field_judge,omitempty"`
	DownstreamEquivalence    *SummaryEquivalenceEvaluation `json:"downstream_equivalence,omitempty"`
	TokenBaseline            int                         `json:"token_baseline"`
	TokenStrategy            int                         `json:"token_strategy"`
	TokenSaved               int                         `json:"token_saved"`
	TokenSavedRatio          float64                     `json:"token_saved_ratio"`
}

type SummaryStrategyThresholdResult struct {
	Threshold                 int                              `json:"threshold"`
	SummaryCallCount          int                              `json:"summary_call_count"`
	CheckpointResults         []SummaryStrategyCheckpointResult `json:"checkpoint_results,omitempty"`
	FinalResult               *SummaryStrategyCheckpointResult  `json:"final_result,omitempty"`
	TokenBaseline             int                              `json:"token_baseline"`
	TokenStrategy             int                              `json:"token_strategy"`
	TokenSaved                int                              `json:"token_saved"`
	TokenSavedRatio           float64                          `json:"token_saved_ratio"`
	SummaryFidelityScore      float64                          `json:"summary_fidelity_score"`
	SummaryUsefulnessScore    float64                          `json:"summary_usefulness_score"`
	DownstreamEquivalenceScore float64                         `json:"downstream_equivalence_score"`
	CriticalFailureCount      int                              `json:"critical_failure_count"`
	DangerousDriftCount       int                              `json:"dangerous_drift_count"`
	Passed                    bool                             `json:"passed"`
}
```

- [ ] **Step 6: Run the focused strategy tests to verify the engine passes**

Run: `go test ./internal/app/rag/evaluation -run "TestEstimateStrategyTokenUsage|TestRunSummaryStrategySweep" -v`

Expected: PASS

### Task 3: Integrate strategy mode into the summary evaluator and artifacts

**Files:**
- Modify: `internal/app/rag/evaluation/summary_evaluator.go`
- Modify: `internal/app/rag/evaluation/summary_artifacts.go`
- Test: `internal/app/rag/evaluation/summary_evaluator_test.go`
- Test: `internal/app/rag/evaluation/summary_artifacts_test.go`

- [ ] **Step 1: Write the failing evaluator test for strategy mode execution**

```go
func TestSummaryEvaluatorRunStrategyMode(t *testing.T) {
	rawPayload := json.RawMessage(`[
		{
			"name":"strategy-sample",
			"input":{"source_messages":[
				{"role":"user","content":"Q1"},
				{"role":"assistant","content":"A1"},
				{"role":"user","content":"Q2"},
				{"role":"assistant","content":"A2"}
			]},
			"strategy_eval":{
				"checkpoints":[{
					"after_turn":1,
					"expected_summary":{"goal":{"must_cover":["strategy"]}},
					"critical_contract":{},
					"next_turn_eval":{"queries":[{"id":"q1","query":"???????","equivalence_expectations":["???? strategy"]}]}
				}]
			}
		}
	]`)
	rawSamples, err := ExtractSampleArray(rawPayload)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	evaluator := NewSummaryEvaluator(&stubSummaryGenerator{output: SummaryGenerationOutput{Structured: raghistory.StructuredSummary{SchemaVersion: 1, Goal: "strategy"}, Rendered: "???strategy"}}, WithSummaryJudge(&stubJudge{results: []JudgeResult{{Passed: true, Score: 1, Details: map[string]any{"fields": map[string]any{"goal": map[string]any{"fidelity": 1, "usefulness": 1}}}}, {Passed: true, Score: 1, Details: map[string]any{"dangerous_drift": false}}}), WithSummaryAnswerGenerator(&stubSummaryAnswerGenerator{outputs: []SummaryAnswerOutput{{Answer: "strategy"}, {Answer: "strategy"}}}), WithSummaryRuntimeOptions(SummaryEvaluatorRuntimeOptions{Mode: SummaryEvalModeStrategy, Thresholds: []int{1}}))

	result, err := evaluator.Run(context.Background(), RunInput{Suite: SuiteSummary, InputPath: "strategy.json", RawPayload: rawPayload, RawSamples: rawSamples})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	artifact := result.Artifacts["executions"].(map[string]any)["strategy-sample"].(map[string]any)
	if _, ok := artifact["threshold_results"]; !ok {
		t.Fatal("strategy artifact expected threshold_results")
	}
}
```

- [ ] **Step 2: Run the focused strategy-evaluator test to verify it fails**

Run: `go test ./internal/app/rag/evaluation -run "TestSummaryEvaluatorRunStrategyMode" -v`

Expected: FAIL because the evaluator still only produces the standard single-summary path.

- [ ] **Step 3: Update `SummaryEvaluator.Run` to branch on runtime mode**

```go
switch e.runtime.normalizedMode() {
case SummaryEvalModeStrategy:
	strategyResult, err := e.runStrategySample(ctx, sample)
	if err != nil {
		return SuiteResult{}, fmt.Errorf("run strategy sample %q: %w", sample.Name, err)
	}
	results = append(results, strategyResult.SharedSampleResult)
	artifacts[sample.Name] = strategyResult.Artifact
	accumulateStrategyTagStats(tagStats, sample.Tags, strategyResult.SharedSampleResult)
default:
	// keep existing standard-mode path unchanged
}
```

- [ ] **Step 4: Extend summary artifacts with threshold-level output**

```go
func buildSummaryStrategyArtifact(result SummaryStrategySampleResult) map[string]any {
	artifact := map[string]any{
		"mode":              "strategy",
		"threshold_results": result.ThresholdResults,
		"pareto_candidates": result.ParetoCandidates,
	}
	if len(result.DiagnosticNotes) > 0 {
		artifact["diagnostic_notes"] = append([]string(nil), result.DiagnosticNotes...)
	}
	return artifact
}
```

- [ ] **Step 5: Run the evaluator and artifact tests to verify strategy mode passes without breaking standard mode**

Run: `go test ./internal/app/rag/evaluation -run "TestSummaryEvaluator|TestSummaryEvaluatorRunStrategyMode|TestSummaryEvaluatorEmitsArtifactsForDebugging" -v`

Expected: PASS

### Task 4: Add runner flags and phase-1 registry wiring for strategy mode

**Files:**
- Modify: `cmd/eval-runner/main.go`
- Modify: `cmd/eval-runner/main_test.go`
- Modify: `internal/app/rag/evaluation/phase1_registry.go`
- Test: `internal/app/rag/evaluation/phase1_registry_test.go`

- [ ] **Step 1: Write the failing CLI parsing test for summary strategy flags**

```go
func TestRunWithDepsPassesSummaryStrategyFlags(t *testing.T) {
	var captured rageval.Phase1RegistryDependencies
	deps := evalRunnerDeps{
		buildRuntime: func(context.Context, string) (*ragbootstrap.Runtime, error) {
			return &ragbootstrap.Runtime{LLMChat: summaryOnlyLLMService{}}, nil
		},
		buildRegistry: func(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, _ []string) (*rageval.Registry, error) {
			captured = rageval.Phase1RegistryDependencies{SummaryOptions: rageval.SummaryEvaluatorRuntimeOptions{Mode: rageval.SummaryEvalModeStrategy, Thresholds: []int{4, 8, 12}}}
			return rageval.NewPhase1RegistryForSuite(suite, captured)
		},
	}

	code := runWithDeps([]string{"-suite", "summary", "-input", "testdata/evals/summary/samples.json", "-summary-mode", "strategy", "-summary-thresholds", "4,8,12"}, io.Discard, io.Discard, deps)
	if code != 0 {
		t.Fatalf("runWithDeps() code = %d, want 0", code)
	}
}
```

- [ ] **Step 2: Run the CLI parsing test to verify it fails**

Run: `go test ./cmd/eval-runner -run "TestRunWithDepsPassesSummaryStrategyFlags" -v`

Expected: FAIL because the new flags and summary runtime options are not parsed yet.

- [ ] **Step 3: Implement the minimal CLI and registry plumbing**

```go
summaryMode := fs.String("summary-mode", "standard", "summary evaluation mode: standard or strategy")
summaryThresholds := fs.String("summary-thresholds", "", "comma-separated summary strategy thresholds, e.g. 4,6,8,10")

type summaryEvalOptions struct {
	mode       rageval.SummaryEvalMode
	thresholds []int
}

registry, err := deps.buildRegistry(runtime, suite, evalKnowledgeBaseIDs, summaryEvalOptions{
	mode:       rageval.SummaryEvalMode(strings.TrimSpace(*summaryMode)),
	thresholds: parseSummaryThresholds(*summaryThresholds),
})
```

- [ ] **Step 4: Run the runner and registry tests to verify wiring passes**

Run: `go test ./cmd/eval-runner ./internal/app/rag/evaluation -run "TestRunWithDepsPassesSummaryStrategyFlags|TestPhase1Registry" -v`

Expected: PASS

### Task 5: Add strategy sample assets and end-to-end regression coverage

**Files:**
- Create: `testdata/evals/summary/strategy_samples.json`
- Modify: `testdata/evals/summary/README.md`
- Modify: `internal/app/rag/evaluation/summary_evaluator_test.go`
- Modify: `cmd/eval-runner/main_test.go`

- [ ] **Step 1: Author the first dense strategy sample file**

```json
[
  {
    "name": "strategy_scope_and_trigger_tradeoff",
    "tags": ["strategy", "long_dialog", "scope_shift"],
    "input": {
      "source_messages": [
        {"role": "user", "content": "???????????????"},
        {"role": "assistant", "content": "???? summary ? rewrite ? Phase 1 ?????"},
        {"role": "user", "content": "????? PostgreSQL ?????????"},
        {"role": "assistant", "content": "??????????????? spec?design?tasks?"},
        {"role": "user", "content": "???????? batch????????????"},
        {"role": "assistant", "content": "???? batch ????????????????????"},
        {"role": "user", "content": "??????????????????? token???????????"},
        {"role": "assistant", "content": "?????????????????? tradeoff?"}
      ]
    },
    "strategy_eval": {
      "checkpoints": [
        {
          "after_turn": 2,
          "expected_summary": {
            "goal": {"must_cover": ["???? spec?design?tasks??????"]},
            "constraints": {"must_cover": ["???? PostgreSQL ??", "?????????"]}
          },
          "critical_contract": {
            "critical_constraints": ["?????????"],
            "forbidden_claims": ["??????"]
          },
          "next_turn_eval": {
            "queries": [
              {
                "id": "q1",
                "query": "??????????????????",
                "equivalence_expectations": ["???????? spec?design?tasks", "??????????"]
              }
            ]
          }
        },
        {
          "after_turn": 4,
          "expected_summary": {
            "goal": {"must_cover": ["?????????????? tradeoff"]},
            "open_questions": {"must_cover": ["?????????"]}
          },
          "critical_contract": {
            "critical_facts": ["?????? batch"],
            "critical_open_questions": ["?????????"]
          },
          "next_turn_eval": {
            "queries": [
              {
                "id": "q2",
                "query": "????????????????",
                "equivalence_expectations": ["???? token ??", "?????????", "??????????"]
              }
            ]
          }
        }
      ]
    }
  }
]
```

- [ ] **Step 2: Document strategy authoring rules in the summary README**

```md
## Strategy Mode Samples

Use `strategy_eval` when the sample is intended to test repeated compression
policy instead of one-shot summary quality.

- keep `input.source_messages` as the full ordered dialogue
- define checkpoints with `after_turn`
- write one meaningful `next_turn_eval` query per checkpoint when possible
- prefer long, dense stateful dialogues over short conversations
```

- [ ] **Step 3: Add end-to-end evaluator coverage using the new strategy sample shape**

Run: `go test ./internal/app/rag/evaluation ./cmd/eval-runner -run "TestSummaryEvaluatorRunStrategyMode|TestRunWithDepsPassesSummaryStrategyFlags" -v`

Expected: PASS

- [ ] **Step 4: Run the full targeted regression set**

Run: `go test ./cmd/eval-runner ./internal/app/rag/evaluation/...`

Expected: PASS with summary strategy mode tests, existing standard summary tests, and existing rewrite tests all green.

## Spec Coverage Check

- Extends `summary` instead of creating a separate evaluator: covered by Tasks 1, 3, and 4.
- Adds a formal `strategy` mode with global threshold sweeps: covered by Tasks 1, 2, and 4.
- Reuses the real incremental structured summary path: covered by Task 2.
- Evaluates token savings, summary correctness, and downstream carry-over together: covered by Tasks 2 and 3.
- Adds checkpoint-based strategy sample authoring: covered by Tasks 1 and 5.
- Emits threshold-level sample artifacts and suite aggregates including Pareto candidates: covered by Tasks 2 and 3.
- Keeps Phase 1 scoped to global fixed thresholds only: reflected in Tasks 2, 3, and 4 with no segmented or adaptive policy work.
