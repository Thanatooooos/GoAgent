# Summary Eval Repair Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Raise the `summary` suite pass count from `2/12` to at least `6/12` by improving summary state classification, conservative field repair, and evaluator-facing diagnostics without weakening critical gates.

**Architecture:** Keep the repair bounded to the existing structured-summary path. Tighten the summary prompt in `internal/app/rag/core/history`, add a conservative `StructuredSummary` repair pass before validation, and keep evaluator semantics intact while making diagnostics easier to interpret through focused tests and artifact review.

**Tech Stack:** Go, `go test`, `cmd/eval-runner`, structured JSON summary generation, existing `history` and `evaluation` packages.

---

## File Structure

### Existing files to modify

- `internal/app/rag/core/history/summary_compression.go`
  - Tighten the structured-summary prompt contract and keep the generation path calling the new repair hook before validation.
- `internal/app/rag/core/history/summary_generation.go`
  - Ensure the direct generation helper uses the same repair + validation flow as runtime compression.
- `internal/app/rag/core/history/summary_validator.go`
  - Reuse current validation after the repair pass; keep validation strict and non-generative.
- `internal/app/rag/core/history/summary_compression_test.go`
  - Lock prompt text changes with focused assertions.
- `internal/app/rag/core/history/summary_validator_test.go`
  - Add repair + validation focused tests for unresolved-item demotion and boundary preservation.
- `internal/app/rag/evaluation/summary_evaluator.go`
  - Preserve hard gates while optionally separating critical vs diagnostic failure output in a clearer way if the implementation chooses to expose that.
- `internal/app/rag/evaluation/summary_rules_test.go`
  - Lock any evaluator-facing diagnostic formatting or non-gating behavior.

### New files to create

- `internal/app/rag/core/history/summary_repair.go`
  - Implement conservative post-processing for schema completion, fact demotion, constraint promotion, recent-progress backfill, and open-question backfill.
- `internal/app/rag/core/history/summary_repair_test.go`
  - Unit tests for each repair behavior.

### Existing files to use for regression checks

- `testdata/evals/summary/samples.json`
- `testdata/evals/summary/latest_run_20260621.json`

---

### Task 1: Tighten the Summary Prompt Contract

**Files:**
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Test: `internal/app/rag/core/history/summary_compression_test.go`

- [ ] **Step 1: Write the failing prompt-contract test**

Add assertions to `internal/app/rag/core/history/summary_compression_test.go` so the prompt must mention active-scope classification, unresolved-item routing, and current-boundary preservation.

```go
func TestBuildStructuredSummaryPromptIncludesRepairOrientedRules(t *testing.T) {
	tier := SummaryBudgetTier{MaxChars: 320}
	latest := domain.ConversationSummary{
		StructuredSummaryJSON: `{"schema_version":1,"goal":"先完成 spec"}`,
	}
	historyMessages := []domain.ConversationMessage{
		{Role: "user", Content: "当前先不要进入实现。"},
		{Role: "assistant", Content: "先完成 spec、design、tasks。"},
		{Role: "user", Content: "根因还没确认，先不要下结论。"},
	}

	prompt := buildStructuredSummaryPrompt(tier, latest, historyMessages)

	requiredPhrases := []string{
		"只保留当前仍然有效的目标和约束",
		"当前不做什么也属于 constraints",
		"未确认、待验证、候选信息放进 open_questions",
		"不要把猜测写成 established_facts",
		"最近刚确认或刚变化的状态优先写入 recent_progress",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q, got:\\n%s", phrase, prompt)
		}
	}
}
```

- [ ] **Step 2: Run the prompt test and verify it fails**

Run:

```powershell
go test ./internal/app/rag/core/history -run TestBuildStructuredSummaryPromptIncludesRepairOrientedRules -count=1
```

Expected: FAIL because the current prompt text does not contain the new repair-oriented rules.

- [ ] **Step 3: Update the structured summary prompt text**

In `internal/app/rag/core/history/summary_compression.go`, extend `structuredSummarySystemPrompt` with repair-oriented instructions. Keep the change local to prompt text.

```go
const structuredSummarySystemPrompt = `你正在将一段对话压缩为结构化工作记忆。只返回 JSON。

JSON 类型约定：
- schema_version: 整数 (number)，固定为 1
- goal: 字符串
- user_preferences: 字符串数组，无该项时返回 []
- constraints: 字符串数组，无该项时返回 []
- established_facts: 字符串数组，无该项时返回 []
- recent_progress: 字符串数组，无该项时返回 []
- open_questions: 字符串数组，无该项时返回 []

