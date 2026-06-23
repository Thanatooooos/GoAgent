# Summary Long-Dialogue Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a resumable 24-turn external-model conversation generator and turn its reviewed output into a long summary-strategy evaluation sample that exercises the 800, 1200, and 1600 token thresholds.

**Architecture:** A checked-in JSON script owns the user questions and their evaluation purpose. A small evaluation-domain generator validates that script, sends accumulated chat history to the existing `infra-ai` chat service one turn at a time, measures tokens with the shared estimator, and checkpoints a provenance-safe raw artifact after each answer. A dedicated CLI wires configuration and the selected model; after the real run, the reviewed dialogue is curated into a normal strategy sample with hand-authored checkpoints and contracts.

**Tech Stack:** Go, existing `infra-ai/chat.LLMService`, `framework/convention.ChatRequest`, shared `core/tokenbudget` estimator, JSON fixtures, Go `testing`, existing `cmd/eval-runner`.

---

## File Structure

- Create `testdata/evals/summary/long_dialogue_questions.json`
  - Owns the scenario system instruction and all 24 controlled user turns.
- Create `internal/app/rag/evaluation/summary_dialog_generation.go`
  - Owns script/artifact types, validation, request construction, generation, resume rules, and suitability calculation.
- Create `internal/app/rag/evaluation/summary_dialog_generation_test.go`
  - Covers ordering, accumulated context, token accounting, interruption, resume, and empty responses.
- Create `internal/app/rag/evaluation/summary_dialog_artifact.go`
  - Owns raw artifact loading and checkpoint-safe JSON persistence.
- Create `internal/app/rag/evaluation/summary_dialog_artifact_test.go`
  - Covers malformed files, incompatible resumes, and safe persisted fields.
- Create `internal/app/rag/evaluation/summary_dialog_export.go`
  - Converts reviewed turns into evaluation `source_messages` and a checkpoint skeleton without inventing gold annotations.
- Create `internal/app/rag/evaluation/summary_dialog_export_test.go`
  - Verifies role/content-only export and checkpoint placement.
- Create `cmd/summary-dialog-gen/main.go`
  - Loads config, resolves model, invokes or resumes generation, and optionally exports the review draft.
- Create `cmd/summary-dialog-gen/main_test.go`
  - Covers flags, dependency wiring, validation-before-runtime, and exit behavior.
- Create `testdata/evals/summary/generated/software_project_state_transitions_v1.json`
  - Stores the reviewed, real-model dialogue as a reproducible fixture.
- Create `testdata/evals/summary/strategy_long_samples.json`
  - Stores the manually annotated long strategy sample.
- Create `internal/app/rag/evaluation/summary_dialog_fixture_test.go`
  - Verifies the reviewed fixture and curated strategy sample remain structurally valid.
- Modify `testdata/evals/summary/README.md`
  - Documents the generation, review, and sweep workflow.
- Modify `openspec/changes/add-token-aware-summary-compression/tasks.md`
  - Records the real long-dialogue threshold sweep and its interpretation.

### Task 1: Add the Controlled 24-Turn Question Script

**Files:**
- Create: `testdata/evals/summary/long_dialogue_questions.json`
- Test: `internal/app/rag/evaluation/summary_dialog_generation_test.go`

- [ ] **Step 1: Write a failing script parsing and validation test**

Add the following initial test:

```go
func TestParseSummaryDialogScriptRequiresSequentialTwentyFourTurns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "testdata", "evals", "summary",
		"long_dialogue_questions.json",
	))
	if err != nil {
		t.Fatal(err)
	}

	script, err := ParseSummaryDialogScript(raw)
	if err != nil {
		t.Fatalf("ParseSummaryDialogScript() error = %v", err)
	}
	if script.ScenarioID != "software_project_state_transitions_v1" {
		t.Fatalf("scenario_id = %q", script.ScenarioID)
	}
	if len(script.Turns) != 24 {
		t.Fatalf("turn count = %d, want 24", len(script.Turns))
	}
	for i, turn := range script.Turns {
		if turn.Turn != i+1 {
			t.Fatalf("turn[%d].turn = %d, want %d", i, turn.Turn, i+1)
		}
		if strings.TrimSpace(turn.Phase) == "" ||
			strings.TrimSpace(turn.Purpose) == "" ||
			strings.TrimSpace(turn.User) == "" {
			t.Fatalf("turn[%d] is incomplete: %+v", i, turn)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestParseSummaryDialogScriptRequiresSequentialTwentyFourTurns -count=1
```

Expected: FAIL because `ParseSummaryDialogScript` and the script file do not exist.

- [ ] **Step 3: Add the script schema and validation**

Create these types and parser in
`internal/app/rag/evaluation/summary_dialog_generation.go`:

```go
type SummaryDialogScript struct {
	SchemaVersion int                 `json:"schema_version"`
	ScenarioID    string              `json:"scenario_id"`
	SystemPrompt  string              `json:"system_prompt"`
	Turns         []SummaryDialogTurn `json:"turns"`
}

type SummaryDialogTurn struct {
	Turn    int    `json:"turn"`
	Phase   string `json:"phase"`
	Purpose string `json:"purpose"`
	User    string `json:"user"`
}

func ParseSummaryDialogScript(raw []byte) (SummaryDialogScript, error) {
	var script SummaryDialogScript
	if err := json.Unmarshal(raw, &script); err != nil {
		return SummaryDialogScript{}, fmt.Errorf("decode summary dialog script: %w", err)
	}
	script.ScenarioID = strings.TrimSpace(script.ScenarioID)
	script.SystemPrompt = strings.TrimSpace(script.SystemPrompt)
	if script.SchemaVersion != 1 {
		return SummaryDialogScript{}, fmt.Errorf("unsupported script schema_version %d", script.SchemaVersion)
	}
	if script.ScenarioID == "" {
		return SummaryDialogScript{}, fmt.Errorf("scenario_id is required")
	}
	if script.SystemPrompt == "" {
		return SummaryDialogScript{}, fmt.Errorf("system_prompt is required")
	}
	if len(script.Turns) != 24 {
		return SummaryDialogScript{}, fmt.Errorf("script requires exactly 24 turns, got %d", len(script.Turns))
	}
	for i := range script.Turns {
		turn := &script.Turns[i]
		turn.Phase = strings.TrimSpace(turn.Phase)
		turn.Purpose = strings.TrimSpace(turn.Purpose)
		turn.User = strings.TrimSpace(turn.User)
		if turn.Turn != i+1 {
			return SummaryDialogScript{}, fmt.Errorf("turn %d must have sequential number %d", i, i+1)
		}
		if turn.Phase == "" || turn.Purpose == "" || turn.User == "" {
			return SummaryDialogScript{}, fmt.Errorf("turn %d requires phase, purpose, and user", turn.Turn)
		}
	}
	return script, nil
}
```

