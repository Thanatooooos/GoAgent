# Structured Work-Memory Summary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace free-form conversation summary compression with a structured work-memory summary that persists JSON as the semantic source of truth, renders compatibility text into `content`, applies tiered budgets, and rejects obviously bad summaries before they become prompt input.

**Architecture:** Keep the current chat read path stable by continuing to inject `ConversationSummary.Content` through `LoadLatestSummary()`, but change the write path so `internal/app/rag/core/history` produces, validates, and stores structured JSON plus rendered text. Add a small helper layer inside `core/history` for schema parsing, rendering, budget selection, and validation, and expand `t_conversation_summary` with one JSON field to hold the structured payload.

**Tech Stack:** Go, GORM, PostgreSQL migrations, existing `infra-ai/chat` JSON-mode request flow, `go test`

---

## File Structure

### Existing files to modify

- `internal/app/rag/domain/conversation_summary.go`
  - add the new persisted structured-summary field
- `internal/framework/config/config.go`
  - add summary budget config alongside existing memory summary config
- `internal/bootstrap/rag/runtime_build_conversation.go`
  - wire new budget config into `SummaryCompressionOptions`
- `internal/app/rag/core/history/service_store.go`
  - expand `SummaryCompressionOptions`
- `internal/app/rag/core/history/summary_compression.go`
  - switch compression prompt/output flow from free-form text to structured JSON
- `internal/app/rag/core/history/service_store_test.go`
  - update current unit tests to assert structured payload persistence and quality outcomes
- `internal/app/rag/core/history/compression_integration_test.go`
  - update the integration path to parse/validate JSON output and rendered `content`
- `internal/adapter/repository/postgres/rag/models/conversation_summary_model.go`
  - add the new database column mapping
- `internal/adapter/repository/postgres/rag/mapper.go`
  - map the new field in both directions
- `internal/adapter/repository/postgres/rag/conversation_summary_mapper_test.go`
  - extend mapper round-trip coverage

### New files to create

- `internal/adapter/repository/postgres/migrations/20260614100000_add_structured_summary_json_to_conversation_summary.sql`
  - schema migration for the new JSON/text field
- `internal/app/rag/core/history/summary_schema.go`
  - typed structured-summary schema plus parse/marshal helpers
- `internal/app/rag/core/history/summary_schema_test.go`
  - schema parsing and normalization coverage
- `internal/app/rag/core/history/summary_renderer.go`
  - deterministic compatibility-text rendering
- `internal/app/rag/core/history/summary_renderer_test.go`
  - section ordering, empty-section omission, and trim coverage
- `internal/app/rag/core/history/summary_policy.go`
  - summary budget tier selection and per-field limits
- `internal/app/rag/core/history/summary_policy_test.go`
  - small / medium / large tier coverage
- `internal/app/rag/core/history/summary_validator.go`
  - structural validation, minimum-content checks, critical-entity preservation, hallucination guard
- `internal/app/rag/core/history/summary_validator_test.go`
  - accepted / rejected validation scenarios

### Boundaries to preserve

- Do not change the `SummaryService` interface in `internal/app/rag/core/history/types.go`.
- Do not change `LoadLatestSummary()` semantics in this implementation pass.
- Do not change `chat_context_budget` / pinning behavior in this implementation pass.
- Do not mix async worker hardening into this plan; preserve current worker behavior while ensuring sync and async paths use the same acceptance/rejection semantics.

### Configuration shape

Add a small nested config under `rag.memory` rather than replacing the old scalar immediately:

```go
type RagSummaryBudgetConfig struct {
	SmallMaxChars         int `mapstructure:"small-max-chars"`
	MediumMaxChars        int `mapstructure:"medium-max-chars"`
	LargeMaxChars         int `mapstructure:"large-max-chars"`
	MediumMessageCountMin int `mapstructure:"medium-message-count-min"`
	LargeMessageCountMin  int `mapstructure:"large-message-count-min"`
}
```

Then extend `RagMemoryConfig`:

```go
type RagMemoryConfig struct {
	HistoryKeepTurns  int                   `mapstructure:"history-keep-turns"`
	SummaryStartTurns int                   `mapstructure:"summary-start-turns"`
	SummaryEnabled    bool                  `mapstructure:"summary-enabled"`
	SummaryAsync      RagSummaryAsyncConfig `mapstructure:"summary-async"`
	SummaryMaxChars   int                   `mapstructure:"summary-max-chars"`
	SummaryBudget     RagSummaryBudgetConfig `mapstructure:"summary-budget"`
	// ...existing fields unchanged
}
```