在写入字段前，先判断每条信息：
1. 是已确认、未确认，还是已被新信息覆盖
2. 它应该归属哪个字段
3. 如果丢失它，是否会影响下一步决策或下一轮回答

各字段内容指南：
- goal：只保留当前活跃目标，不要把旁支讨论、未来可能工作、已作废方向写成主目标。
- constraints：当前仍然有效的边界条件。当前不做什么、当前不进入哪个阶段，也属于 constraints。
- established_facts：只写已确认事实。不要把猜测、候选方案、待验证项写进 established_facts。
- recent_progress：优先写最近刚确认、刚变更、刚收紧的状态，例如优先级变化、范围收紧、方案切换、交付要求确认。
- open_questions：仍未确认但会影响下一步的问题。只要存在未确认、待验证、候选方向，优先放进 open_questions。

规则：
1. 不要编造事实。不确定的信息放进 open_questions。
2. 已被新信息覆盖/作废的旧事实不要保留。
3. 错误码、配置 key、版本号、具体决策必须逐字保留在摘要文本中。
4. 最终渲染预算约 %d 字符。
`
```

- [ ] **Step 4: Run the prompt tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestBuildStructuredSummaryPrompt" -count=1
```

Expected: PASS for both the existing prompt assertions and the new repair-oriented prompt test.

- [ ] **Step 5: Commit**

```powershell
git add internal/app/rag/core/history/summary_compression.go internal/app/rag/core/history/summary_compression_test.go
git commit -m "test: tighten summary prompt contract"
```

---

### Task 2: Add Conservative Structured Summary Repair Helpers

**Files:**
- Create: `internal/app/rag/core/history/summary_repair.go`
- Test: `internal/app/rag/core/history/summary_repair_test.go`

- [ ] **Step 1: Write failing repair tests**

Create `internal/app/rag/core/history/summary_repair_test.go` with focused tests for fact demotion, constraint promotion, recent-progress backfill, and open-question backfill.

```go
func TestRepairStructuredSummaryDemotesUnresolvedFacts(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "排查 rewrite 延迟",
		EstablishedFacts: []string{"怀疑是 prompt template 引入了额外占位符"},
		RecentProgress:   []string{"已观察到 TIMEOUT_AFTER_REWRITE"},
	}

	got := RepairStructuredSummary(input)

	if len(got.EstablishedFacts) != 0 {
		t.Fatalf("EstablishedFacts = %#v, want empty after demotion", got.EstablishedFacts)
	}
	if len(got.OpenQuestions) != 1 || got.OpenQuestions[0] != "怀疑是 prompt template 引入了额外占位符" {
		t.Fatalf("OpenQuestions = %#v, want demoted unresolved item", got.OpenQuestions)
	}
}

func TestRepairStructuredSummaryPromotesActiveBoundaryIntoConstraints(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:  1,
		Goal:           "先完成 summary 评测设计",
		RecentProgress: []string{"当前不进入实现"},
	}

	got := RepairStructuredSummary(input)

	if len(got.Constraints) != 1 || got.Constraints[0] != "当前不进入实现" {
		t.Fatalf("Constraints = %#v, want promoted active boundary", got.Constraints)
	}
}

func TestRepairStructuredSummaryBackfillsRecentProgress(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "收敛评测阶段范围",
		EstablishedFacts: []string{"优先级已提升为 P1"},
	}

	got := RepairStructuredSummary(input)

	if len(got.RecentProgress) == 0 {
		t.Fatalf("RecentProgress = %#v, want backfilled progress", got.RecentProgress)
	}
}
```

- [ ] **Step 2: Run the repair tests and verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestRepairStructuredSummary" -count=1
```

Expected: FAIL because `RepairStructuredSummary` does not exist yet.

- [ ] **Step 3: Implement the repair helper**

Create `internal/app/rag/core/history/summary_repair.go` with one public repair entrypoint and small internal helpers. Keep the repair conservative and non-generative.

