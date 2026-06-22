# Summary Priority Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make summary-backed planning answers preserve the conversation's active workstream by introducing explicit priority hierarchy in structured summaries, rendering, repair, and regression evaluation.

**Architecture:** Extend the existing structured summary schema in `internal/app/rag/core/history` with explicit `active_priorities` and `background_issues`, teach prompt/repair/renderer logic to classify into those lanes, then regress the behavior with the concrete planning comparison question and the existing summary suite.

**Tech Stack:** Go, `go test`, `cmd/eval-runner`, structured JSON summary generation, existing `history` and `evaluation` packages, local experiment runner in `tmp/summary_compare_experiment.go`.

---

## File Structure

### Existing files to modify

- `internal/app/rag/core/history/summary_schema.go`
  - extend `StructuredSummary` with explicit hierarchy fields and keep parsing/normalization compatible
- `internal/app/rag/core/history/summary_renderer.go`
  - render hierarchy-first text order and new section labels
- `internal/app/rag/core/history/summary_compression.go`
  - update the structured summary prompt contract and schema description
- `internal/app/rag/core/history/summary_generation.go`
  - keep direct generation path aligned with the updated schema and rendering
- `internal/app/rag/core/history/summary_validator.go`
  - add deterministic checks for priority preservation and non-current-focus demotion
- `internal/app/rag/core/history/summary_repair.go`
  - add hierarchy-aware repair rules
- `internal/app/rag/core/history/summary_renderer_test.go`
  - lock render order and section omission behavior
- `internal/app/rag/core/history/summary_compression_test.go`
  - lock prompt contract updates
- `internal/app/rag/core/history/summary_validator_test.go`
  - cover priority-preservation rules
- `internal/app/rag/core/history/summary_repair_test.go`
  - cover lane promotion/demotion behavior
- `internal/app/rag/evaluation/summary_adapter_test.go`
  - keep generation adapter compatible with the new schema shape
- `tmp/summary_compare_experiment.go`
  - keep the local planning-comparison experiment using the approved harder question

### Existing files to inspect during implementation

- `internal/app/rag/core/history/summary_rules.go`
- `internal/app/rag/core/history/summary_policy.go`
- `testdata/evals/summary/samples.json`
- `tmp/summary_compare_experiment.md`

---

### Task 1: Extend the Structured Summary Schema

**Files:**
- Modify: `internal/app/rag/core/history/summary_schema.go`
- Test: `internal/app/rag/core/history/summary_schema_test.go`

- [ ] **Step 1: Write the failing schema test**

Add a test that parses and normalizes the new hierarchy fields.

```go
func TestParseStructuredSummarySupportsPriorityHierarchyFields(t *testing.T) {
	raw := `{
		"schema_version": 2,
		"goal": "收敛 summary 方案",
		"active_priorities": ["先完成 spec 和 tasks"],
		"background_issues": ["CI flaky 不是当前重点"]
	}`

	got, err := ParseStructuredSummary(raw)
	if err != nil {
		t.Fatalf("ParseStructuredSummary() error = %v", err)
	}

	if len(got.ActivePriorities) != 1 || got.ActivePriorities[0] != "先完成 spec 和 tasks" {
		t.Fatalf("ActivePriorities = %#v", got.ActivePriorities)
	}
	if len(got.BackgroundIssues) != 1 || got.BackgroundIssues[0] != "CI flaky 不是当前重点" {
		t.Fatalf("BackgroundIssues = %#v", got.BackgroundIssues)
	}
}
```

- [ ] **Step 2: Run the schema test and verify it fails**

Run:

```powershell
go test ./internal/app/rag/core/history -run TestParseStructuredSummarySupportsPriorityHierarchyFields -count=1
```

Expected: FAIL because the current schema does not expose the new fields.

- [ ] **Step 3: Implement the schema extension**

Update `StructuredSummary` and its normalization helpers in
`internal/app/rag/core/history/summary_schema.go`.

```go
type StructuredSummary struct {
	SchemaVersion    int      `json:"schema_version"`
	Goal             string   `json:"goal"`
	ActivePriorities []string `json:"active_priorities"`
	UserPreferences  []string `json:"user_preferences"`
	Constraints      []string `json:"constraints"`
	EstablishedFacts []string `json:"established_facts"`
	RecentProgress   []string `json:"recent_progress"`
	OpenQuestions    []string `json:"open_questions"`
	BackgroundIssues []string `json:"background_issues"`
}
```