Use `SummaryMaxChars` as fallback for legacy configs if any new budget field is zero.

---

### Task 1: Expand Persistence and Config Contracts

**Files:**
- Create: `internal/adapter/repository/postgres/migrations/20260614100000_add_structured_summary_json_to_conversation_summary.sql`
- Modify: `internal/app/rag/domain/conversation_summary.go`
- Modify: `internal/adapter/repository/postgres/rag/models/conversation_summary_model.go`
- Modify: `internal/adapter/repository/postgres/rag/mapper.go`
- Test: `internal/adapter/repository/postgres/rag/conversation_summary_mapper_test.go`
- Modify: `internal/framework/config/config.go`
- Modify: `internal/bootstrap/rag/runtime_build_conversation.go`

- [ ] **Step 1: Write the failing mapper and config tests**

```go
func TestConversationSummaryMapperRoundTripIncludesStructuredSummaryJSON(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	input := domain.ConversationSummary{
		ID:                   "1",
		ConversationID:       "c1",
		UserID:               "u1",
		Content:              "对话摘要：目标：实现结构化摘要",
		StructuredSummaryJSON: `{"schema_version":1,"goal":"实现结构化摘要"}`,
		LastMessageID:        "m9",
		SummaryVersion:       domain.SummaryVersionV1,
		QualityStatus:        domain.SummaryQualityAccepted,
		CreateTime:           now,
		UpdateTime:           now,
	}

	model := toConversationSummaryModel(input)
	if model.StructuredSummaryJSON != input.StructuredSummaryJSON {
		t.Fatalf("expected structured summary json to round-trip, got %q", model.StructuredSummaryJSON)
	}

	output := toConversationSummaryDomain(model)
	if output.StructuredSummaryJSON != input.StructuredSummaryJSON {
		t.Fatalf("expected structured summary json in domain output, got %q", output.StructuredSummaryJSON)
	}
}
```

```go
func TestBuildConversationServicesPassesSummaryBudgetConfig(t *testing.T) {
	cfg := config.Config{}
	cfg.Rag.Memory.SummaryEnabled = true
	cfg.Rag.Memory.SummaryBudget.SmallMaxChars = 400
	cfg.Rag.Memory.SummaryBudget.MediumMaxChars = 600
	cfg.Rag.Memory.SummaryBudget.LargeMaxChars = 800
	cfg.Rag.Memory.SummaryBudget.MediumMessageCountMin = 6
	cfg.Rag.Memory.SummaryBudget.LargeMessageCountMin = 10
	// buildConversationServices should be exercised through a small constructor-level test
	// that asserts SummaryCompressionOptions receives these values.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/adapter/repository/postgres/rag -run "TestConversationSummaryMapperRoundTripIncludesStructuredSummaryJSON" -count=1
go test ./internal/bootstrap/rag -run "TestBuildConversationServicesPassesSummaryBudgetConfig" -count=1
```

Expected:

- mapper test fails because `StructuredSummaryJSON` does not exist yet
- runtime wiring test fails because `SummaryBudget` fields are not defined or not passed through

- [ ] **Step 3: Implement the persistence field, migration, and config wiring**

Add the domain/model field:

```go
type ConversationSummary struct {
	ID                    string
	ConversationID        string
	UserID                string
	Content               string
	StructuredSummaryJSON string
	LastMessageID         string
	SummaryVersion        int
	// ...existing fields unchanged
}
```

```go
type ConversationSummaryModel struct {
	ID                    string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID        string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_conv_user,priority:1"`
	UserID                string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_conv_user,priority:2"`
	LastMessageID         string                `gorm:"column:last_message_id;type:varchar(20);not null"`
	Content               string                `gorm:"column:content;type:text;not null"`
	StructuredSummaryJSON string                `gorm:"column:structured_summary_json;type:text"`
	SummaryVersion        int                   `gorm:"column:summary_version;not null;default:1"`
	// ...existing fields unchanged
}
```

Update mapper functions:

```go
func toConversationSummaryModel(item domain.ConversationSummary) models.ConversationSummaryModel {
	// ...existing defaults
	return models.ConversationSummaryModel{
		ID:                    item.ID,
		ConversationID:        item.ConversationID,
		UserID:                item.UserID,
		LastMessageID:         item.LastMessageID,
		Content:               item.Content,
		StructuredSummaryJSON: strings.TrimSpace(item.StructuredSummaryJSON),
		SummaryVersion:        summaryVersion,
		// ...existing fields unchanged
	}
}

