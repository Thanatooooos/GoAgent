# Token-Aware Summary Compression Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trigger incremental conversation summaries by token budget after assistant persistence, while bounding retrieve/tool context and measuring the final prompt with one estimator.

**Architecture:** A shared `tokenbudget` package becomes the only production estimation contract. The summary worker loads only messages after the latest coverage boundary and persists through a coverage-advancing write. Chat reserves stage budgets before the next request, then measures actual retrieve/tool/prompt content and degrades structurally when required.

**Tech Stack:** Go, GORM/PostgreSQL, existing RAG chat/retrieve/tool/history services, existing in-memory summary worker, OpenSpec

---

## Working-Tree Rule

The repository already contains uncommitted partial token-trigger work. Before each task, inspect `git diff` for files in scope. Preserve those edits and refactor them into this design; never reset or overwrite them.

## Task 1: Create the Shared Token Budget Package

**Files:**

- Create: `internal/app/rag/core/tokenbudget/estimator.go`
- Create: `internal/app/rag/core/tokenbudget/estimator_test.go`
- Create: `internal/app/rag/core/tokenbudget/truncate.go`
- Create: `internal/app/rag/core/tokenbudget/truncate_test.go`
- Modify: `internal/app/rag/core/history/token_estimator.go`
- Modify: `internal/app/rag/service/sessionrecall/tokens.go`
- Modify: `internal/app/rag/service/chat/imports.go`
- Modify: `internal/app/rag/service/aliases.go`
- Modify: `internal/app/rag/evaluation/summary_strategy_tokens.go`

- [ ] **Step 1: Write failing estimator tests**

```go
func TestEstimateMessagesAddsEnvelopeOverhead(t *testing.T) {
	got := EstimateMessages([]convention.ChatMessage{
		convention.UserMessage("a"),
		convention.AssistantMessage("b"),
	}, FixedEstimator(10), 4)
	if got != 28 {
		t.Fatalf("got %d, want 28", got)
	}
}

func TestApplySafetyFactorRoundsUp(t *testing.T) {
	if got := ApplySafetyFactor(101, 1.15); got != 117 {
		t.Fatalf("got %d, want 117", got)
	}
}

func TestTruncateTextRespectsBudget(t *testing.T) {
	got, truncated := TruncateText("abcdefghij", 6, RuneEstimator{})
	if !truncated || RuneEstimator{}.EstimateTokens(got) > 6 {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
}
```

- [ ] **Step 2: Verify failure**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/tokenbudget -v
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement the estimator**

```go
type Estimator interface {
	EstimateTokens(text string) int
}

type DefaultEstimator struct {
	base *tokenestimate.Estimator
}

func NewDefaultEstimator() *DefaultEstimator {
	return &DefaultEstimator{base: tokenestimate.NewEstimator()}
}

func (e *DefaultEstimator) EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if e == nil || e.base == nil {
		return tokenestimate.NewEstimator().Estimate(text)
	}
	return e.base.Estimate(text)
}

type FixedEstimator int

func (e FixedEstimator) EstimateTokens(string) int { return int(e) }

type RuneEstimator struct{}

func (RuneEstimator) EstimateTokens(text string) int {
	return utf8.RuneCountInString(strings.TrimSpace(text))
}

func EstimateMessages(messages []convention.ChatMessage, estimator Estimator, overhead int) int {
	if estimator == nil {
		estimator = NewDefaultEstimator()
	}
	if overhead < 0 {
		overhead = 0
	}
	total := 0
	for _, message := range messages {
		total += estimator.EstimateTokens(message.Content) + overhead
	}
	return total
}

func ApplySafetyFactor(tokens int, factor float64) int {
	if tokens <= 0 {
		return 0
	}
	if factor < 1 {
		factor = 1
	}
	return int(math.Ceil(float64(tokens) * factor))
}
```

- [ ] **Step 4: Implement token truncation**

```go
func TruncateText(text string, budget int, estimator Estimator) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" || budget <= 0 {
		return "", text != ""
	}
	if estimator == nil {
		estimator = NewDefaultEstimator()
	}
	if estimator.EstimateTokens(text) <= budget {
		return text, false
	}
	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		candidate := strings.TrimSpace(string(runes[:mid])) + "\n...[truncated]"
		if estimator.EstimateTokens(candidate) <= budget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	if low == 0 {
		return "", true
	}
	return strings.TrimSpace(string(runes[:low])) + "\n...[truncated]", true
}
```