- [ ] **Step 4: Create the exact 24-turn script**

Create `testdata/evals/summary/long_dialogue_questions.json` with:

```json
{
  "schema_version": 1,
  "scenario_id": "software_project_state_transitions_v1",
  "system_prompt": "你是这个软件项目的技术协作伙伴。请基于完整对话连续回答，不要假装执行了尚未执行的工作。每次回答使用自然的中文，通常写 2 到 4 个紧凑段落，给出具体理由、影响和下一步；保留不确定性，不要把假设说成事实，也不要主动遗忘已经确认且仍有效的约束。",
  "turns": [
    {
      "turn": 1,
      "phase": "initial_scope",
      "purpose": "establish the initial goal",
      "user": "我们要建设一个离线评估框架。先帮我梳理它当前最核心的目标、为什么需要它，以及第一版应该证明什么。"
    },
    {
      "turn": 2,
      "phase": "initial_scope",
      "purpose": "narrow phase one scope",
      "user": "第一阶段先只覆盖 summary 和 rewrite，不做 tool 和最终回答评估。请分析这个范围的合理性，以及它会怎样影响样本和指标设计。"
    },
    {
      "turn": 3,
      "phase": "initial_scope",
      "purpose": "define delivery criteria",
      "user": "第一版交付要能重复运行、输出结构化结果，并且能定位失败原因。请把这些要求转成可验证的交付标准。"
    },
    {
      "turn": 4,
      "phase": "initial_scope",
      "purpose": "establish no-implementation constraint",
      "user": "现在仍处于 spec、design 和 tasks 收口阶段，暂时不能开始生产实现。请说明此刻应该做什么、不应该做什么。"
    },
    {
      "turn": 5,
      "phase": "constraints",
      "purpose": "establish initial database decision",
      "user": "评估结果存储先暂定 MySQL，并要求开发和生产使用同一种数据库。请评估这个决定会涉及哪些数据结构和查询能力。"
    },
    {
      "turn": 6,
      "phase": "constraints",
      "purpose": "introduce compatibility constraint",
      "user": "还要保留现有 turn 阈值脚本的兼容性，新方案不能让旧命令立刻失效。请给出兼容策略和迁移边界。"
    },
    {
      "turn": 7,
      "phase": "constraints",
      "purpose": "introduce schedule and quality tension",
      "user": "我们希望本周得到可运行基线，但不能为了赶进度跳过金标准样本和失败诊断。请分析怎样切分工作才能同时满足进度与质量。"
    },
    {
      "turn": 8,
      "phase": "constraints",
      "purpose": "add reproducibility requirements",
      "user": "每次评估还必须记录模型、样本集、估算器版本和运行时间，不能把 API key 或请求头写进产物。请设计最小的运行元数据。"
    },
    {
      "turn": 9,
      "phase": "investigation",
      "purpose": "introduce an exact error entity",
      "user": "第一次 summary 基线出现了 ERR_SUMMARY_DRIFT：摘要保留了过期决策，却漏掉当前约束。请分析可能原因，但不要直接断言根因已经确认。"
    },
    {
      "turn": 10,
      "phase": "investigation",
      "purpose": "introduce exact configuration evidence",
      "user": "当前配置线索是 summary.history-turn-threshold=6，并且长消息会让同样的 6 turn 大小差异很大。请解释这条证据支持什么、不支持什么。"
    },
    {
      "turn": 11,
      "phase": "investigation",
      "purpose": "compare trigger hypotheses",
      "user": "团队提出两个假设：按 turn 触发不稳定；按 token 触发可能更接近真实上下文压力。请给出验证这两个假设需要的实验，而不是直接选边。"
    },
    {
      "turn": 12,
      "phase": "investigation",
      "purpose": "preserve an unresolved scoring question",
      "user": "自动评分的危险漂移阈值现在还没有足够数据确定。请明确把它保留为开放问题，并说明在阈值未定时可以先做哪些工作。"
    },
    {
      "turn": 13,
      "phase": "decision_override",
      "purpose": "introduce evidence against MySQL",
      "user": "新证据显示评估查询需要 pgvector 和更方便的 JSON 字段分析，这与之前暂定的 MySQL 方案有冲突。请重新评估数据库选择。"
    },
    {
      "turn": 14,
      "phase": "decision_override",
      "purpose": "replace database decision and invalidate stale state",
      "user": "现在正式决定开发和生产统一改用 PostgreSQL，之前的 MySQL 方案作废。请说明当前有效决定，以及摘要和后续任务中必须怎样处理旧决定。"
    },
    {
      "turn": 15,
      "phase": "decision_override",
      "purpose": "analyze consequences of the replacement",
      "user": "基于 PostgreSQL 新决定，请重新梳理存储、JSON 查询、向量能力和迁移风险，但不要声称迁移已经执行。"
    },
    {
      "turn": 16,
      "phase": "decision_override",
      "purpose": "carry forward an earlier constraint",
      "user": "数据库决定虽然变了，但当前仍不能进入生产实现。请确认哪些状态发生了变化，哪些早期约束仍然有效。"
    },
    {
      "turn": 17,
      "phase": "goal_shift",
      "purpose": "shift the active goal to compression timing",
      "user": "主线现在切换到 summary 压缩触发时机：要从按 turn 改为按 token 评估。请更新当前目标，同时说明离线评估框架的大背景如何保留。"
    },
    {
      "turn": 18,
      "phase": "goal_shift",
      "purpose": "establish candidate token thresholds",
      "user": "候选 token 阈值先比较 800、1200、1600。请设计这三个候选应该比较的压缩次数、token 节省、摘要正确性和下游延续质量。"
    },
    {
      "turn": 19,
      "phase": "goal_shift",
      "purpose": "preserve unresolved safety factor",
      "user": "生产触发可能需要 safety factor，但现在没有真实 trace 分布。请判断离线 sweep 是否应直接加入 safety factor，并把尚未解决的部分说清楚。"
    },
    {
      "turn": 20,
      "phase": "goal_shift",
      "purpose": "define retrieve and tool accounting",
      "user": "后续 retrieve 和 tool 阶段还会注入内容。请说明 token 预算中怎样估算这些动态内容，以及估算误差应该如何被观测。"
    },
    {
      "turn": 21,
      "phase": "reconciliation",
      "purpose": "separate completed progress from planned work",
      "user": "请做一次进度盘点：区分已经确认的设计决定、已经完成的验证、尚未执行的实现，以及仍开放的问题。不要把计划写成已完成。"
    },
    {
      "turn": 22,
      "phase": "reconciliation",
      "purpose": "test long-range state recall",
      "user": "回看整个讨论，当前数据库到底是什么，MySQL 是否还有效，当前能否开始生产实现，旧的 turn 阈值兼容要求是否仍然存在？请逐项回答并给依据。"
    },
    {
      "turn": 23,
      "phase": "reconciliation",
      "purpose": "reconcile all open questions",
      "user": "现在还有哪些问题没有定论？请特别区分危险漂移评分阈值、production safety factor、真实 trace 校准和已经确定的 800/1200/1600 离线候选。"
    },
    {
      "turn": 24,
      "phase": "reconciliation",
      "purpose": "test final next-action recommendation",
      "user": "如果现在只能推进一件事，下一步应该是什么？请结合当前主目标、不能直接进入生产实现的约束、需要真实长对话样本以及后续阈值 sweep 给出明确建议。"
    }
  ]
}
```