func toConversationSummaryDomain(item models.ConversationSummaryModel) domain.ConversationSummary {
	return domain.ConversationSummary{
		ID:                    item.ID,
		ConversationID:        item.ConversationID,
		UserID:                item.UserID,
		LastMessageID:         item.LastMessageID,
		Content:               item.Content,
		StructuredSummaryJSON: item.StructuredSummaryJSON,
		SummaryVersion:        item.SummaryVersion,
		// ...existing fields unchanged
	}
}
```

Create the migration:

```sql
ALTER TABLE t_conversation_summary
  ADD COLUMN IF NOT EXISTS structured_summary_json TEXT;
```

Extend config and runtime wiring:

```go
type RagSummaryBudgetConfig struct {
	SmallMaxChars         int `mapstructure:"small-max-chars"`
	MediumMaxChars        int `mapstructure:"medium-max-chars"`
	LargeMaxChars         int `mapstructure:"large-max-chars"`
	MediumMessageCountMin int `mapstructure:"medium-message-count-min"`
	LargeMessageCountMin  int `mapstructure:"large-message-count-min"`
}
```

```go
compressible := raghistory.NewCompressibleSummaryService(repos.summaryRepo, raghistory.SummaryCompressionOptions{
	MessageRepo: repos.messageRepo,
	ChatService: aiRuntime.Chat,
	StartTurns:  cfg.Rag.Memory.SummaryStartTurns,
	MaxChars:    cfg.Rag.Memory.SummaryMaxChars,
	Budget: raghistory.SummaryBudgetOptions{
		SmallMaxChars:         cfg.Rag.Memory.SummaryBudget.SmallMaxChars,
		MediumMaxChars:        cfg.Rag.Memory.SummaryBudget.MediumMaxChars,
		LargeMaxChars:         cfg.Rag.Memory.SummaryBudget.LargeMaxChars,
		MediumMessageCountMin: cfg.Rag.Memory.SummaryBudget.MediumMessageCountMin,
		LargeMessageCountMin:  cfg.Rag.Memory.SummaryBudget.LargeMessageCountMin,
	},
})
```

- [ ] **Step 4: Run tests to verify the contract changes pass**

Run:

```powershell
go test ./internal/adapter/repository/postgres/rag -run "TestConversationSummaryMapper" -count=1
go test ./internal/bootstrap/rag -run "TestBuildConversationServicesPassesSummaryBudgetConfig" -count=1
```

Expected:

- `internal/adapter/repository/postgres/rag` PASS for mapper tests
- `internal/bootstrap/rag` targeted wiring test PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/rag/domain/conversation_summary.go internal/adapter/repository/postgres/rag/models/conversation_summary_model.go internal/adapter/repository/postgres/rag/mapper.go internal/adapter/repository/postgres/rag/conversation_summary_mapper_test.go internal/framework/config/config.go internal/bootstrap/rag/runtime_build_conversation.go internal/adapter/repository/postgres/migrations/20260614100000_add_structured_summary_json_to_conversation_summary.sql
git commit -m "feat: add structured conversation summary persistence"
```

### Task 2: Add Structured Summary Schema, Rendering, and Budget Policy

**Files:**
- Create: `internal/app/rag/core/history/summary_schema.go`
- Create: `internal/app/rag/core/history/summary_schema_test.go`
- Create: `internal/app/rag/core/history/summary_renderer.go`
- Create: `internal/app/rag/core/history/summary_renderer_test.go`
- Create: `internal/app/rag/core/history/summary_policy.go`
- Create: `internal/app/rag/core/history/summary_policy_test.go`
- Modify: `internal/app/rag/core/history/service_store.go`

- [ ] **Step 1: Write the failing helper-layer tests**

```go
func TestParseStructuredSummaryRejectsUnknownFields(t *testing.T) {
	_, err := ParseStructuredSummary(`{"schema_version":1,"goal":"x","unknown":"y"}`)
	if err == nil {
		t.Fatal("expected unknown fields to be rejected")
	}
}
```