- [ ] **Step 5: Delegate old estimator paths**

Expose aliases/delegating constructors from history and session recall. Replace production `RoughTokenEstimator{}` construction with `tokenbudget.NewDefaultEstimator()`. Update summary strategy accounting to use the same interface.

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/tokenbudget ./internal/app/rag/service/sessionrecall ./internal/app/rag/service/chat ./internal/app/rag/evaluation
git add internal/app/rag/core/tokenbudget internal/app/rag/core/history/token_estimator.go internal/app/rag/service/sessionrecall/tokens.go internal/app/rag/service/chat/imports.go internal/app/rag/service/aliases.go internal/app/rag/evaluation/summary_strategy_tokens.go go.mod go.sum
git commit -m "refactor(rag): unify token estimation"
```

Expected: tests PASS and one estimator contract remains in production paths.

## Task 2: Add Stage Budget Configuration

**Files:**

- Modify: `internal/framework/config/config.go`
- Modify: `configs/application.yaml`
- Modify: `internal/bootstrap/rag/runtime_summary_trigger.go`
- Modify: `internal/bootstrap/rag/runtime_summary_trigger_test.go`
- Modify: `internal/bootstrap/rag/runtime_build_chat.go`
- Modify: `internal/app/rag/service/chat/budget_context.go`

- [ ] **Step 1: Write the failing automatic-budget test**

```go
func TestComputeSummaryTriggerTokensUsesStageReserves(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.ChatContext.MaxPromptTokens = 8000
	cfg.Rag.Memory.ChatContext.FixedReserveTokens = 800
	cfg.Rag.Memory.ChatContext.SafetyReserveTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.MemoryTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.SessionRecallTokens = 1500
	cfg.Rag.Memory.ChatContext.StageBudget.RetrieveTokens = 2000
	cfg.Rag.Memory.ChatContext.StageBudget.ToolTokens = 1500
	if got := computeSummaryTriggerTokens(cfg); got != 1200 {
		t.Fatalf("got %d, want 1200", got)
	}
}
```

- [ ] **Step 2: Verify failure**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/bootstrap/rag -run TestComputeSummaryTriggerTokensUsesStageReserves -v
```

- [ ] **Step 3: Add configuration contracts**

```go
type RagSummaryTokenConfig struct {
	SafetyFactor          float64 `mapstructure:"safety-factor"`
	MessageOverheadTokens int     `mapstructure:"message-overhead-tokens"`
}

type RagChatStageBudgetConfig struct {
	MemoryTokens        int `mapstructure:"memory-tokens"`
	SessionRecallTokens int `mapstructure:"session-recall-tokens"`
	RetrieveTokens      int `mapstructure:"retrieve-tokens"`
	ToolTokens          int `mapstructure:"tool-tokens"`
}

type RagChatContextConfig struct {
	Enabled             bool                     `mapstructure:"enabled"`
	MaxPromptTokens     int                      `mapstructure:"max-prompt-tokens"`
	FixedReserveTokens  int                      `mapstructure:"fixed-reserve-tokens"`
	SafetyReserveTokens int                      `mapstructure:"safety-reserve-tokens"`
	StageBudget         RagChatStageBudgetConfig `mapstructure:"stage-budget"`
}
```

Add `SummaryToken RagSummaryTokenConfig` to `RagMemoryConfig`.

- [ ] **Step 4: Add checked-in defaults**

```yaml
summary-trigger-tokens: 0
summary-token:
  safety-factor: 1.15
  message-overhead-tokens: 4
chat-context:
  enabled: true
  max-prompt-tokens: 8000
  fixed-reserve-tokens: 800
  safety-reserve-tokens: 500
  stage-budget:
    memory-tokens: 500
    session-recall-tokens: 1500
    retrieve-tokens: 2000
    tool-tokens: 1500
```

- [ ] **Step 5: Replace history-budget derivation**