- [ ] **Step 5: Run the script test**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestParseSummaryDialogScriptRequiresSequentialTwentyFourTurns -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the script contract**

```powershell
git add testdata/evals/summary/long_dialogue_questions.json internal/app/rag/evaluation/summary_dialog_generation.go internal/app/rag/evaluation/summary_dialog_generation_test.go
git commit -m "test: define long summary dialogue scenario"
```

### Task 2: Implement Artifact Persistence, Token Suitability, and Resume Validation

**Files:**
- Create: `internal/app/rag/evaluation/summary_dialog_artifact.go`
- Create: `internal/app/rag/evaluation/summary_dialog_artifact_test.go`
- Modify: `internal/app/rag/evaluation/summary_dialog_generation.go`

- [ ] **Step 1: Write failing artifact tests**

Cover these exact behaviors:

```go
func TestWriteAndLoadSummaryDialogArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dialog.json")
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "scenario",
		Status:        SummaryDialogStatusInProgress,
		Provider:      "configured",
		Model:         "model-a",
		Estimator: SummaryDialogEstimatorMetadata{
			Name: "fixed", Version: "test", MessageOverheadTokens: 4,
		},
		Turns: []SummaryDialogGeneratedTurn{{
			Turn: 1, Phase: "scope", Purpose: "goal",
			User: "question", Assistant: "answer", CumulativeTokens: 28,
		}},
	}
	if err := WriteSummaryDialogArtifact(path, artifact); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSummaryDialogArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(artifact, loaded); diff != "" {
		t.Fatalf("artifact mismatch (-want +got):\n%s", diff)
	}
}

func TestValidateSummaryDialogResumeRejectsDifferentModel(t *testing.T) {
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1, ScenarioID: "scenario", Model: "model-a",
	}
	err := ValidateSummaryDialogResume(
		artifact, "scenario", "configured", "model-b", 4,
	)
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model mismatch, got %v", err)
	}
}

func TestSummaryDialogSuitabilityRequiresDistinctCrossingTurns(t *testing.T) {
	turns := []SummaryDialogGeneratedTurn{
		{Turn: 5, CumulativeTokens: 790},
		{Turn: 6, CumulativeTokens: 810},
		{Turn: 10, CumulativeTokens: 1210},
		{Turn: 15, CumulativeTokens: 1610},
		{Turn: 24, CumulativeTokens: 2500},
	}
	got := EvaluateSummaryDialogSuitability(turns)
	if !got.Suitable || got.CrossedAt["800"] != 6 ||
		got.CrossedAt["1200"] != 10 || got.CrossedAt["1600"] != 15 {
		t.Fatalf("unexpected suitability: %+v", got)
	}
}
```

Use standard equality checks instead of `cmp.Diff` if `go-cmp` is not already a
direct test dependency.

- [ ] **Step 2: Run artifact tests to verify failure**

Run:

```powershell
go test ./internal/app/rag/evaluation -run 'Test(WriteAndLoadSummaryDialogArtifact|ValidateSummaryDialogResume|SummaryDialogSuitability)' -count=1
```

Expected: FAIL because artifact APIs do not exist.

- [ ] **Step 3: Define the artifact and suitability types**

Add:

```go
type SummaryDialogStatus string

const (
	SummaryDialogStatusInProgress SummaryDialogStatus = "in_progress"
	SummaryDialogStatusComplete   SummaryDialogStatus = "complete"
)

type SummaryDialogEstimatorMetadata struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	MessageOverheadTokens int    `json:"message_overhead_tokens"`
}

type SummaryDialogGeneratedTurn struct {
	Turn             int       `json:"turn"`
	Phase            string    `json:"phase"`
	Purpose          string    `json:"purpose"`
	User             string    `json:"user"`
	Assistant        string    `json:"assistant"`
	CumulativeTokens int       `json:"cumulative_tokens"`
	GeneratedAt      time.Time `json:"generated_at"`
}

type SummaryDialogSuitability struct {
	Suitable   bool           `json:"suitable"`
	CrossedAt  map[string]int `json:"crossed_at"`
	FinalTokens int           `json:"final_tokens"`
	Reasons    []string       `json:"reasons,omitempty"`
}

type SummaryDialogArtifact struct {
	SchemaVersion int                            `json:"schema_version"`
	ScenarioID    string                         `json:"scenario_id"`
	Status        SummaryDialogStatus            `json:"status"`
	Provider      string                         `json:"provider"`
	Model         string                         `json:"model"`
	Estimator     SummaryDialogEstimatorMetadata `json:"estimator"`
	Turns         []SummaryDialogGeneratedTurn   `json:"turns"`
	Suitability   SummaryDialogSuitability       `json:"suitability"`
}
```

Implement `EvaluateSummaryDialogSuitability` with thresholds
`[]int{800, 1200, 1600}`. Record the first completed turn at which each threshold
is reached. Suitability is true only when all crossings exist at strictly
increasing turns and final tokens are at least 2400.

- [ ] **Step 4: Implement safe JSON persistence**

Follow the repository's existing checkpoint-file pattern:

```go
func WriteSummaryDialogArtifact(path string, artifact SummaryDialogArtifact) error {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary dialog artifact: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure summary dialog directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create summary dialog temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write summary dialog temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close summary dialog temp file: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace summary dialog artifact: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit summary dialog artifact: %w", err)
	}
	return nil
}
```

`LoadSummaryDialogArtifact` must reject malformed JSON, schema versions other
than 1, missing scenario/model values, non-sequential persisted turns, and
empty persisted answers.

`ValidateSummaryDialogResume` must reject mismatches in scenario ID, provider,
model ID, and message overhead. It must also reject an artifact with more turns
than the script.

- [ ] **Step 5: Run artifact tests**

Run:

```powershell
go test ./internal/app/rag/evaluation -run 'Test(WriteAndLoadSummaryDialogArtifact|ValidateSummaryDialogResume|SummaryDialogSuitability)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit artifact behavior**

```powershell
git add internal/app/rag/evaluation/summary_dialog_artifact.go internal/app/rag/evaluation/summary_dialog_artifact_test.go internal/app/rag/evaluation/summary_dialog_generation.go
git commit -m "feat: persist resumable summary dialogues"
```

### Task 3: Implement Sequential External-Model Generation

**Files:**
- Modify: `internal/app/rag/evaluation/summary_dialog_generation.go`
- Modify: `internal/app/rag/evaluation/summary_dialog_generation_test.go`

- [ ] **Step 1: Write failing generation tests**

Define a narrow dependency:

```go
type SummaryDialogChat interface {
	ChatWithModel(convention.ChatRequest, string) (string, error)
}
```

Add these test helpers:

```go
type recordingSummaryDialogChat struct {
	responses []string
	requests  []convention.ChatRequest
	errAt     int
}

func (c *recordingSummaryDialogChat) ChatWithModel(
	request convention.ChatRequest,
	_ string,
) (string, error) {
	index := len(c.requests)
	c.requests = append(c.requests, request)
	if c.errAt > 0 && index+1 == c.errAt {
		return "", errors.New("provider failed")
	}
	if index >= len(c.responses) {
		return "", errors.New("missing fake response")
	}
	return c.responses[index], nil
}

type recordingSummaryDialogStore struct {
	snapshots []SummaryDialogArtifact
}

func (s *recordingSummaryDialogStore) Save(artifact SummaryDialogArtifact) error {
	s.snapshots = append(s.snapshots, artifact)
	return nil
}

func validTwentyFourTurnScript() SummaryDialogScript {
	turns := make([]SummaryDialogTurn, 24)
	for i := range turns {
		turns[i] = SummaryDialogTurn{
			Turn: i + 1, Phase: "phase", Purpose: "purpose",
			User: fmt.Sprintf("q%d", i+1),
		}
	}
	return SummaryDialogScript{
		SchemaVersion: 1, ScenarioID: "scenario",
		SystemPrompt: "system", Turns: turns,
	}
}

func repeatedAnswers(count int) []string {
	answers := make([]string, count)
	for i := range answers {
		answers[i] = fmt.Sprintf("a%d", i+1)
	}
	return answers
}
```

Add tests proving:

```go
func TestGenerateSummaryDialogCarriesAccumulatedContextAndPersistsEachTurn(t *testing.T) {
	script := validTwentyFourTurnScript()
	chat := &recordingSummaryDialogChat{responses: repeatedAnswers(24)}
	store := &recordingSummaryDialogStore{}

	artifact, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script:                script,
		ModelID:              "model-a",
		Provider:             "configured",
		Chat:                 chat,
		Estimator:            tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4,
		Store:                store,
		Now:                  fixedClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 24 || len(store.snapshots) != 24 {
		t.Fatalf("requests=%d snapshots=%d", len(chat.requests), len(store.snapshots))
	}
	gotRoles := messageRoles(chat.requests[2].Messages)
	wantRoles := []convention.Role{
		convention.SystemRole,
		convention.UserRole, convention.AssistantRole,
		convention.UserRole, convention.AssistantRole,
		convention.UserRole,
	}
	if !slices.Equal(gotRoles, wantRoles) {
		t.Fatalf("roles = %v, want %v", gotRoles, wantRoles)
	}
	if artifact.Status != SummaryDialogStatusComplete {
		t.Fatalf("status = %q", artifact.Status)
	}
}