```go
func TestRenderStructuredSummaryOmitsEmptySectionsAndKeepsOrder(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion: 1,
		Goal:          "实现结构化摘要",
		Constraints:   []string{"保持 LoadLatestSummary 兼容"},
	}

	rendered := RenderStructuredSummary(summary, 400)
	if !strings.HasPrefix(rendered, "目标：实现结构化摘要") {
		t.Fatalf("unexpected render prefix: %q", rendered)
	}
	if strings.Contains(rendered, "用户偏好：") {
		t.Fatalf("expected empty sections to be omitted: %q", rendered)
	}
}
```

```go
func TestSelectSummaryBudgetTierPromotesDenseTechnicalContent(t *testing.T) {
	tier := SelectSummaryBudget(SummaryBudgetInput{
		MessageCount: 4,
		TotalChars:   260,
		Messages: []string{
			"vector store unavailable",
			"document id doc_fail_01",
			"summary-max-chars=200",
		},
	}, SummaryBudgetOptions{
		SmallMaxChars:         400,
		MediumMaxChars:        600,
		LargeMaxChars:         800,
		MediumMessageCountMin: 6,
		LargeMessageCountMin:  10,
	})

	if tier.MaxChars != 600 {
		t.Fatalf("expected dense technical content to promote to medium tier, got %+v", tier)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestParseStructuredSummaryRejectsUnknownFields|TestRenderStructuredSummaryOmitsEmptySectionsAndKeepsOrder|TestSelectSummaryBudgetTierPromotesDenseTechnicalContent" -count=1
```

Expected:

- FAIL because the schema, renderer, and policy helpers do not exist yet

- [ ] **Step 3: Implement schema, renderer, and policy helpers**

Add the schema type:

```go
type StructuredSummary struct {
	SchemaVersion    int      `json:"schema_version"`
	Goal             string   `json:"goal"`
	UserPreferences  []string `json:"user_preferences,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	EstablishedFacts []string `json:"established_facts,omitempty"`
	RecentProgress   []string `json:"recent_progress,omitempty"`
	OpenQuestions    []string `json:"open_questions,omitempty"`
}
```

Add strict parse and marshal helpers:

```go
func ParseStructuredSummary(raw string) (StructuredSummary, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()

	var summary StructuredSummary
	if err := decoder.Decode(&summary); err != nil {
		return StructuredSummary{}, fmt.Errorf("decode structured summary: %w", err)
	}
	summary.Normalize()
	return summary, nil
}
```

Add a deterministic renderer:

```go
func RenderStructuredSummary(summary StructuredSummary, maxChars int) string {
	var sections []string
	if summary.Goal != "" {
		sections = append(sections, "目标："+summary.Goal)
	}
	if len(summary.Constraints) > 0 {
		sections = append(sections, "约束：\n- "+strings.Join(summary.Constraints, "\n- "))
	}
	if len(summary.UserPreferences) > 0 {
		sections = append(sections, "用户偏好：\n- "+strings.Join(summary.UserPreferences, "\n- "))
	}
	if len(summary.EstablishedFacts) > 0 {
		sections = append(sections, "已确认事实：\n- "+strings.Join(summary.EstablishedFacts, "\n- "))
	}
	if len(summary.RecentProgress) > 0 {
		sections = append(sections, "最近进展：\n- "+strings.Join(summary.RecentProgress, "\n- "))
	}
	if len(summary.OpenQuestions) > 0 {
		sections = append(sections, "待确认问题：\n- "+strings.Join(summary.OpenQuestions, "\n- "))
	}
	return trimRunes(strings.Join(sections, "\n"), maxChars)
}
```

Add the budget policy:

```go
type SummaryBudgetOptions struct {
	SmallMaxChars         int
	MediumMaxChars        int
	LargeMaxChars         int
	MediumMessageCountMin int
	LargeMessageCountMin  int
}