```go
func computeSummaryTriggerTokens(cfg *config.Config) int {
	if cfg == nil {
		return defaultSummaryTriggerMinHistoryBudget
	}
	if cfg.Rag.Memory.SummaryTriggerTokens > 0 {
		return cfg.Rag.Memory.SummaryTriggerTokens
	}
	ctx := cfg.Rag.Memory.ChatContext
	maxPrompt := ctx.MaxPromptTokens
	if maxPrompt <= 0 {
		maxPrompt = defaultSummaryTriggerMaxPromptTokens
	}
	reserved := ctx.FixedReserveTokens + ctx.SafetyReserveTokens +
		ctx.StageBudget.MemoryTokens + ctx.StageBudget.SessionRecallTokens +
		ctx.StageBudget.RetrieveTokens + ctx.StageBudget.ToolTokens
	if budget := maxPrompt - reserved; budget >= defaultSummaryTriggerMinHistoryBudget {
		return budget
	}
	return defaultSummaryTriggerMinHistoryBudget
}
```

- [ ] **Step 6: Thread the same budget fields into `ChatContextBudgetOptions`**

Map all values in `buildChatContextBudgetOptions`, including the shared estimator.

- [ ] **Step 7: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/framework/config ./internal/bootstrap/rag ./internal/app/rag/service/chat
git add internal/framework/config/config.go configs/application.yaml internal/bootstrap/rag/runtime_summary_trigger.go internal/bootstrap/rag/runtime_summary_trigger_test.go internal/bootstrap/rag/runtime_build_chat.go internal/app/rag/service/chat/budget_context.go
git commit -m "feat(rag): configure prompt stage token budgets"
```

## Task 3: Make Summary Compression Incremental

**Files:**

- Modify: `internal/app/rag/port/repository.go`
- Modify: `internal/adapter/repository/postgres/rag/conversation_message_repo.go`
- Modify: `internal/app/rag/core/history/summary_job.go`
- Modify: `internal/app/rag/core/history/service_store.go`
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Create: `internal/app/rag/core/history/summary_incremental_test.go`

- [ ] **Step 1: Write the failing boundary test**

```go
func TestCompressionLoadsOnlyUncoveredMessages(t *testing.T) {
	messageRepo := &recordingMessageRepo{messages: []domain.ConversationMessage{
		{ID: "11", Role: "user", Content: "new question"},
		{ID: "12", Role: "assistant", Content: "new answer"},
	}}
	summaryRepo := &recordingSummaryRepo{latestSummary: domain.ConversationSummary{
		ID: "s1", CoveredToMessageID: "10", LastMessageID: "10",
		Content: "目标：old", StructuredSummaryJSON: `{"schema_version":1,"goal":"old"}`,
	}}
	engine := compressionEngineForTest(messageRepo, summaryRepo, tokenbudget.FixedEstimator(100))
	err := engine.runConversationSummaryCompression(context.Background(), SummaryJobInput{
		ConversationID: "c1", UserID: "u1", TargetMessageID: "12",
	})
	if err != nil {
		t.Fatal(err)
	}
	if messageRepo.filter.AfterID != "10" || messageRepo.filter.ThroughID != "12" {
		t.Fatalf("filter=%+v", messageRepo.filter)
	}
	if summaryRepo.created.CoveredFromMessageID != "11" ||
		summaryRepo.created.CoveredToMessageID != "12" {
		t.Fatalf("summary=%+v", summaryRepo.created)
	}
}
```

- [ ] **Step 2: Verify failure**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/history -run TestCompressionLoadsOnlyUncoveredMessages -v
```

- [ ] **Step 3: Add an inclusive target boundary**

Add `ThroughID string` to `ConversationMessageListFilter` and map it to:

```go
if filter.ThroughID != "" {
	query = query.Where("id <= ?", filter.ThroughID)
}
```

Rename the partial `TriggerMessageID` field to:

```go
TargetMessageID string
```

- [ ] **Step 4: Replace count/turn gating with one aligned query**

```go
latest, err := e.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
if err != nil {
	return fmt.Errorf("load latest summary: %w", err)
}
tail, err := e.messageRepo.List(ctx, port.ConversationMessageListFilter{
	ConversationID: conversationID,
	UserID: userID,
	Roles: []string{string(convention.UserRole), string(convention.AssistantRole)},
	AfterID: latest.CoveredToMessageID,
	ThroughID: input.TargetMessageID,
	Order: port.ConversationMessageOrderAsc,
	Limit: 500,
})
if err != nil || len(tail) == 0 {
	return err
}
raw := e.estimator.EstimateTokens(latest.Content) +
	estimateDomainMessagesTokens(tail, e.estimator, e.messageOverheadTokens)
if tokenbudget.ApplySafetyFactor(raw, e.safetyFactor) < e.triggerTokens {
	return nil
}
```