Keep normalization behavior conservative:

- trim whitespace
- dedupe items per field
- initialize nil slices to `[]`

- [ ] **Step 4: Run schema tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestParseStructuredSummary|TestRepairStructuredSummary" -count=1
```

Expected: PASS for parsing and normalization coverage that touches the new schema shape.

---

### Task 2: Make the Prompt Emit Explicit Priority Lanes

**Files:**
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Test: `internal/app/rag/core/history/summary_compression_test.go`

- [ ] **Step 1: Write the failing prompt test**

Add a test that requires `active_priorities` and `background_issues` guidance in
the prompt.

```go
func TestBuildStructuredSummaryPromptIncludesPriorityHierarchyRules(t *testing.T) {
	tier := SummaryBudgetTier{MaxChars: 400}
	prompt := buildStructuredSummaryPrompt(tier, domain.ConversationSummary{}, []domain.ConversationMessage{
		{Role: "user", Content: "CI flaky 不是当前重点。"},
		{Role: "assistant", Content: "先完成 spec、design、tasks。"},
	})

	required := []string{
		"active_priorities",
		"background_issues",
		"如果信息被明确说明不是当前重点，不要写入 active_priorities",
		"active_priorities 按执行优先级排序",
	}
	for _, phrase := range required {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("expected prompt to contain %q, got:\\n%s", phrase, prompt)
		}
	}
}
```

- [ ] **Step 2: Run the prompt test and verify it fails**

Run:

```powershell
go test ./internal/app/rag/core/history -run TestBuildStructuredSummaryPromptIncludesPriorityHierarchyRules -count=1
```

Expected: FAIL because the current prompt does not describe the new fields or
classification rules.

- [ ] **Step 3: Update the structured summary prompt**

In `internal/app/rag/core/history/summary_compression.go`, extend the JSON
schema section and field rules.

```go
- active_priorities: 字符串数组，无该项时返回 []
- background_issues: 字符串数组，无该项时返回 []

- active_priorities：当前下一步应该优先推进的事项。只写当前范围内有效且应主导后续计划的问题，按优先级排序。最多 5 项。
- background_issues：对话中明确提到但不是当前重点的问题。它们需要保留，但不要写进 active_priorities。最多 5 项。

规则：
1. 如果对话明确说某事项“不是当前重点/只是背景问题/暂不处理”，不要写进 active_priorities。
2. 如果事项只是待确认方向，但并未被设为当前主线，放进 open_questions，不要抬升为 active_priorities。
3. 如果用户已经总结当前重点，优先保持该重点顺序。
```

- [ ] **Step 4: Run prompt tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestBuildStructuredSummaryPrompt" -count=1
```

Expected: PASS for both legacy and new prompt assertions.

---

### Task 3: Render Mainline Priorities Before Open Questions

**Files:**
- Modify: `internal/app/rag/core/history/summary_renderer.go`
- Test: `internal/app/rag/core/history/summary_renderer_test.go`

- [ ] **Step 1: Write the failing renderer test**

Add a test that locks the new render order.

```go
func TestRenderStructuredSummaryPlacesActivePrioritiesBeforeOpenQuestions(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    2,
		Goal:             "起草 summary 样本",
		ActivePriorities: []string{"先完成 spec、design、tasks"},
		OpenQuestions:    []string{"prompt template 是否引入额外占位符"},
		BackgroundIssues: []string{"CI flaky 不是当前重点"},
	}

	rendered := RenderStructuredSummary(summary, 0)

	activeIndex := strings.Index(rendered, "当前优先级：")
	openIndex := strings.Index(rendered, "待确认问题：")
	backgroundIndex := strings.Index(rendered, "背景问题：")
	if !(activeIndex >= 0 && openIndex > activeIndex && backgroundIndex > openIndex) {
		t.Fatalf("unexpected section order: %q", rendered)
	}
}
```

- [ ] **Step 2: Run the renderer test and verify it fails**

Run:

```powershell
go test ./internal/app/rag/core/history -run TestRenderStructuredSummaryPlacesActivePrioritiesBeforeOpenQuestions -count=1
```

Expected: FAIL because the current renderer does not include the new sections.

- [ ] **Step 3: Update the renderer**

In `internal/app/rag/core/history/summary_renderer.go`, add the new sections
and render them in this order:

```go
if len(summary.ActivePriorities) > 0 {
	sections = append(sections, "当前优先级：\n- "+strings.Join(summary.ActivePriorities, "\n- "))
}
...
if len(summary.BackgroundIssues) > 0 {
	sections = append(sections, "背景问题：\n- "+strings.Join(summary.BackgroundIssues, "\n- "))
}
```

Keep `open_questions` ahead of `background_issues`, but behind
`active_priorities`.

- [ ] **Step 4: Run renderer tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestRenderStructuredSummary" -count=1
```

Expected: PASS for section order and empty-section omission behavior.

---

### Task 4: Add Hierarchy-Aware Repair and Validation

**Files:**
- Modify: `internal/app/rag/core/history/summary_repair.go`
- Modify: `internal/app/rag/core/history/summary_validator.go`
- Test: `internal/app/rag/core/history/summary_repair_test.go`
- Test: `internal/app/rag/core/history/summary_validator_test.go`

- [ ] **Step 1: Write failing repair and validation tests**

Add tests for promotion, demotion, and active-priority preservation.

```go
func TestRepairStructuredSummaryDemotesNonCurrentFocusIntoBackgroundIssues(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    2,
		ActivePriorities: []string{"CI flaky 不是当前重点"},
	}

	got := RepairStructuredSummary(input)

	if len(got.ActivePriorities) != 0 {
		t.Fatalf("ActivePriorities = %#v, want empty", got.ActivePriorities)
	}
	if len(got.BackgroundIssues) != 1 || got.BackgroundIssues[0] != "CI flaky 不是当前重点" {
		t.Fatalf("BackgroundIssues = %#v", got.BackgroundIssues)
	}
}