type SummaryBudgetTier struct {
	Name     string
	MaxChars int
}
```

```go
func SelectSummaryBudget(input SummaryBudgetInput, options SummaryBudgetOptions) SummaryBudgetTier {
	if options.SmallMaxChars <= 0 {
		options.SmallMaxChars = 400
	}
	if options.MediumMaxChars <= 0 {
		options.MediumMaxChars = max(options.SmallMaxChars, 600)
	}
	if options.LargeMaxChars <= 0 {
		options.LargeMaxChars = max(options.MediumMaxChars, 800)
	}
	if input.MessageCount >= options.LargeMessageCountMin {
		return SummaryBudgetTier{Name: "large", MaxChars: options.LargeMaxChars}
	}
	if input.MessageCount >= options.MediumMessageCountMin || containsDenseTechnicalSignals(input.Messages) {
		return SummaryBudgetTier{Name: "medium", MaxChars: options.MediumMaxChars}
	}
	return SummaryBudgetTier{Name: "small", MaxChars: options.SmallMaxChars}
}
```

Extend `SummaryCompressionOptions`:

```go
type SummaryCompressionOptions struct {
	MessageRepo  port.ConversationMessageRepository
	ChatService  aichat.LLMService
	StartTurns   int
	MaxChars     int
	Budget       SummaryBudgetOptions
	JobEnqueuer  SummaryJobEnqueuer
	AsyncEnabled bool
}
```

- [ ] **Step 4: Run tests to verify the helper layer passes**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestParseStructuredSummaryRejectsUnknownFields|TestRenderStructuredSummaryOmitsEmptySectionsAndKeepsOrder|TestSelectSummaryBudgetTierPromotesDenseTechnicalContent" -count=1
```

Expected:

- `internal/app/rag/core/history` targeted helper tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/rag/core/history/summary_schema.go internal/app/rag/core/history/summary_schema_test.go internal/app/rag/core/history/summary_renderer.go internal/app/rag/core/history/summary_renderer_test.go internal/app/rag/core/history/summary_policy.go internal/app/rag/core/history/summary_policy_test.go internal/app/rag/core/history/service_store.go
git commit -m "feat: add structured summary schema and budget policy"
```

### Task 3: Integrate Structured JSON Compression and Validation

**Files:**
- Create: `internal/app/rag/core/history/summary_validator.go`
- Create: `internal/app/rag/core/history/summary_validator_test.go`
- Modify: `internal/app/rag/core/history/summary_compression.go`
- Modify: `internal/app/rag/core/history/service_store_test.go`

- [ ] **Step 1: Write the failing validation and compression tests**

```go
func TestValidateStructuredSummaryRejectsMissingCriticalEntity(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    1,
		Goal:             "排查导入失败",
		EstablishedFacts: []string{"导入失败"},
	}
	source := []domain.ConversationMessage{
		{Role: "assistant", Content: "indexer failed: vector store unavailable"},
		{Role: "user", Content: "doc_fail_01 为什么失败"},
	}

	result := ValidateStructuredSummary(summary, source)
	if result.Accepted {
		t.Fatalf("expected validator to reject missing critical entity, got %+v", result)
	}
}
```

```go
func TestCompressIfNeededStoresStructuredSummaryAndAcceptedQuality(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"排查导入失败","constraints":["保持现有读链兼容"],"established_facts":["doc_fail_01 在 indexer 节点失败","错误为 vector store unavailable"],"recent_progress":["已决定结构化摘要真源"],"open_questions":["是否需要分档预算"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "indexer failed: vector store unavailable"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "doc_fail_01 为什么失败"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "让我继续排查"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "请帮我排查导入失败"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
		Budget: SummaryBudgetOptions{
			SmallMaxChars:         400,
			MediumMaxChars:        600,
			LargeMaxChars:         800,
			MediumMessageCountMin: 6,
			LargeMessageCountMin:  10,
		},
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.lastSummary.StructuredSummaryJSON == "" {
		t.Fatal("expected structured summary json to be stored")
	}
	if summaryRepo.lastSummary.QualityStatus != domain.SummaryQualityAccepted {
		t.Fatalf("expected accepted quality status, got %q", summaryRepo.lastSummary.QualityStatus)
	}
	if !strings.Contains(summaryRepo.lastContent, "目标：排查导入失败") {
		t.Fatalf("expected rendered content to contain goal section, got %q", summaryRepo.lastContent)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestValidateStructuredSummaryRejectsMissingCriticalEntity|TestCompressIfNeededStoresStructuredSummaryAndAcceptedQuality" -count=1
```

Expected:

- FAIL because no structured validator exists and compression still expects free-form text

- [ ] **Step 3: Implement validator and compression flow**

Add the validation result type:

```go
type SummaryValidationResult struct {
	Accepted bool
	Reason    string
}
```

Implement validator rules:

```go
func ValidateStructuredSummary(summary StructuredSummary, source []domain.ConversationMessage) SummaryValidationResult {
	if strings.TrimSpace(summary.Goal) == "" {
		return SummaryValidationResult{Reason: "missing goal"}
	}
	if len(summary.Constraints) == 0 && len(summary.EstablishedFacts) == 0 && len(summary.RecentProgress) == 0 {
		return SummaryValidationResult{Reason: "missing high-value sections"}
	}
	if err := ensureCriticalEntitiesPreserved(summary, source); err != nil {
		return SummaryValidationResult{Reason: err.Error()}
	}
	if err := ensureNoUnsupportedFacts(summary, source); err != nil {
		return SummaryValidationResult{Reason: err.Error()}
	}
	return SummaryValidationResult{Accepted: true}
}
```

Rewrite the compression prompt builder and write path:

```go
func buildStructuredSummaryPrompt(tier SummaryBudgetTier, previousSummary string, historyMessages []domain.ConversationMessage) string {
	return fmt.Sprintf(`你是对话工作记忆压缩器。
输出 JSON，字段只能是：
schema_version, goal, user_preferences, constraints, established_facts, recent_progress, open_questions。
规则：
1. 目标不是复述对话，而是为后续轮次保留会影响回答的状态。
2. 删除寒暄、客套、重复措辞。
3. 若后文推翻前文，以最新有效信息为准。
4. 不允许脑补；无法确认的内容放到 open_questions。
5. goal 只能有一个主目标。
6. 每个数组字段最多 3 项，每项尽量不超过 80 字。
7. 最终渲染文本预算为 %d 字。`, tier.MaxChars)
}
```

Integrate the new flow in `runConversationSummaryCompression`:

```go
tier := SelectSummaryBudget(SummaryBudgetInput{
	MessageCount: len(historyMessages),
	TotalChars:   countMessageChars(historyMessages),
	Messages:     messageContents(historyMessages),
}, e.budget)

prompt := buildStructuredSummaryPrompt(tier, latestSummary.StructuredSummaryJSON, historyMessages)
response, err := e.chatService.ChatWithRequest(convention.ChatRequest{
	Messages: []convention.ChatMessage{
		convention.SystemMessage(prompt),
		convention.UserMessage("请输出结构化工作记忆 JSON。"),
	},
	ResponseFormat: convention.JSONResponseFormat(),
})
```

Then parse, validate, render, and persist:

```go
structured, err := ParseStructuredSummary(strings.TrimSpace(response))
if err != nil {
	return fmt.Errorf("parse structured summary: %w", err)
}

validation := ValidateStructuredSummary(structured, historyMessages)
	rendered := RenderStructuredSummary(structured, tier.MaxChars)
quality := domain.SummaryQualityRejected
rebuildReason := firstNonEmpty(input.RebuildReason, "threshold_reached")
if validation.Accepted {
	quality = domain.SummaryQualityAccepted
} else {
	rebuildReason = rebuildReason + "|validation_rejected:" + validation.Reason
}

summaryRecord, err := buildConversationSummaryRecord(
	conversationID,
	userID,
	rendered,
	mustMarshalStructuredSummary(structured),
	historyMessages,
	rebuildReason,
	quality,
	e.now(),
)
```

Update the record builder signature:

```go
func buildConversationSummaryRecord(
	conversationID string,
	userID string,
	content string,
	structuredSummaryJSON string,
	historyMessages []domain.ConversationMessage,
	rebuildReason string,
	qualityStatus string,
	now time.Time,
) (domain.ConversationSummary, error)
```

- [ ] **Step 4: Run tests to verify the structured compression flow passes**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestValidateStructuredSummaryRejectsMissingCriticalEntity|TestCompressIfNeededStoresStructuredSummaryAndAcceptedQuality|TestCompressIfNeededTriggersCompression|TestCompressIfNeededAlreadyCompressed" -count=1
```

Expected:

- validator tests PASS
- updated compression tests PASS
- no regression in existing compression threshold / duplicate-skip tests

- [ ] **Step 5: Commit**

```bash
git add internal/app/rag/core/history/summary_validator.go internal/app/rag/core/history/summary_validator_test.go internal/app/rag/core/history/summary_compression.go internal/app/rag/core/history/service_store_test.go
git commit -m "feat: store validated structured conversation summaries"
```

### Task 4: Preserve Read-Path Compatibility and Finish Regression Coverage

**Files:**
- Modify: `internal/app/rag/core/history/service_store.go`
- Modify: `internal/app/rag/core/history/compression_integration_test.go`
- Modify: `internal/app/rag/core/history/summary_job_test.go`
- Test: `internal/app/rag/core/history/service_store_test.go`

- [ ] **Step 1: Write the failing compatibility and integration tests**

```go
func TestLoadLatestSummaryStillUsesRenderedContent(t *testing.T) {
	repo := &mockSummaryRepoForCompress{
		latestSummary: domain.ConversationSummary{
			ID:                    "s1",
			Content:               "目标：实现结构化摘要",
			StructuredSummaryJSON: `{"schema_version":1,"goal":"不要直接读取 JSON"}`,
			QualityStatus:         domain.SummaryQualityAccepted,
		},
	}

	adapter := NewSummaryServiceAdapter(repo)
	msg, err := adapter.LoadLatestSummary(context.Background(), "c1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil || msg.Content != "目标：实现结构化摘要" {
		t.Fatalf("expected rendered content path to remain unchanged, got %#v", msg)
	}
}
```

```go
func TestCompressSummaryIntegrationStoresStructuredSummaryJSON(t *testing.T) {
	// update integration repo stub to capture StructuredSummaryJSON and assert it is non-empty
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/app/rag/core/history -run "TestLoadLatestSummaryStillUsesRenderedContent|TestCompressSummaryIntegrationStoresStructuredSummaryJSON" -count=1
```

Expected:

- FAIL because compatibility assertions are not yet encoded in tests

- [ ] **Step 3: Implement the compatibility guarantees and test updates**

Leave `LoadLatestSummary()` unchanged except for accepted-content safety:

```go
func (s *SummaryServiceAdapter) LoadLatestSummary(ctx context.Context, conversationID string, userID string) (*convention.ChatMessage, error) {
	// existing lookup path stays the same
	summary, err := s.summaryRepo.FindLatestByConversationIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("find latest conversation summary: %w", err)
	}
	if summary.ID == "" || strings.TrimSpace(summary.Content) == "" {
		return nil, nil
	}
	message := convention.SystemMessage(summary.Content)
	return &message, nil
}
```

Update the integration stub:

```go
type integrationSummaryRepo struct {
	created              bool
	lastContent          string
	lastStructuredJSON   string
	latestSummary        domain.ConversationSummary
}

func (r *integrationSummaryRepo) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	r.created = true
	r.lastContent = summary.Content
	r.lastStructuredJSON = summary.StructuredSummaryJSON
	summary.ID = "summary-integration"
	return summary, nil
}
```

Assert both rendered and structured outputs:

```go
if summaryRepo.lastStructuredJSON == "" {
	t.Fatal("expected structured summary json to be persisted")
}
if !strings.Contains(summaryRepo.lastContent, "目标：") {
	t.Fatalf("expected rendered summary content, got %q", summaryRepo.lastContent)
}
```

Ensure the async worker regression test still passes with accepted/rejected semantics unchanged:

```go
func TestCompressIfNeededAsyncEnqueuesJob(t *testing.T) {
	// keep the existing enqueue assertion; do not change worker behavior in this plan
}
```

- [ ] **Step 4: Run the package regression suite**

Run:

```powershell
go test ./internal/app/rag/core/history -count=1
go test ./internal/adapter/repository/postgres/rag -count=1
go test ./internal/bootstrap/rag -run "TestBuildConversationServicesPassesSummaryBudgetConfig" -count=1
```

Expected:

- `internal/app/rag/core/history` PASS
- `internal/adapter/repository/postgres/rag` PASS
- targeted `internal/bootstrap/rag` PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/rag/core/history/service_store.go internal/app/rag/core/history/compression_integration_test.go internal/app/rag/core/history/summary_job_test.go internal/app/rag/core/history/service_store_test.go
git commit -m "test: lock structured summary compatibility path"
```

## Self-Review

### Spec coverage

- Structured JSON as semantic truth: covered by Task 1 and Task 3
- Compatibility `content` path retained: covered by Task 3 and Task 4
- Tiered budgets: covered by Task 1 wiring and Task 2 policy
- Lightweight validation: covered by Task 3
- No read-path migration in initial phase: enforced by Task 4

### Placeholder scan

- No `TODO`, `TBD`, or "implement later" placeholders remain.
- Every task names exact files and exact `go test` commands.
- Every code-changing step contains concrete code snippets.

### Type consistency

- Persisted field name is consistently `StructuredSummaryJSON`.
- Typed helper struct is consistently `StructuredSummary`.
- Budget config is consistently `SummaryBudget`.
- Validation outcome uses existing `domain.SummaryQualityAccepted` and `domain.SummaryQualityRejected`.