```go
package history

import "strings"

func RepairStructuredSummary(summary StructuredSummary) StructuredSummary {
	summary.Normalize()
	summary = ensureSummarySchemaDefaults(summary)
	summary = demoteUnresolvedEstablishedFacts(summary)
	summary = promoteActiveBoundaries(summary)
	summary = backfillRecentProgress(summary)
	summary = backfillOpenQuestions(summary)
	summary.Normalize()
	return summary
}

func ensureSummarySchemaDefaults(summary StructuredSummary) StructuredSummary {
	if summary.SchemaVersion <= 0 {
		summary.SchemaVersion = 1
	}
	if summary.UserPreferences == nil {
		summary.UserPreferences = []string{}
	}
	if summary.Constraints == nil {
		summary.Constraints = []string{}
	}
	if summary.EstablishedFacts == nil {
		summary.EstablishedFacts = []string{}
	}
	if summary.RecentProgress == nil {
		summary.RecentProgress = []string{}
	}
	if summary.OpenQuestions == nil {
		summary.OpenQuestions = []string{}
	}
	return summary
}

func demoteUnresolvedEstablishedFacts(summary StructuredSummary) StructuredSummary {
	keptFacts := make([]string, 0, len(summary.EstablishedFacts))
	for _, item := range summary.EstablishedFacts {
		if containsAnySummaryMarker(item, "可能", "怀疑", "候选", "待确认", "待验证", "待评审", "尚未确认") {
			summary.OpenQuestions = append(summary.OpenQuestions, item)
			continue
		}
		keptFacts = append(keptFacts, item)
	}
	summary.EstablishedFacts = dedupeSummaryItems(keptFacts)
	summary.OpenQuestions = dedupeSummaryItems(summary.OpenQuestions)
	return summary
}

func promoteActiveBoundaries(summary StructuredSummary) StructuredSummary {
	for _, item := range append([]string{}, summary.Goal) {
		if containsAnySummaryMarker(item, "不进入实现", "先不做", "仅做 summary", "仅做 rewrite", "本周必须") {
			summary.Constraints = append(summary.Constraints, item)
		}
	}
	for _, item := range summary.RecentProgress {
		if containsAnySummaryMarker(item, "不进入实现", "先不做", "本周必须", "Phase 1") {
			summary.Constraints = append(summary.Constraints, item)
		}
	}
	summary.Constraints = dedupeSummaryItems(summary.Constraints)
	return summary
}
```

- [ ] **Step 4: Run the repair tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestRepairStructuredSummary" -count=1
```

Expected: PASS for the new repair-helper tests.

- [ ] **Step 5: Commit**

```powershell
git add internal/app/rag/core/history/summary_repair.go internal/app/rag/core/history/summary_repair_test.go
git commit -m "feat: add conservative summary repair helpers"
```

---

### Task 3: Apply Repair Before Validation in Both Summary Generation Paths

**Files:**
- Modify: `internal/app/rag/core/history/summary_generation.go`
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Modify: `internal/app/rag/core/history/summary_validator_test.go`
- Test: `internal/app/rag/core/history/service_store_test.go`

- [ ] **Step 1: Write failing integration tests for repair-before-validation**

Add tests that prove unresolved facts are repaired before validation and that repaired summaries still preserve critical entities.

```go
func TestGenerateStructuredSummaryRepairsBeforeValidation(t *testing.T) {
	chat := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"排查 rewrite 延迟","established_facts":["怀疑是 prompt template 引入额外占位符"],"recent_progress":["已观察到 TIMEOUT_AFTER_REWRITE"]}`,
	}

	output, err := GenerateStructuredSummary(context.Background(), chat, GenerateStructuredSummaryInput{
		SourceMessages: []domain.ConversationMessage{
			{Role: "user", Content: "先不要下结论，根因还没确认。"},
			{Role: "assistant", Content: "目前只观察到 TIMEOUT_AFTER_REWRITE。"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateStructuredSummary() error = %v", err)
	}

	if len(output.Structured.EstablishedFacts) != 0 {
		t.Fatalf("EstablishedFacts = %#v, want unresolved fact removed", output.Structured.EstablishedFacts)
	}
	if len(output.Structured.OpenQuestions) == 0 {
		t.Fatalf("OpenQuestions = %#v, want repaired unresolved item", output.Structured.OpenQuestions)
	}
	if !output.Validation.Accepted {
		t.Fatalf("Validation = %+v, want accepted after repair", output.Validation)
	}
}
```

- [ ] **Step 2: Run the integration tests and verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestGenerateStructuredSummaryRepairsBeforeValidation|TestValidateStructuredSummary" -count=1
```

Expected: FAIL because the current generation path validates the raw parsed summary without a repair pass.

- [ ] **Step 3: Wire the repair helper into both generation flows**

Update both `GenerateStructuredSummary` and `runConversationSummaryCompression` so they repair the parsed summary before validation and rendering.

```go
structured, err := ParseStructuredSummary(strings.TrimSpace(response))
if err != nil {
	return GenerateStructuredSummaryOutput{}, fmt.Errorf("parse structured summary: %w", err)
}