func TestGenerateSummaryDialogResumesWithoutRegeneratingCompletedTurns(t *testing.T) {
	script := validTwentyFourTurnScript()
	existing := SummaryDialogArtifact{
		SchemaVersion: 1, ScenarioID: "scenario",
		Status: SummaryDialogStatusInProgress, Model: "model-a",
		Estimator: SummaryDialogEstimatorMetadata{
			Name: "rune", MessageOverheadTokens: 4,
		},
	}
	for i := 0; i < 23; i++ {
		existing.Turns = append(existing.Turns, SummaryDialogGeneratedTurn{
			Turn: i + 1, Phase: "phase", Purpose: "purpose",
			User: fmt.Sprintf("q%d", i+1), Assistant: fmt.Sprintf("a%d", i+1),
		})
	}
	chat := &recordingSummaryDialogChat{responses: []string{"a24"}}

	artifact, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script: script, Existing: &existing, ModelID: "model-a",
		Chat: chat, Estimator: tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4, Store: &recordingSummaryDialogStore{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 1 || len(artifact.Turns) != 24 {
		t.Fatalf("requests=%d turns=%d", len(chat.requests), len(artifact.Turns))
	}
}

func TestGenerateSummaryDialogKeepsCheckpointWhenLaterCallFails(t *testing.T) {
	chat := &recordingSummaryDialogChat{
		responses: repeatedAnswers(24),
		errAt: 2,
	}
	store := &recordingSummaryDialogStore{}
	_, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script: validTwentyFourTurnScript(), ModelID: "model-a",
		Chat: chat, Estimator: tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4, Store: store,
	})
	if err == nil || len(store.snapshots) != 1 {
		t.Fatalf("err=%v snapshots=%d", err, len(store.snapshots))
	}
}

func TestGenerateSummaryDialogRejectsEmptyAnswerWithoutAdvancing(t *testing.T) {
	store := &recordingSummaryDialogStore{}
	_, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script: validTwentyFourTurnScript(), ModelID: "model-a",
		Chat: &recordingSummaryDialogChat{responses: []string{"   "}},
		Estimator: tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4, Store: store,
	})
	if err == nil || len(store.snapshots) != 0 {
		t.Fatalf("err=%v snapshots=%d", err, len(store.snapshots))
	}
}
```

- [ ] **Step 2: Run generation tests to verify failure**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestGenerateSummaryDialog -count=1
```

Expected: FAIL because generation APIs do not exist.

- [ ] **Step 3: Implement request construction and token accounting**

Add:

```go
type SummaryDialogArtifactStore interface {
	Save(SummaryDialogArtifact) error
}

type SummaryDialogGenerationInput struct {
	Script                 SummaryDialogScript
	Existing               *SummaryDialogArtifact
	ModelID                string
	Provider               string
	Chat                   SummaryDialogChat
	Estimator              tokenbudget.Estimator
	MessageOverheadTokens  int
	Store                  SummaryDialogArtifactStore
	Now                    func() time.Time
}

func buildSummaryDialogRequest(
	script SummaryDialogScript,
	completed []SummaryDialogGeneratedTurn,
	next SummaryDialogTurn,
) convention.ChatRequest {
	messages := make([]convention.ChatMessage, 0, 2+len(completed)*2)
	messages = append(messages, convention.SystemMessage(script.SystemPrompt))
	for _, turn := range completed {
		messages = append(messages,
			convention.UserMessage(turn.User),
			convention.AssistantMessage(turn.Assistant),
		)
	}
	messages = append(messages, convention.UserMessage(next.User))
	temperature := 0.4
	maxTokens := 800
	thinking := false
	return convention.ChatRequest{
		Messages: messages,
		Temperature: &temperature,
		MaxTokens: &maxTokens,
		Thinking: &thinking,
	}
}
```

After receiving an answer, compute cumulative tokens over all completed user and
assistant messages with:

```go
func estimateGeneratedDialogTokens(
	turns []SummaryDialogGeneratedTurn,
	estimator tokenbudget.Estimator,
	overhead int,
) int {
	messages := make([]convention.ChatMessage, 0, len(turns)*2)
	for _, turn := range turns {
		messages = append(messages,
			convention.UserMessage(turn.User),
			convention.AssistantMessage(turn.Assistant),
		)
	}
	return tokenbudget.EstimateMessages(messages, estimator, overhead)
}
```

Do not count the scenario system prompt in `cumulative_tokens`; strategy
evaluation measures the source user/assistant dialogue.

- [ ] **Step 4: Implement the generation loop**

`GenerateSummaryDialog` must:

1. Validate script, model, chat, estimator, and store.
2. Initialize or validate the existing artifact.
3. Start at `len(existing.Turns)`.
4. Check `ctx.Err()` before every external call.
5. Build a full-history request and call `ChatWithModel`.
6. Trim and reject an empty answer.
7. Append one generated turn.
8. Recompute cumulative tokens and suitability.
9. Set status to `complete` only after the final turn.
10. Save after every successful answer.
11. Return an error containing the failed turn number without modifying the
    already saved artifact.

Use `time.Now().UTC` when `Now` is nil. Normalize negative overhead to zero.

- [ ] **Step 5: Run generation tests**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestGenerateSummaryDialog -count=1
```

Expected: PASS.

- [ ] **Step 6: Run the complete evaluation package**

Run:

```powershell
go test ./internal/app/rag/evaluation -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit generation behavior**

```powershell
git add internal/app/rag/evaluation/summary_dialog_generation.go internal/app/rag/evaluation/summary_dialog_generation_test.go
git commit -m "feat: generate long summary dialogues"
```

### Task 4: Add Review-Draft Export