Use `tail` as the exact fresh source for summary generation and coverage persistence. Remove `startTurns`, role counts, and fixed latest-message slicing from the trigger decision.

- [ ] **Step 5: Add below-threshold and already-covered tests**

Use fixed estimators to prove:

- a large pre-coverage history does not retrigger compression;
- a short uncovered tail remains below threshold;
- an empty uncovered tail does not call the LLM.

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/history ./internal/adapter/repository/postgres/rag
git add internal/app/rag/port/repository.go internal/adapter/repository/postgres/rag/conversation_message_repo.go internal/app/rag/core/history/summary_job.go internal/app/rag/core/history/service_store.go internal/app/rag/core/history/summary_compression.go internal/app/rag/core/history/summary_incremental_test.go
git commit -m "feat(history): compress uncovered messages by token budget"
```

## Task 4: Make Summary Jobs Idempotent

**Files:**

- Modify: `internal/app/rag/port/repository.go`
- Modify: `internal/adapter/repository/postgres/rag/conversation_summary_repo.go`
- Modify: `internal/app/rag/core/history/summary_job.go`
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Modify: `internal/app/rag/core/history/summary_job_test.go`

- [ ] **Step 1: Write failing duplicate-coverage tests**

Test that:

```go
accepted, err := repo.CreateIfCoverageAdvances(ctx, summaryWithCoverage("20"))
// accepted == true
accepted, err = repo.CreateIfCoverageAdvances(ctx, summaryWithCoverage("20"))
// accepted == false
accepted, err = repo.CreateIfCoverageAdvances(ctx, summaryWithCoverage("19"))
// accepted == false
```

Also enqueue the same `SummaryJobInput` twice and assert the runner executes once.

- [ ] **Step 2: Add repository contract**

```go
CreateIfCoverageAdvances(ctx context.Context, summary domain.ConversationSummary) (bool, error)
```

- [ ] **Step 3: Implement PostgreSQL serialization**

Within one transaction:

```go
lockKey := summary.ConversationID + ":" + summary.UserID
tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", lockKey)
```

Read the latest summary, compare distributed IDs using `math/big.Int`, skip equal/older coverage, otherwise insert.

- [ ] **Step 4: Add worker in-flight keys**

Key:

```go
conversationID + ":" + userID + ":" + targetMessageID
```

Track queued/running keys under a mutex and delete the key after execution.

- [ ] **Step 5: Persist through the advancing write**

```go
accepted, err := e.summaryRepo.CreateIfCoverageAdvances(ctx, summaryRecord)
if err != nil {
	return fmt.Errorf("save compressed summary: %w", err)
}
if !accepted {
	return nil
}
```

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/history ./internal/adapter/repository/postgres/rag
git add internal/app/rag/port/repository.go internal/adapter/repository/postgres/rag/conversation_summary_repo.go internal/app/rag/core/history/summary_job.go internal/app/rag/core/history/summary_compression.go internal/app/rag/core/history/summary_job_test.go
git commit -m "feat(history): make summary coverage writes idempotent"
```

## Task 5: Wire the Real Chat Success Path

**Files:**

- Modify: `internal/app/rag/core/history/types.go`
- Modify: `internal/app/rag/core/history/service_store.go`
- Modify: `internal/app/rag/service/chat/deps.go`
- Modify: `internal/app/rag/service/chat/execute_orchestrator.go`
- Modify: `internal/bootstrap/rag/runtime_build_conversation.go`
- Modify: `internal/bootstrap/rag/runtime_build_chat.go`
- Create: `internal/app/rag/service/chat/summary_trigger_test.go`

- [ ] **Step 1: Write failing scheduling tests**

```go
type triggerRecorder struct {
	input raghistory.SummaryJobInput
	err error
}

func (r *triggerRecorder) EnqueueSummaryCheck(_ context.Context, input raghistory.SummaryJobInput) error {
	r.input = input
	return r.err
}
```

Call `persistAssistantMessage`, then assert:

- recorder target equals returned assistant message ID;
- conversation/user IDs match;
- recorder error does not make persistence fail.

- [ ] **Step 2: Define and implement the trigger**

```go
type SummaryTrigger interface {
	EnqueueSummaryCheck(ctx context.Context, input SummaryJobInput) error
}

func (s *SummaryServiceAdapter) EnqueueSummaryCheck(ctx context.Context, input SummaryJobInput) error {
	if s == nil || s.jobEnqueuer == nil {
		return nil
	}
	return s.jobEnqueuer.EnqueueConversationSummary(ctx, input)
}
```