structured = RepairStructuredSummary(structured)
validation := ValidateStructuredSummary(structured, input.SourceMessages)

return GenerateStructuredSummaryOutput{
	Structured: structured,
	Rendered:   RenderStructuredSummary(structured, tier.MaxChars),
	Raw:        strings.TrimSpace(response),
	Validation: validation,
}, nil
```

And in `summary_compression.go`:

```go
structured, err := ParseStructuredSummary(strings.TrimSpace(response))
if err != nil {
	return fmt.Errorf("parse structured summary: %w", err)
}

structured = RepairStructuredSummary(structured)
validation := ValidateStructuredSummary(structured, historyMessages)
if !validation.Accepted {
	return nil
}
```

- [ ] **Step 4: Run the history package tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history/... -count=1
```

Expected: PASS, including existing store and compression tests plus the new repair-before-validation coverage.

- [ ] **Step 5: Commit**

```powershell
git add internal/app/rag/core/history/summary_generation.go internal/app/rag/core/history/summary_compression.go internal/app/rag/core/history/summary_validator_test.go internal/app/rag/core/history/service_store_test.go
git commit -m "feat: repair structured summaries before validation"
```

---

### Task 4: Keep Evaluator Gates Stable but Clarify Diagnostic Output

**Files:**
- Modify: `internal/app/rag/evaluation/summary_evaluator.go`
- Modify: `internal/app/rag/evaluation/summary_artifacts.go`
- Test: `internal/app/rag/evaluation/summary_rules_test.go`
- Test: `internal/app/rag/evaluation/summary_evaluator_test.go`

- [ ] **Step 1: Write failing evaluator diagnostics tests**

Add tests that verify critical failures remain unchanged while diagnostic-only reasons are surfaced separately in artifacts or sample output metadata.

```go
func TestSummaryEvaluatorSeparatesCriticalAndDiagnosticFailureReasons(t *testing.T) {
	generator := &stubSummaryGenerator{
		output: SummaryGenerationOutput{
			Structured: raghistory.StructuredSummary{
				SchemaVersion:    1,
				Goal:             "stabilize summary evaluation",
				Constraints:      []string{"keep schema stable"},
				EstablishedFacts: []string{"judge already returns field-level diagnostics"},
				RecentProgress:   []string{"open_questions still missing"},
			},
		},
	}
	evaluator := NewSummaryEvaluator(generator)

	result, err := evaluator.Run(context.Background(), RunInput{
		InputPath:  "testdata/evals/summary/samples.json",
		RawSamples: json.RawMessage(summaryMissingOpenQuestionsFixture),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	sample := result.Samples[0]
	if len(sample.CriticalFailures) != 0 {
		t.Fatalf("CriticalFailures = %#v, want none for diagnostic-only miss", sample.CriticalFailures)
	}
	if len(sample.FailureReasons) != 0 {
		t.Fatalf("FailureReasons = %#v, want none when only diagnostic checks fail", sample.FailureReasons)
	}
}
```

- [ ] **Step 2: Run the evaluator tests and verify they fail if output shape changes are needed**

Run:

```powershell
go test ./internal/app/rag/evaluation -run "TestSummaryEvaluator|TestEvaluateSummaryRules" -count=1
```

Expected: FAIL only if the chosen output-shape clarification has not been implemented yet. If the current code already behaves as desired, keep this task to artifact clarity only and let the test pass after the artifact assertion is added.

- [ ] **Step 3: Implement artifact-side diagnostic clarity without weakening gates**

If needed, add a separate artifact field for diagnostic-only rule misses while preserving `CriticalFailures` and final `Passed` behavior.

```go
func buildSummarySampleArtifact(
	sample SummarySample,
	generated SummaryGenerationOutput,
	rules SummaryRuleEvaluation,
	fieldJudge *SummaryFieldJudgeEvaluation,
	equivalence *SummaryEquivalenceEvaluation,
) map[string]any {
	artifact := map[string]any{
		"generated_summary":       generated.Structured,
		"rendered_summary":        generated.Rendered,
		"raw_summary":             generated.Raw,
		"rule_evaluation":         rules,
		"diagnostic_rule_reasons": append([]string(nil), rules.FailureReasons...),
		"source_message_count":    len(sample.Input.SourceMessages),
	}
	// existing previous_summary / judge additions stay unchanged
	return artifact
}
```