**Files:**
- Create: `internal/app/rag/evaluation/summary_dialog_export.go`
- Create: `internal/app/rag/evaluation/summary_dialog_export_test.go`

- [ ] **Step 1: Write the failing export test**

```go
func completedArtifactWithTwentyFourTurns() SummaryDialogArtifact {
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID: "software_project_state_transitions_v1",
		Status: SummaryDialogStatusComplete,
		Model: "model-a",
		Suitability: SummaryDialogSuitability{
			Suitable: true, FinalTokens: 2500,
			CrossedAt: map[string]int{"800": 6, "1200": 10, "1600": 15},
		},
	}
	for i := 0; i < 24; i++ {
		artifact.Turns = append(artifact.Turns, SummaryDialogGeneratedTurn{
			Turn: i + 1,
			Phase: "phase",
			Purpose: "purpose",
			User: fmt.Sprintf("q%d", i+1),
			Assistant: fmt.Sprintf("a%d", i+1),
		})
	}
	return artifact
}

func TestBuildSummaryDialogReviewDraftExportsRoleContentAndCheckpoints(t *testing.T) {
	artifact := completedArtifactWithTwentyFourTurns()
	draft, err := BuildSummaryDialogReviewDraft(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Input.SourceMessages) != 48 {
		t.Fatalf("source message count = %d", len(draft.Input.SourceMessages))
	}
	if got := draft.Input.SourceMessages[0]; got.Role != "user" || got.Content != artifact.Turns[0].User {
		t.Fatalf("first message = %+v", got)
	}
	if got := draft.Input.SourceMessages[1]; got.Role != "assistant" || got.Content != artifact.Turns[0].Assistant {
		t.Fatalf("second message = %+v", got)
	}
	want := []int{6, 12, 18, 24}
	for i, checkpoint := range draft.StrategyEval.Checkpoints {
		if checkpoint.AfterTurn != want[i] {
			t.Fatalf("checkpoint[%d] = %d", i, checkpoint.AfterTurn)
		}
	}
	if draft.StrategyEval.FinalEval == nil || draft.StrategyEval.FinalEval.AfterTurn != 24 {
		t.Fatalf("final eval = %+v", draft.StrategyEval.FinalEval)
	}
	if len(draft.ExpectedSummary.Goal.MustCover) != 0 ||
		len(draft.CriticalContract.CriticalFacts) != 0 {
		t.Fatal("export must not invent gold annotations")
	}
}
```

- [ ] **Step 2: Run the test to verify failure**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestBuildSummaryDialogReviewDraft -count=1
```

Expected: FAIL because export APIs do not exist.

- [ ] **Step 3: Implement the review-draft export**

Add:

```go
func BuildSummaryDialogReviewDraft(artifact SummaryDialogArtifact) (SummarySample, error) {
	if artifact.Status != SummaryDialogStatusComplete || len(artifact.Turns) != 24 {
		return SummarySample{}, fmt.Errorf("completed 24-turn artifact is required")
	}
	messages := make([]SummaryMessage, 0, 48)
	for _, turn := range artifact.Turns {
		messages = append(messages,
			SummaryMessage{Role: "user", Content: turn.User},
			SummaryMessage{Role: "assistant", Content: turn.Assistant},
		)
	}
	checkpoints := make([]SummaryStrategyCheckpoint, 0, 4)
	for _, afterTurn := range []int{6, 12, 18, 24} {
		checkpoints = append(checkpoints, SummaryStrategyCheckpoint{AfterTurn: afterTurn})
	}
	return SummarySample{
		Name: "software_project_state_transitions_long_dialogue",
		Tags: []string{
			"strategy", "long_dialog", "state_override",
			"goal_shift", "open_questions",
		},
		Input: SummaryInput{SourceMessages: messages},
		StrategyEval: &SummaryStrategyEval{
			Checkpoints: checkpoints,
			FinalEval: &SummaryStrategyCheckpoint{AfterTurn: 24},
		},
		Metadata: map[string]any{
			"scenario_id": artifact.ScenarioID,
			"source_model": artifact.Model,
			"source_tokens": artifact.Suitability.FinalTokens,
			"review_status": "annotations_required",
		},
	}, nil
}
```

The function deliberately leaves scoring annotations empty. It is a review
draft, not a valid claim that the model-generated dialogue established every
scripted fact.

- [ ] **Step 4: Run export tests**

Run:

```powershell
go test ./internal/app/rag/evaluation -run TestBuildSummaryDialogReviewDraft -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit export behavior**

```powershell
git add internal/app/rag/evaluation/summary_dialog_export.go internal/app/rag/evaluation/summary_dialog_export_test.go
git commit -m "feat: export summary dialogue review drafts"
```

### Task 5: Add the `summary-dialog-gen` CLI

**Files:**
- Create: `cmd/summary-dialog-gen/main.go`
- Create: `cmd/summary-dialog-gen/main_test.go`

- [ ] **Step 1: Write failing CLI option tests**

Refactor execution behind:

```go
type summaryDialogGenOptions struct {
	ScriptPath string
	OutputPath string
	DraftPath  string
	ModelID    string
	ConfigDir  string
	Provider   string
	Overhead   int
	Overwrite  bool
}

type summaryDialogGenDeps struct {
	LoadConfig func(string) error
	NewChat    func() aichat.LLMService
}
```

Test:

- default script is
  `testdata/evals/summary/long_dialogue_questions.json`
- default raw output is
  `tmp/software_project_state_transitions_v1_raw.json`
- default draft output is empty, so no review draft is written unless requested
- empty model is rejected before runtime creation
- malformed script is rejected before runtime creation
- existing raw output resumes from its next unanswered turn
- `-overwrite` ignores an existing raw artifact and starts from turn 1
- a complete artifact can be exported with `-draft-output`

Use a fake `LLMService` that implements all interface methods and records
`ChatWithModel` calls.

- [ ] **Step 2: Run CLI tests to verify failure**