- [ ] **Step 3: Inject it through chat deps**

Add `SummaryTrigger raghistory.SummaryTrigger` to `RagChatDeps` and a matching field to `RagChatService`.

- [ ] **Step 4: Schedule after assistant persistence**

```go
if s.summaryTrigger != nil {
	err := s.summaryTrigger.EnqueueSummaryCheck(context.WithoutCancel(ctx), raghistory.SummaryJobInput{
		ConversationID: state.meta.ConversationID,
		UserID: input.UserID,
		TargetMessageID: created.ID,
		RebuildReason: "token_threshold_reached",
	})
	if err != nil {
		log.FromContext(ctx).Warnw("enqueue summary check failed", "error", err)
	}
}
```

Do not return this error.

- [ ] **Step 5: Always construct the asynchronous worker when summary is enabled**

Call `compressible.EnableAsyncSummaryJobs(32)` during conversation bundle construction and pass the adapter as the chat trigger. Production chat must not synchronously generate summaries.

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/service/chat ./internal/bootstrap/rag
git add internal/app/rag/core/history/types.go internal/app/rag/core/history/service_store.go internal/app/rag/service/chat/deps.go internal/app/rag/service/chat/execute_orchestrator.go internal/bootstrap/rag/runtime_build_conversation.go internal/bootstrap/rag/runtime_build_chat.go internal/app/rag/service/chat/summary_trigger_test.go
git commit -m "feat(chat): enqueue summary checks after assistant persistence"
```

## Task 6: Bound Retrieve and Tool Context

**Files:**

- Create: `internal/app/rag/core/retrieve/context_budget.go`
- Create: `internal/app/rag/core/retrieve/context_budget_test.go`
- Modify: `internal/app/rag/tool/runtime/renderer.go`
- Modify: `internal/app/rag/tool/tool_test.go`
- Modify: `internal/app/rag/service/chat/execute_tool_workflow.go`

- [ ] **Step 1: Write failing retrieve test**

Build two ranked chunks with a budget that fits only the first. Assert the first remains, the second is absent, and stats report truncation.

- [ ] **Step 2: Implement ranked chunk assembly**

```go
type ContextBudgetStats struct {
	CandidateChunks int
	RetainedChunks int
	TokensBefore int
	TokensAfter int
	Truncated bool
}
```

Iterate in rank order, append complete chunks while they fit, truncate the final chunk with `tokenbudget.TruncateText`, preserve `[n]` and section metadata, then stop.

- [ ] **Step 3: Write failing tool section test**

Supply conclusion, source, and verbose detail sections. Use a budget that keeps conclusion/source but drops detail.

- [ ] **Step 4: Implement budget-aware tool rendering**

Add:

```go
func RenderContextWithinBudget(
	results []Result,
	budget int,
	estimator tokenbudget.Estimator,
) (string, tokenbudget.TruncationStats)
```

Priorities:

- summary/conclusion: required, 100
- URLs/sources: required, 90
- diagnostic evidence: 70
- fetched raw text: 30
- verbose detail: 10

Keep `maxRenderedToolContextLen = 12000` as a final hard cap.

- [ ] **Step 5: Apply both budgets before prompt construction**

Rebuild `retrieveResult.KnowledgeContext` from ranked chunks using `RetrieveTokens`. Normalize workflow `result.Context` using `ToolTokens`. Attach before/after token stats to trace metadata.

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/retrieve ./internal/app/rag/tool/... ./internal/app/rag/service/chat
git add internal/app/rag/core/retrieve/context_budget.go internal/app/rag/core/retrieve/context_budget_test.go internal/app/rag/tool/runtime/renderer.go internal/app/rag/tool/tool_test.go internal/app/rag/service/chat/execute_tool_workflow.go
git commit -m "feat(rag): cap retrieve and tool context by tokens"
```

## Task 7: Recalculate the Actual Prompt and Report Stage Tokens

**Files:**

- Modify: `internal/app/rag/service/chat/budget_context.go`
- Modify: `internal/app/rag/service/chat/chat_context_budget_test.go`
- Modify: `internal/app/rag/service/chat/stage_types.go`

- [ ] **Step 1: Write failing stage-breakdown test**

Build a prompt context containing summary/history, memory, session, retrieve, and tool text. Assert non-zero values for each stage and assert total equals the estimate of `promptService.BuildMessages`.