- [ ] **Step 4: Run evaluator tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/evaluation -run "TestSummaryEvaluator|TestEvaluateSummaryRules" -count=1
```

Expected: PASS with no behavior change to critical gates.

- [ ] **Step 5: Commit**

```powershell
git add internal/app/rag/evaluation/summary_evaluator.go internal/app/rag/evaluation/summary_artifacts.go internal/app/rag/evaluation/summary_rules_test.go internal/app/rag/evaluation/summary_evaluator_test.go
git commit -m "test: clarify summary evaluator diagnostics"
```

---

### Task 5: Regress the Priority Samples, Then Run the Full Summary Suite

**Files:**
- Modify: `testdata/evals/summary/latest_run_20260621.json` (do not edit; use as baseline reference only)
- Produce: `testdata/evals/summary/latest_run_repair_check.json` (generated output artifact)

- [ ] **Step 1: Run focused package tests before the suite**

Run:

```powershell
go test ./internal/app/rag/core/history/... -count=1
go test ./internal/app/rag/evaluation/... -run Summary -count=1
```

Expected: PASS. Do not proceed to the full suite if either command fails.

- [ ] **Step 2: Run the full summary suite and capture output**

Run:

```powershell
$env:GOCACHE=(Resolve-Path '.').Path + '\\.gocache-summary-repair'
$env:GOTMPDIR=(Resolve-Path '.').Path + '\\.tmp-go-summary-repair'
New-Item -ItemType Directory -Force $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
go run ./cmd/eval-runner -suite summary -input testdata/evals/summary/samples.json | Tee-Object -FilePath testdata/evals/summary/latest_run_repair_check.json
```

Expected: command exits `0` and emits one JSON suite result object.

- [ ] **Step 3: Verify the targeted samples improved**

Inspect these samples in the JSON result:

- `state_override_phase_scope`
- `critical_entity_component_version`
- `goal_drift_multi_topic_discussion`
- `fact_vs_open_question_uncertain_root_cause`
- `state_override_priority_flip`
- `long_dialogue_dense_constraint_stack`

Use:

```powershell
$data = Get-Content -Raw 'testdata/evals/summary/latest_run_repair_check.json' | ConvertFrom-Json
$data.samples | Where-Object { $_.name -in @(
  'state_override_phase_scope',
  'critical_entity_component_version',
  'goal_drift_multi_topic_discussion',
  'fact_vs_open_question_uncertain_root_cause',
  'state_override_priority_flip',
  'long_dialogue_dense_constraint_stack'
) } | Select-Object name, passed, failure_reasons
```

Expected: at least four of the six priority samples now pass, or show strictly narrower failure reasons than the baseline run.

- [ ] **Step 4: Verify the suite-level goal**

Run:

```powershell
$data = Get-Content -Raw 'testdata/evals/summary/latest_run_repair_check.json' | ConvertFrom-Json
$passed = @($data.samples | Where-Object { $_.passed }).Count
$danger = @($data.samples | Where-Object { $_.critical_failures -match 'dangerous_downstream_drift' }).Count
\"passed=$passed\"
\"dangerous_drift_samples=$danger\"
```

Expected:

- `passed` is at least `6`
- `dangerous_drift_samples` does not increase above the baseline count of `3`

- [ ] **Step 5: Commit**

```powershell
git add internal/app/rag/core/history internal/app/rag/evaluation
git commit -m "feat: improve summary eval pass rate with conservative repair"
```

---

## Self-Review

### Spec coverage

- Prompt-side state classification is covered by Task 1.
- Conservative field repair is covered by Task 2.
- Repair-before-validation integration is covered by Task 3.
- Evaluator diagnostics clarity without weakening gates is covered by Task 4.
- Full regression against the `6/12` target is covered by Task 5.

### Placeholder scan

- No `TODO`, `TBD`, or deferred placeholders remain.
- Every task includes exact file paths, commands, and code snippets.

### Type consistency

- `RepairStructuredSummary` is introduced in Task 2 and reused consistently in Tasks 3 and 5.
- `GenerateStructuredSummaryOutput`, `StructuredSummary`, `SummaryRuleEvaluation`, and `buildSummarySampleArtifact` match existing code identifiers in the repository.