Run:

```powershell
go test ./cmd/summary-dialog-gen -count=1
```

Expected: FAIL because the command does not exist.

- [ ] **Step 3: Implement CLI parsing and runtime wiring**

Support:

```text
-script testdata/evals/summary/long_dialogue_questions.json
-output tmp/software_project_state_transitions_v1_raw.json
-draft-output tmp/software_project_state_transitions_v1_draft.json
-model qwen-max-test
-provider configured
-config-dir configs
-message-overhead 4
-overwrite=false
```

`runSummaryDialogGen` must:

1. Read and parse the script.
2. Load an existing output if present and `-overwrite` is false.
3. Load `.env` without overriding existing environment variables.
4. Load application config.
5. Create `infraai.NewRuntime().Chat`.
6. Invoke `GenerateSummaryDialog`.
7. Write the final artifact through the same store.
8. If `-draft-output` is non-empty and generation completed, marshal
   `[]SummarySample{draft}` as the top-level JSON array expected by
   `eval-runner`.
9. Print completed turns, final token count, crossing turns, suitability, and
   output paths.

Use a file-backed store:

```go
type summaryDialogFileStore struct{ path string }

func (s summaryDialogFileStore) Save(artifact rageval.SummaryDialogArtifact) error {
	return rageval.WriteSummaryDialogArtifact(s.path, artifact)
}
```

- [ ] **Step 4: Run CLI tests**

Run:

```powershell
go test ./cmd/summary-dialog-gen -count=1
```

Expected: PASS.

- [ ] **Step 5: Run focused tests**

Run:

```powershell
go test ./cmd/summary-dialog-gen ./internal/app/rag/evaluation ./internal/app/rag/core/tokenbudget -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the CLI**

```powershell
git add cmd/summary-dialog-gen internal/app/rag/evaluation/summary_dialog_*.go
git commit -m "feat: add resumable summary dialogue generator"
```

### Task 6: Generate and Review the Real 24-Turn Dialogue

**Files:**
- Create: `testdata/evals/summary/generated/software_project_state_transitions_v1.json`
- Create: `testdata/evals/summary/strategy_long_samples.json`
- Create: `internal/app/rag/evaluation/summary_dialog_fixture_test.go`

- [ ] **Step 1: Run the real external-model generation with explicit approval**

Before this command, confirm approval to transmit the controlled question script
and accumulated generated dialogue to the configured external provider.

Run:

```powershell
go run ./cmd/summary-dialog-gen `
  -script testdata/evals/summary/long_dialogue_questions.json `
  -output tmp/software_project_state_transitions_v1_raw.json `
  -draft-output tmp/software_project_state_transitions_v1_draft.json `
  -model qwen-max-test `
  -provider configured `
  -config-dir configs `
  -message-overhead 4
```

Expected:

- exit code 0
- 24 completed turns
- final tokens at least 2400
- 800, 1200, and 1600 crossed at strictly increasing completed turns
- `suitable=true`

If the call is interrupted, rerun the same command. It must resume without
regenerating completed turns.

- [ ] **Step 2: Review the raw dialogue**

Review every generated answer against its preceding messages. Reject and
regenerate the scenario as a new scenario version if any of these are true:

- it claims implementation or migration already happened when it did not
- it treats MySQL as current after turn 14
- it loses the no-production-implementation constraint
- it turns the dangerous-drift threshold or safety factor into a decided value
- it claims retrieve/tool token estimates are exact
- it fails to carry the legacy turn compatibility requirement
- it contains credentials, request headers, or provider internals

Do not edit individual assistant answers silently. Preserve the generated raw
artifact and create a new scenario version when regeneration is necessary.

- [ ] **Step 3: Copy the approved reproducible dialogue fixture**

Copy the approved raw artifact to:

```text
testdata/evals/summary/generated/software_project_state_transitions_v1.json
```

Keep the concrete model, estimator metadata, timestamps, turns, cumulative
tokens, and suitability. Confirm no secrets are present before staging.

- [ ] **Step 4: Author the long strategy sample from dialogue evidence**

Start from `tmp/software_project_state_transitions_v1_draft.json`. For each
checkpoint, add only facts actually present in the generated answers:

- turn 6: initial goal, Phase-1 summary/rewrite scope, no-implementation
  constraint, initial database state, and legacy compatibility
- turn 12: `ERR_SUMMARY_DRIFT`, `summary.history-turn-threshold=6`, two trigger
  hypotheses, and unresolved dangerous-drift threshold
- turn 18: PostgreSQL replacing MySQL, MySQL invalidation, no-implementation
  constraint still active, active goal shifted to token triggering, and
  800/1200/1600 candidates
- turn 24: current database and goal, stale-state prohibition, retrieve/tool
  estimation approach, all remaining open questions, and the recommended next
  action

For every checkpoint:

- put indispensable state in `must_cover` and `critical_contract`
- put stale MySQL/current-turn-only claims in `must_not_claim` and
  `forbidden_claims`
- add one to three `next_turn_eval` queries that require earlier state
- make `final_eval.after_turn` equal 24
- change metadata `review_status` to `reviewed`

Save the resulting top-level JSON array to
`testdata/evals/summary/strategy_long_samples.json`.

- [ ] **Step 5: Add a fixture validation test**

Create `internal/app/rag/evaluation/summary_dialog_fixture_test.go`:

```go
func TestLongSummaryStrategyFixture(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(
		root, "testdata", "evals", "summary", "strategy_long_samples.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var encoded []json.RawMessage
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	samples, err := ParseSummarySamples(encoded)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("sample count = %d, want 1", len(samples))
	}
	sample := samples[0]
	if len(sample.Input.SourceMessages) != 48 {
		t.Fatalf("source message count = %d, want 48", len(sample.Input.SourceMessages))
	}
	if sample.StrategyEval == nil || len(sample.StrategyEval.Checkpoints) != 4 {
		t.Fatalf("strategy eval = %+v", sample.StrategyEval)
	}
	wantTurns := []int{6, 12, 18, 24}
	for i, checkpoint := range sample.StrategyEval.Checkpoints {
		if checkpoint.AfterTurn != wantTurns[i] {
			t.Fatalf("checkpoint[%d] = %d, want %d", i, checkpoint.AfterTurn, wantTurns[i])
		}
	}
	if sample.StrategyEval.FinalEval == nil ||
		sample.StrategyEval.FinalEval.AfterTurn != 24 {
		t.Fatalf("final eval = %+v", sample.StrategyEval.FinalEval)
	}
	if sample.Metadata["review_status"] != "reviewed" {
		t.Fatalf("review_status = %v", sample.Metadata["review_status"])
	}
}
```

- [ ] **Step 6: Validate the curated sample without external judging**

Run the parser and strategy unit tests:

```powershell
go test ./internal/app/rag/evaluation -run 'Test(LongSummaryStrategyFixture|ParseSummarySamples|SummaryStrategy)' -count=1
```

Expected: sample parsing succeeds, checkpoints are 6/12/18/24, final eval is 24,
and there are 48 source messages.

- [ ] **Step 7: Commit the reviewed fixture and sample**

```powershell
git add testdata/evals/summary/generated/software_project_state_transitions_v1.json testdata/evals/summary/strategy_long_samples.json internal/app/rag/evaluation/summary_dialog_fixture_test.go
git commit -m "test: add realistic long summary strategy sample"
```

### Task 7: Run the Real Threshold Sweep and Document the Workflow

**Files:**
- Modify: `testdata/evals/summary/README.md`
- Modify: `openspec/changes/add-token-aware-summary-compression/tasks.md`
- Create: `tmp/summary_strategy_long_thresholds_20260623.json` (not committed)

- [ ] **Step 1: Run the 800/1200/1600 real-model sweep**

With explicit approval for the reviewed dialogue to be transmitted to the
configured model provider, run:

```powershell
go run ./cmd/eval-runner `
  -suite summary `
  -input testdata/evals/summary/strategy_long_samples.json `
  -summary-mode strategy `
  -summary-token-thresholds 800,1200,1600 `
  -output tmp/summary_strategy_long_thresholds_20260623.json
```

Expected:

- each threshold triggers at least once
- threshold results report `threshold_unit: "tokens"`
- compression counts differ for at least two candidates, or their committed
  coverage timing differs
- token saved ratio, fidelity, usefulness, and downstream equivalence are
  present

- [ ] **Step 2: Inspect the result with the standard library JSON parser**

Run:

```powershell
@'
import json
from pathlib import Path

path = Path("tmp/summary_strategy_long_thresholds_20260623.json")
payload = json.loads(path.read_text(encoding="utf-8"))
for row in payload["aggregate"]["thresholds"]:
    print({
        "threshold": row["threshold"],
        "unit": row["threshold_unit"],
        "calls": row["summary_call_count"],
        "saved": row["token_saved_ratio"],
        "fidelity": row["structured_fidelity"],
        "usefulness": row["structured_usefulness"],
        "equivalence": row["downstream_equivalence"],
        "pass_rate": row["pass_rate"],
    })
'@ | python -
```

Expected: three rows for 800, 1200, and 1600 with nonzero summary call counts.

- [ ] **Step 3: Document generation and review commands**

Add a “Long strategy dialogue generation” section to
`testdata/evals/summary/README.md` documenting:

- the controlled script path
- the raw artifact path
- resume behavior
- explicit external-provider approval requirement
- the review rule that generated answers are not gold annotations
- the curated sample path
- the threshold sweep command

- [ ] **Step 4: Update OpenSpec evidence**

In `openspec/changes/add-token-aware-summary-compression/tasks.md`, record:

- generation date and model
- final measured dialogue tokens
- crossing turns for 800/1200/1600
- sweep command and result artifact
- summary calls and quality metrics for each candidate
- a concise interpretation that distinguishes smoke validation from production
  threshold selection

- [ ] **Step 5: Run focused verification**

Run:

```powershell
go test ./cmd/summary-dialog-gen ./cmd/eval-runner ./internal/app/rag/evaluation ./internal/app/rag/core/tokenbudget -count=1
```

Expected: PASS.

Run:

```powershell
openspec validate add-token-aware-summary-compression --strict
```

Expected:

```text
Change 'add-token-aware-summary-compression' is valid
```

Run:

```powershell
git diff --check
```

Expected: no whitespace errors; line-ending warnings are acceptable.

- [ ] **Step 6: Run repository-wide tests and classify unrelated failures**

Run:

```powershell
go test ./... -count=1
```

Expected: all affected packages pass. If the existing duplicate temporary
commands or unrelated configuration assertions still fail, capture their exact
package and error without modifying unrelated files.

- [ ] **Step 7: Commit documentation and OpenSpec evidence**

```powershell
git add testdata/evals/summary/README.md openspec/changes/add-token-aware-summary-compression/tasks.md
git commit -m "docs: record long summary threshold evaluation"
```

## Completion Checklist

- [ ] The checked-in script contains exactly 24 sequential turns.
- [ ] Generation carries the full prior conversation into every model request.
- [ ] Every successful turn is persisted before the next external call.
- [ ] Resume never regenerates completed turns.
- [ ] Token totals use the shared estimator and configured message overhead.
- [ ] The real dialogue crosses 800, 1200, and 1600 at distinct turns and ends
      at or above 2400 tokens.
- [ ] The raw fixture contains no credentials or hidden request metadata.
- [ ] Gold annotations are authored from generated dialogue evidence, not from
      the intended question script.
- [ ] The curated sample has checkpoints at 6, 12, 18, and 24 plus final eval.
- [ ] The real sweep triggers compression for all three candidates.
- [ ] Focused tests and strict OpenSpec validation pass.