func TestValidateStructuredSummaryRequiresActivePriorityWhenConversationSetsCurrentFocus(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion: 2,
		Goal:          "起草 summary 样本",
		OpenQuestions: []string{"ERR_POOL_TIMEOUT 根因是否已确认"},
	}

	source := []domain.ConversationMessage{
		{Role: "user", Content: "当前真正活跃的目标还是把 summary 样本起草出来，并把 must_cover、critical_contract 的边界写清楚。"},
	}

	result := ValidateStructuredSummary(summary, source)
	if result.Accepted {
		t.Fatalf("Validation.Accepted = true, want false when active priority is missing")
	}
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestRepairStructuredSummaryDemotesNonCurrentFocusIntoBackgroundIssues|TestValidateStructuredSummaryRequiresActivePriorityWhenConversationSetsCurrentFocus" -count=1
```

Expected: FAIL because current repair and validation do not understand these
lanes.

- [ ] **Step 3: Implement hierarchy-aware repair and validation**

In `summary_repair.go`, add conservative lane movement:

```go
func demoteBackgroundOnlyItems(summary StructuredSummary) StructuredSummary {
	kept := make([]string, 0, len(summary.ActivePriorities))
	for _, item := range summary.ActivePriorities {
		if containsAnySummaryMarker(item, "不是当前重点", "背景问题", "暂不处理") {
			summary.BackgroundIssues = append(summary.BackgroundIssues, item)
			continue
		}
		kept = append(kept, item)
	}
	summary.ActivePriorities = dedupeSummaryItems(kept)
	summary.BackgroundIssues = dedupeSummaryItems(summary.BackgroundIssues)
	return summary
}
```

In `summary_validator.go`, add deterministic checks:

```go
if sourceSetsCurrentFocus(sourceMessages) && len(summary.ActivePriorities) == 0 {
	result.Accepted = false
	result.FailureReasons = append(result.FailureReasons, "missing_active_priority")
}
if activePrioritiesContainBackgroundOnly(summary.ActivePriorities) {
	result.Accepted = false
	result.FailureReasons = append(result.FailureReasons, "background_issue_promoted")
}
```

- [ ] **Step 4: Run the history tests and verify they pass**

Run:

```powershell
go test ./internal/app/rag/core/history/... -count=1
```

Expected: PASS for repair, validation, renderer, and prompt-contract tests.

---

### Task 5: Regress with the Hard Planning Question

**Files:**
- Modify: `tmp/summary_compare_experiment.go`
- Produce: `tmp/summary_compare_experiment.json`
- Produce: `tmp/summary_compare_experiment.md`

- [ ] **Step 1: Keep the experiment pinned to the approved question**

Ensure the exact question string in `tmp/summary_compare_experiment.go` remains:

```go
question := "如果你现在接手这个项目，下一周你会怎么排优先级？请直接给方案，不要复述背景，控制在 6 条以内。"
```

- [ ] **Step 2: Run the experiment after hierarchy changes**

Run:

```powershell
$env:GOCACHE=(Join-Path (Resolve-Path '.').Path '.gocache-experiment')
$env:GOTMPDIR=(Join-Path (Resolve-Path '.').Path '.tmp-go-experiment')
New-Item -ItemType Directory -Force $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
go run ./tmp/summary_compare_experiment.go
```

Expected: command exits `0` and writes:

- `tmp/summary_compare_experiment.json`
- `tmp/summary_compare_experiment.md`

- [ ] **Step 3: Verify the priority-order outcome**

Inspect `tmp/summary_compare_experiment.md` and confirm:

- summary-backed answer begins with mainline items such as `spec/design/tasks`,
  `must_cover`, `critical_contract`, or sample-rule work
- `ERR_POOL_TIMEOUT` may appear, but not ahead of all mainline planning items
- `CI flaky` does not appear as a top active priority unless the question
  explicitly asks about CI

Use:

```powershell
Get-Content -Encoding utf8 -Raw tmp\summary_compare_experiment.md
```

Expected: the first 2-3 bullet points of the summary-backed answer align with
the original-conversation answer's mainline.

---

### Task 6: Run the Summary Suite Regression

**Files:**
- Produce: `testdata/evals/summary/latest_run_priority_hierarchy.json`

- [ ] **Step 1: Run focused package tests first**

Run:

```powershell
go test ./internal/app/rag/core/history/... -count=1
go test ./internal/app/rag/evaluation/... -run Summary -count=1
```

Expected: PASS before full-suite execution.

- [ ] **Step 2: Run the full summary suite**

Run:

```powershell
$env:GOCACHE=(Join-Path (Resolve-Path '.').Path '.gocache-summary-priority')
$env:GOTMPDIR=(Join-Path (Resolve-Path '.').Path '.tmp-go-summary-priority')
New-Item -ItemType Directory -Force $env:GOCACHE | Out-Null
New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
go run ./cmd/eval-runner -suite summary -input testdata/evals/summary/samples.json -output testdata/evals/summary/latest_run_priority_hierarchy.json
```

Expected: command exits `0` and writes a JSON suite result.

- [ ] **Step 3: Verify no regression in coverage and watch the drift signals**

Run:

```powershell
$data = Get-Content -Encoding utf8 -Raw testdata/evals/summary/latest_run_priority_hierarchy.json | ConvertFrom-Json
$passed = @($data.samples | Where-Object { $_.passed }).Count
$danger = @($data.samples | Where-Object { $_.critical_failures -contains 'dangerous_downstream_drift' }).Count
"passed=$passed"
"dangerous_drift=$danger"
```

Expected:

- `passed` is not lower than the current repaired baseline
- `dangerous_drift` does not increase

---

## Self-Review

### Spec coverage

- Explicit priority hierarchy fields are covered by Task 1.
- Prompt-side classification rules are covered by Task 2.
- Renderer ordering is covered by Task 3.
- Repair and validation alignment are covered by Task 4.
- Hard-question regression is covered by Task 5.
- Full-suite regression is covered by Task 6.

### Placeholder scan

- No `TODO`, `TBD`, or deferred placeholders remain.
- Each task names exact files and verification commands.

### Type consistency

- `ActivePriorities` and `BackgroundIssues` are introduced in Task 1 and reused
  consistently in Tasks 2-6.
- Validation and repair checks refer to the same field names and lane
  semantics throughout the plan.