- [ ] **Step 2: Add result contract**

```go
type ChatContextStageTokens struct {
	Fixed int `json:"fixed"`
	History int `json:"history"`
	Memory int `json:"memory"`
	Session int `json:"session"`
	Retrieve int `json:"retrieve"`
	Tool int `json:"tool"`
	Total int `json:"total"`
}
```

Add `StageTokens ChatContextStageTokens` to `ChatContextBudgetResult`.

- [ ] **Step 3: Measure actual stage outputs**

Calculate component estimates from the actual context strings, but calculate `Total` from built messages so prompt wrappers are included.

- [ ] **Step 4: Preserve deterministic degradation**

Rebuild and re-estimate after each action:

1. remove oldest unpinned history;
2. shrink tool;
3. shrink retrieve;
4. shrink session recall;
5. shrink long-term memory.

Never remove the latest summary, current question, required system prompt, or policy.

- [ ] **Step 5: Emit trace breakdown**

```go
extra["stageTokens"] = result.StageTokens
extra["estimatedPromptTokens"] = result.StageTokens.Total
```

Keep existing degradation fields.

- [ ] **Step 6: Verify and commit**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/service/chat -run 'TestApplyChatContextBudget|TestRunPromptStage|TestChatContextBudgetTrace' -v
git add internal/app/rag/service/chat/budget_context.go internal/app/rag/service/chat/chat_context_budget_test.go internal/app/rag/service/chat/stage_types.go
git commit -m "feat(chat): report actual prompt stage token usage"
```

## Task 8: Prove Delivery and Close the OpenSpec

**Files:**

- Modify: `internal/app/rag/service/chat/summary_trigger_test.go`
- Modify: `internal/app/rag/core/history/compression_integration_test.go`
- Modify: `openspec/changes/add-token-aware-summary-compression/tasks.md`
- Modify: `docs/project_progress_context.md`

- [ ] **Step 1: Add production-path integration coverage**

Use real `persistAssistantMessage`, real `SummaryServiceAdapter`, and real worker with in-memory repositories. Configure threshold `1`, persist an assistant response, wait at most two seconds, and assert:

- assistant persistence returned before waiting;
- one summary was accepted;
- summary coverage ends at the assistant message ID;
- a duplicate job does not create a second effective summary.

- [ ] **Step 2: Run targeted verification**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./internal/app/rag/core/tokenbudget ./internal/app/rag/core/history ./internal/app/rag/core/retrieve ./internal/app/rag/tool/... ./internal/app/rag/service/sessionrecall ./internal/app/rag/service/chat ./internal/bootstrap/rag ./internal/app/rag/evaluation
```

Expected: PASS.

- [ ] **Step 3: Run repository verification**

```powershell
$env:GOCACHE='D:\goagent\.gocache'
go test ./...
go vet ./...
```

Expected: PASS. Record exact unrelated pre-existing failures if any, while requiring all changed packages to pass.

- [ ] **Step 4: Validate OpenSpec**

```powershell
openspec validate add-token-aware-summary-compression --strict
```

Expected:

```text
Change 'add-token-aware-summary-compression' is valid
```

- [ ] **Step 5: Perform delivery audit**

Confirm:

- module: estimator, budget, boundary, truncation, and idempotency tests pass;
- integration: assistant persistence schedules the worker;
- delivery: checked-in defaults run and trace exposes stage tokens.

Only after all three hold, check completed tasks in the OpenSpec.

- [ ] **Step 6: Update project context and commit**

Document token-triggered async incremental summary, retrieve/tool budgets, and prompt stage token reporting.

```powershell
git add openspec/changes/add-token-aware-summary-compression/tasks.md docs/project_progress_context.md internal/app/rag/service/chat/summary_trigger_test.go internal/app/rag/core/history/compression_integration_test.go
git commit -m "test(rag): verify token-aware summary delivery"
```

## Spec Coverage

- Shared estimator, overhead, and safety factor: Task 1.
- Stage-reserved automatic history budget: Task 2.
- Incremental coverage boundary and token trigger: Task 3.
- Concurrent/duplicate coverage safety: Task 4.
- Async real chat wiring and fail-open behavior: Task 5.
- Retrieve/tool token limits and hard cap: Task 6.
- Actual final prompt calculation and degradation: Task 7.
- Module/integration/delivery proof and OpenSpec reconciliation: Task 8.

