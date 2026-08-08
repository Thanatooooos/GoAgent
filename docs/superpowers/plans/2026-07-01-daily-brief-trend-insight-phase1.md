# Daily Brief Trend Insight Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an AI-first `Trend Insight` module for `Daily Brief` that shows one top-of-page analytical paragraph plus up to two supporting signals, backed by a precomputed snapshot loaded from `GET /daily-brief/today`.

**Architecture:** Add a new sibling package `internal/app/dailybrief/trend` for Phase-1 trend analysis. Persist one `TrendInsightSnapshot` per user/topic/brief date in Postgres, refresh it after a successful brief publish, then extend `ReadService`, HTTP DTOs, and the React page to render the snapshot when present without blocking the normal brief flow.

**Tech Stack:** Go, GORM/Postgres, Gin, React 18, TypeScript, Vite

---

## File Map

**Create**

- `internal/app/dailybrief/trend/types.go`
- `internal/app/dailybrief/trend/analyzer.go`
- `internal/app/dailybrief/trend/analyzer_test.go`
- `internal/app/dailybrief/trend/snapshot_service.go`
- `internal/app/dailybrief/trend/snapshot_service_test.go`
- `internal/app/dailybrief/port/trend_snapshot_repository.go`
- `internal/adapter/repository/postgres/dailybrief/models/trend_snapshot_model.go`
- `internal/adapter/repository/postgres/dailybrief/trend_snapshot_repo.go`
- `internal/adapter/repository/postgres/migrations/20260701110000_create_daily_brief_trend_snapshot_table.sql`
- `docs/superpowers/plans/2026-07-01-daily-brief-trend-insight-phase1.md`

**Modify**

- `internal/app/dailybrief/port/repository.go`
- `internal/app/dailybrief/service/read_service.go`
- `internal/app/dailybrief/service/read_service_test.go`
- `internal/app/dailybrief/service/generation_orchestrator.go`
- `internal/adapter/repository/postgres/dailybrief/issue_repo.go`
- `internal/adapter/repository/postgres/dailybrief/mapper.go`
- `internal/adapter/repository/postgres/dailybrief/repository_contract_test.go`
- `internal/adapter/repository/postgres/dailybrief/migration_test.go`
- `internal/bootstrap/dailybrief/runtime.go`
- `internal/adapter/http/dailybrief/dto.go`
- `internal/adapter/http/dailybrief/handler_test.go`
- `frontend/src/types/dailyBrief.ts`
- `frontend/src/pages/DailyBriefPage.tsx`

**Why these files**

- `trend/*` holds all new analysis logic and keeps `service/` from growing further.
- `port/*` and Postgres repo files add the new snapshot persistence seam.
- `generation_orchestrator.go` is the safest existing place to refresh the snapshot after a successful publish.
- `read_service.go`, DTOs, and frontend files expose the snapshot through the existing `/daily-brief/today` path.

### Task 1: Add the snapshot schema and repository seam

**Files:**

- Create: `internal/app/dailybrief/port/trend_snapshot_repository.go`
- Create: `internal/adapter/repository/postgres/dailybrief/models/trend_snapshot_model.go`
- Create: `internal/adapter/repository/postgres/dailybrief/trend_snapshot_repo.go`
- Create: `internal/adapter/repository/postgres/migrations/20260701110000_create_daily_brief_trend_snapshot_table.sql`
- Modify: `internal/app/dailybrief/port/repository.go`
- Modify: `internal/adapter/repository/postgres/dailybrief/mapper.go`
- Modify: `internal/adapter/repository/postgres/dailybrief/repository_contract_test.go`
- Modify: `internal/adapter/repository/postgres/dailybrief/migration_test.go`

- [ ] **Step 1: Write the failing repository contract and migration tests**

```go
// internal/adapter/repository/postgres/dailybrief/repository_contract_test.go
var _ port.TrendInsightSnapshotRepository = NewTrendInsightSnapshotRepository(nil)

// internal/adapter/repository/postgres/dailybrief/migration_test.go
required = append(required,
  "CREATE TABLE IF NOT EXISTS t_daily_brief_trend_snapshot",
  "uk_daily_brief_trend_snapshot_user_topic_date",
  "idx_daily_brief_trend_snapshot_user_generated",
  "current_summary_json",
  "signals_json",
  "background_json",
  "evidence_json",
  "methodology_json",
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/repository/postgres/dailybrief -run "TestRepositoriesImplementDailyBriefPorts|TestDailyBriefMigrationDefinesCoreTablesAndIndexes" -v`

Expected: FAIL with missing `TrendInsightSnapshotRepository` and missing migration tokens.

- [ ] **Step 3: Add the repository interface and Postgres model**

```go
// internal/app/dailybrief/port/trend_snapshot_repository.go
package port

import (
  "context"

  "local/rag-project/internal/app/dailybrief/trend"
)

type TrendSnapshotListFilter struct {
  UserID string
  Topic  string
  Limit  int
}

type TrendInsightSnapshotRepository interface {
  Upsert(ctx context.Context, snapshot trend.TrendInsightSnapshot) (trend.TrendInsightSnapshot, error)
  GetLatestByUserID(ctx context.Context, userID string) (trend.TrendInsightSnapshot, error)
  List(ctx context.Context, filter TrendSnapshotListFilter) ([]trend.TrendInsightSnapshot, error)
}
```

```go
// internal/adapter/repository/postgres/dailybrief/models/trend_snapshot_model.go
package models

import "time"

type TrendSnapshotModel struct {
  ID                 string     `gorm:"column:id;primaryKey"`
  UserID             string     `gorm:"column:user_id"`
  Topic              string     `gorm:"column:topic"`
  BriefDate          string     `gorm:"column:brief_date"`
  Interpretation     string     `gorm:"column:interpretation"`
  CurrentSummaryJSON string     `gorm:"column:current_summary_json"`
  SignalsJSON        string     `gorm:"column:signals_json"`
  BackgroundJSON     string     `gorm:"column:background_json"`
  EvidenceJSON       string     `gorm:"column:evidence_json"`
  MethodologyJSON    string     `gorm:"column:methodology_json"`
  GeneratedAt        *time.Time `gorm:"column:generated_at"`
  CreateTime         time.Time  `gorm:"column:create_time"`
  UpdateTime         time.Time  `gorm:"column:update_time"`
}

func (TrendSnapshotModel) TableName() string { return "t_daily_brief_trend_snapshot" }
```

- [ ] **Step 4: Add the repo implementation and migration**

```go
// internal/adapter/repository/postgres/dailybrief/trend_snapshot_repo.go
func (r *TrendInsightSnapshotRepository) GetLatestByUserID(ctx context.Context, userID string) (trend.TrendInsightSnapshot, error) {
  var model models.TrendSnapshotModel
  err := r.db.WithContext(ctx).
    Where("user_id = ?", userID).
    Order("generated_at desc, update_time desc").
    First(&model).Error
  if errors.Is(err, gorm.ErrRecordNotFound) {
    return trend.TrendInsightSnapshot{}, nil
  }
  if err != nil {
    return trend.TrendInsightSnapshot{}, fmt.Errorf("get latest daily brief trend snapshot: %w", err)
  }
  return toTrendSnapshotDomain(model)
}
```

```sql
CREATE TABLE IF NOT EXISTS t_daily_brief_trend_snapshot (
  id VARCHAR(64) PRIMARY KEY,
  user_id VARCHAR(64) NOT NULL,
  topic VARCHAR(128) NOT NULL,
  brief_date VARCHAR(16) NOT NULL,
  interpretation TEXT NOT NULL,
  current_summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  signals_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  background_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  methodology_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  generated_at TIMESTAMPTZ NULL,
  create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uk_daily_brief_trend_snapshot_user_topic_date UNIQUE (user_id, topic, brief_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_brief_trend_snapshot_user_generated
  ON t_daily_brief_trend_snapshot (user_id, generated_at DESC);
```

- [ ] **Step 5: Run repository tests again**

Run: `go test ./internal/adapter/repository/postgres/dailybrief -run "TestRepositoriesImplementDailyBriefPorts|TestDailyBriefMigrationDefinesCoreTablesAndIndexes" -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/dailybrief/port/trend_snapshot_repository.go internal/app/dailybrief/port/repository.go internal/adapter/repository/postgres/dailybrief/models/trend_snapshot_model.go internal/adapter/repository/postgres/dailybrief/trend_snapshot_repo.go internal/adapter/repository/postgres/dailybrief/mapper.go internal/adapter/repository/postgres/dailybrief/repository_contract_test.go internal/adapter/repository/postgres/dailybrief/migration_test.go internal/adapter/repository/postgres/migrations/20260701110000_create_daily_brief_trend_snapshot_table.sql
git commit -m "feat: add daily brief trend snapshot storage"
```

### Task 2: Build the AI-first trend analyzer package

**Files:**

- Create: `internal/app/dailybrief/trend/types.go`
- Create: `internal/app/dailybrief/trend/analyzer.go`
- Create: `internal/app/dailybrief/trend/analyzer_test.go`

- [ ] **Step 1: Write failing analyzer tests for summary, change, and primary frame selection**

```go
func TestAnalyzeAIItemsBuildsCapabilityToUtilityInsight(t *testing.T) {
  analyzer := NewAnalyzer()
  items := []domain.Item{
    {Title: "Open model release lowers inference cost", Topic: "tech.ai.models", Source: "openai-blog", Summary: "open weights and serving"},
    {Title: "Agent framework adds enterprise workflow tools", Topic: "tech.ai.models", Source: "github-trending", Summary: "agent tooling for developers"},
    {Title: "Benchmark leaderboard update", Topic: "tech.ai.models", Source: "hacker-news", Summary: "benchmark only"},
  }

  snapshot, ok := analyzer.AnalyzeAI("alice", "2026-07-01", items, items)
  if !ok {
    t.Fatalf("expected analyzable snapshot")
  }
  if snapshot.Topic != "ai" {
    t.Fatalf("unexpected topic: %q", snapshot.Topic)
  }
  if snapshot.CurrentSummary == nil || len(snapshot.CurrentSummary.DominantThemes) == 0 {
    t.Fatalf("expected current summary themes")
  }
  if len(snapshot.Signals) == 0 {
    t.Fatalf("expected supporting signals")
  }
  if snapshot.PrimaryFrame != "capability_to_utility" {
    t.Fatalf("unexpected primary frame: %q", snapshot.PrimaryFrame)
  }
}
```

- [ ] **Step 2: Run the analyzer test to verify it fails**

Run: `go test ./internal/app/dailybrief/trend -run TestAnalyzeAIItemsBuildsCapabilityToUtilityInsight -v`

Expected: FAIL because the package and analyzer do not exist.

- [ ] **Step 3: Add the Phase-1 types**

```go
// internal/app/dailybrief/trend/types.go
package trend

import "time"

type CurrentStateSummary struct {
  Summary          string   `json:"summary"`
  DominantThemes   []string `json:"dominantThemes"`
  SupportingThemes []string `json:"supportingThemes"`
}

type TrendSignal struct {
  Kind    string   `json:"kind"`
  Title   string   `json:"title"`
  Summary string   `json:"summary,omitempty"`
  Tags    []string `json:"tags,omitempty"`
}

type TrendEvidenceItem struct {
  Title       string `json:"title"`
  URL         string `json:"url"`
  Source      string `json:"source"`
  Topic       string `json:"topic"`
  BriefDate   string `json:"briefDate,omitempty"`
  PublishedAt string `json:"publishedAt,omitempty"`
}

type TrendInsightSnapshot struct {
  ID             string
  UserID         string
  Topic          string
  BriefDate      string
  Interpretation string
  CurrentSummary *CurrentStateSummary
  Signals        []TrendSignal
  Background     *TrendBackground
  EvidenceGroups []TrendEvidenceGroup
  Methodology    *TrendMethodology
  PrimaryFrame   string
  GeneratedAt    *time.Time
  CreatedAt      time.Time
  UpdatedAt      time.Time
}
```

- [ ] **Step 4: Add the narrow analyzer implementation**

```go
// internal/app/dailybrief/trend/analyzer.go
func (a *Analyzer) AnalyzeAI(userID, briefDate string, recentItems, previousItems []domain.Item) (TrendInsightSnapshot, bool) {
  if len(recentItems) < 3 {
    return TrendInsightSnapshot{}, false
  }

  summary := buildCurrentStateSummary(recentItems)
  change := buildDomainChangeSet(recentItems, previousItems)
  frame := selectPrimaryFrame(summary, change)
  if frame == "" {
    return TrendInsightSnapshot{}, false
  }

  now := time.Now().UTC()
  return TrendInsightSnapshot{
    UserID:         userID,
    Topic:          "ai",
    BriefDate:      briefDate,
    Interpretation: composeInterpretation(summary, change, frame),
    CurrentSummary: &summary,
    Signals:        buildSignals(change),
    Background:     buildBackground(recentItems, previousItems),
    EvidenceGroups: buildEvidenceGroups(change, recentItems),
    Methodology:    &TrendMethodology{Summary: "Based on recent AI brief coverage compared with the immediately preceding stretch.", SampleSize: len(recentItems)},
    PrimaryFrame:   frame,
    GeneratedAt:    &now,
  }, true
}
```

- [ ] **Step 5: Run the analyzer package tests**

Run: `go test ./internal/app/dailybrief/trend -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/dailybrief/trend/types.go internal/app/dailybrief/trend/analyzer.go internal/app/dailybrief/trend/analyzer_test.go
git commit -m "feat: add daily brief trend analyzer"
```

### Task 3: Refresh the snapshot after publish

**Files:**

- Create: `internal/app/dailybrief/trend/snapshot_service.go`
- Create: `internal/app/dailybrief/trend/snapshot_service_test.go`
- Modify: `internal/app/dailybrief/port/repository.go`
- Modify: `internal/adapter/repository/postgres/dailybrief/issue_repo.go`
- Modify: `internal/app/dailybrief/service/generation_orchestrator.go`
- Modify: `internal/bootstrap/dailybrief/runtime.go`

- [ ] **Step 1: Write the failing snapshot refresh tests**

```go
func TestSnapshotServiceRefreshForUserStoresLatestAISnapshot(t *testing.T) {
  issueRepo := &stubIssueRepo{issues: []domain.Issue{
    {ID: "issue-1", UserID: "alice", BriefDate: "2026-06-30", Status: domain.IssueStatusReady},
    {ID: "issue-2", UserID: "alice", BriefDate: "2026-07-01", Status: domain.IssueStatusReady},
  }}
  itemRepo := &stubItemRepo{itemsByIssue: map[string][]domain.Item{
    "issue-1": {{Title: "Closed benchmark release", Topic: "tech.ai.models"}},
    "issue-2": {{Title: "Open model and agent tooling", Topic: "tech.ai.models"}},
  }}
  repo := &stubTrendSnapshotRepo{}
  service := NewSnapshotService(issueRepo, itemRepo, repo, NewAnalyzer())

  err := service.RefreshForUser(context.Background(), "alice", "2026-07-01")
  if err != nil {
    t.Fatalf("RefreshForUser returned error: %v", err)
  }
  if repo.saved.Topic != "ai" {
    t.Fatalf("unexpected saved topic: %q", repo.saved.Topic)
  }
}
```

- [ ] **Step 2: Run the trend tests to verify the new service test fails**

Run: `go test ./internal/app/dailybrief/trend -run TestSnapshotServiceRefreshForUserStoresLatestAISnapshot -v`

Expected: FAIL because `SnapshotService` and date-range support do not exist.

- [ ] **Step 3: Extend issue filtering and add the snapshot service**

```go
// internal/app/dailybrief/port/repository.go
type IssueListFilter struct {
  UserID        string
  BriefDate     string
  Status        string
  BriefDateFrom string
  BriefDateTo   string
  ListOptions
}
```

```go
// internal/adapter/repository/postgres/dailybrief/issue_repo.go
if filter.BriefDateFrom != "" {
  query = query.Where("brief_date >= ?", filter.BriefDateFrom)
}
if filter.BriefDateTo != "" {
  query = query.Where("brief_date <= ?", filter.BriefDateTo)
}
```

```go
// internal/app/dailybrief/trend/snapshot_service.go
type SnapshotService struct {
  issueRepo    port.IssueRepository
  itemRepo     port.ItemRepository
  snapshotRepo port.TrendInsightSnapshotRepository
  analyzer     *Analyzer
}

func (s *SnapshotService) RefreshForUser(ctx context.Context, userID, briefDate string) error {
  recentIssues, err := s.issueRepo.List(ctx, port.IssueListFilter{
    UserID:        userID,
    Status:        domain.IssueStatusReady,
    BriefDateTo:   briefDate,
    BriefDateFrom: previousDate(briefDate, 13),
    ListOptions:   port.ListOptions{Limit: 14},
  })
  if err != nil {
    return err
  }
  recentItems, previousItems, err := loadComparisonItems(ctx, s.itemRepo, recentIssues, briefDate)
  if err != nil {
    return err
  }
  snapshot, ok := s.analyzer.AnalyzeAI(userID, briefDate, recentItems, previousItems)
  if !ok {
    return nil
  }
  _, err = s.snapshotRepo.Upsert(ctx, snapshot)
  return err
}
```

- [ ] **Step 4: Call snapshot refresh from the generation orchestrator without breaking publish**

```go
// internal/app/dailybrief/service/generation_orchestrator.go
type TrendSnapshotRefresher interface {
  RefreshForUser(ctx context.Context, userID, briefDate string) error
}

// add field
trendSnapshotRefresher TrendSnapshotRefresher

// after run update succeeds
if o.trendSnapshotRefresher != nil {
  if err := o.trendSnapshotRefresher.RefreshForUser(finalizeCtx, request.Subscription.UserID, request.BriefDate); err != nil {
    // do not fail the brief publish path on trend refresh errors
  }
}
```

- [ ] **Step 5: Wire the refresher in runtime and run the relevant tests**

Run: `go test ./internal/app/dailybrief/trend ./internal/app/dailybrief/service ./internal/bootstrap/dailybrief -run "TestSnapshotServiceRefreshForUserStoresLatestAISnapshot|TestDailyBriefPipelineE2E" -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/dailybrief/trend/snapshot_service.go internal/app/dailybrief/trend/snapshot_service_test.go internal/app/dailybrief/port/repository.go internal/adapter/repository/postgres/dailybrief/issue_repo.go internal/app/dailybrief/service/generation_orchestrator.go internal/bootstrap/dailybrief/runtime.go
git commit -m "feat: refresh daily brief trend snapshots after publish"
```

### Task 4: Extend the read path and HTTP response

**Files:**

- Modify: `internal/app/dailybrief/service/read_service.go`
- Modify: `internal/app/dailybrief/service/read_service_test.go`
- Modify: `internal/adapter/http/dailybrief/dto.go`
- Modify: `internal/adapter/http/dailybrief/handler_test.go`
- Modify: `internal/bootstrap/dailybrief/runtime.go`

- [ ] **Step 1: Write the failing read-service and handler tests**

```go
func TestReadServiceGetTodayIncludesLatestTrendSnapshot(t *testing.T) {
  snapshotRepo := &stubTrendSnapshotRepo{
    latest: trend.TrendInsightSnapshot{
      Topic:          "ai",
      BriefDate:      "2026-07-01",
      Interpretation: "AI is shifting toward usability and workflow fit.",
      Signals:        []trend.TrendSignal{{Kind: "shift", Title: "Agent tooling is more prominent"}},
    },
  }
  service := NewReadService(subscriptionRepo, issueRepo, itemRepo, snapshotRepo)

  model, err := service.GetToday(context.Background(), "user-1", someNow)
  if err != nil {
    t.Fatalf("GetToday returned error: %v", err)
  }
  if model.TrendInsight == nil || model.TrendInsight.Topic != "ai" {
    t.Fatalf("expected trend insight in read model")
  }
}
```

```go
func TestHandlerGetTodayReturnsTrendInsightWhenPresent(t *testing.T) {
  // assert payload.data.trendInsight.interpretation is present
}
```

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run: `go test ./internal/app/dailybrief/service ./internal/adapter/http/dailybrief -run "TestReadServiceGetTodayIncludesLatestTrendSnapshot|TestHandlerGetTodayReturnsTrendInsightWhenPresent" -v`

Expected: FAIL because `ReadService` and DTOs do not expose `trendInsight`.

- [ ] **Step 3: Add the read-model field and load the latest snapshot only for today**

```go
type IssueReadModel struct {
  Issue             domain.Issue
  Items             []domain.Item
  Status            string
  PageState         string
  BriefDate         string
  LastGeneratedTime *time.Time
  TrendInsight      *trend.TrendInsightSnapshot
}

func (s *ReadService) GetToday(ctx context.Context, userID string, now time.Time) (IssueReadModel, error) {
  model, err := s.getByDateWithSubscription(ctx, userID, briefDate, subscription, now)
  if err != nil {
    return IssueReadModel{}, err
  }
  if s.trendSnapshotRepo != nil {
    snapshot, err := s.trendSnapshotRepo.GetLatestByUserID(ctx, strings.TrimSpace(userID))
    if err != nil {
      return IssueReadModel{}, err
    }
    if strings.TrimSpace(snapshot.Topic) != "" {
      model.TrendInsight = &snapshot
    }
  }
  return model, nil
}
```

- [ ] **Step 4: Extend the DTOs**

```go
type issueResponse struct {
  PageState       string           `json:"pageState"`
  Issue           *issueDTO        `json:"issue,omitempty"`
  BriefDate       string           `json:"briefDate"`
  LastGeneratedAt *string          `json:"lastGeneratedAt,omitempty"`
  TrendInsight    *trendInsightDTO `json:"trendInsight,omitempty"`
}
```

```go
type trendInsightDTO struct {
  Topic          string             `json:"topic"`
  Interpretation string             `json:"interpretation"`
  CurrentSummary *trendCurrentDTO   `json:"currentSummary,omitempty"`
  Signals        []trendSignalDTO   `json:"signals"`
  Background     *trendBackgroundDTO `json:"background,omitempty"`
  EvidenceGroups []trendEvidenceDTO `json:"evidenceGroups,omitempty"`
}
```

- [ ] **Step 5: Run backend tests**

Run: `go test ./internal/app/dailybrief/service ./internal/adapter/http/dailybrief ./internal/bootstrap/dailybrief -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/dailybrief/service/read_service.go internal/app/dailybrief/service/read_service_test.go internal/adapter/http/dailybrief/dto.go internal/adapter/http/dailybrief/handler_test.go internal/bootstrap/dailybrief/runtime.go
git commit -m "feat: expose daily brief trend insight in today response"
```

### Task 5: Render the trend module in the React page

**Files:**

- Modify: `frontend/src/types/dailyBrief.ts`
- Modify: `frontend/src/pages/DailyBriefPage.tsx`

- [ ] **Step 1: Add the failing TypeScript types by updating the page to reference `trendInsight`**

```tsx
// inside DailyBriefPage.tsx
const trendInsight = issueData?.trendInsight ?? null;
```

- [ ] **Step 2: Run the frontend build to verify it fails on missing types**

Run: `npm --prefix frontend run build`

Expected: FAIL with TypeScript errors because `DailyBriefIssueResponse` has no `trendInsight`.

- [ ] **Step 3: Add the frontend types**

```ts
export interface DailyBriefTrendSignal {
  kind: "rising" | "falling" | "new" | "persistent" | "shift";
  title: string;
  summary?: string | null;
  tags?: string[];
}

export interface DailyBriefTrendInsight {
  topic: string;
  interpretation: string;
  currentSummary?: {
    summary: string;
    dominantThemes: string[];
  } | null;
  signals: DailyBriefTrendSignal[];
  background?: {
    summary: string;
    alignsWithRecent: boolean;
  } | null;
  evidenceGroups?: {
    title: string;
    summary: string;
    items: DailyBriefIssueItem[];
  }[];
}

export interface DailyBriefIssueResponse {
  pageState: DailyBriefPageState;
  issue?: DailyBriefIssue | null;
  briefDate: string;
  lastGeneratedAt?: string | null;
  trendInsight?: DailyBriefTrendInsight | null;
}
```

- [ ] **Step 4: Render a simple top card above the normal issue headline**

```tsx
{trendInsight ? (
  <Card className="border-sky-200 bg-gradient-to-br from-sky-50 via-white to-cyan-50 shadow-sm">
    <CardHeader className="pb-3">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.24em] text-sky-700">Trend Insight</p>
          <CardTitle className="mt-2 text-xl text-slate-900">AI 最近的变化</CardTitle>
        </div>
        <Badge variant="outline" className="border-sky-200 text-sky-700">
          {trendInsight.topic.toUpperCase()}
        </Badge>
      </div>
    </CardHeader>
    <CardContent className="space-y-4">
      <p className="text-[15px] leading-8 text-slate-700">{trendInsight.interpretation}</p>
      <div className="grid gap-3 md:grid-cols-2">
        {trendInsight.signals.slice(0, 2).map((signal) => (
          <div key={signal.title} className="rounded-2xl border border-sky-100 bg-white/80 p-4">
            <p className="text-sm font-semibold text-slate-900">{signal.title}</p>
            {signal.summary ? <p className="mt-2 text-sm leading-6 text-slate-600">{signal.summary}</p> : null}
          </div>
        ))}
      </div>
      {trendInsight.background?.summary ? (
        <p className="text-sm leading-6 text-slate-500">{trendInsight.background.summary}</p>
      ) : null}
    </CardContent>
  </Card>
) : null}
```

- [ ] **Step 5: Run frontend verification**

Run: `npm --prefix frontend run build`

Expected: PASS

Run: `npm --prefix frontend run lint`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/dailyBrief.ts frontend/src/pages/DailyBriefPage.tsx
git commit -m "feat: render daily brief trend insight module"
```

### Task 6: Final verification and scope guard

**Files:**

- Modify: `internal/app/dailybrief/trend/analyzer.go`
- Modify: `internal/app/dailybrief/trend/snapshot_service.go`
- Modify: `internal/app/dailybrief/service/read_service.go`
- Modify: `frontend/src/pages/DailyBriefPage.tsx`

- [ ] **Step 1: Re-check the Phase-1 scope against the implementation**

```text
Phase-1 scope checklist:
- AI-only topic selection
- one primary frame only
- at most two signal cards
- no /daily-brief/issues trend playback
- no persisted TrendObservation table
- normal brief page still works when trend insight is absent
```

- [ ] **Step 2: Run the backend verification suite**

Run: `go test ./internal/app/dailybrief/trend ./internal/app/dailybrief/service ./internal/adapter/http/dailybrief ./internal/adapter/repository/postgres/dailybrief ./internal/bootstrap/dailybrief -v`

Expected: PASS

- [ ] **Step 3: Run the frontend verification suite**

Run: `npm --prefix frontend run build`

Expected: PASS

Run: `npm --prefix frontend run lint`

Expected: PASS

- [ ] **Step 4: Run the OpenSpec validation again**

Run: `openspec validate add-daily-brief-trend-insights --strict`

Expected: PASS

- [ ] **Step 5: Commit the final integration pass**

```bash
git add internal/app/dailybrief/trend internal/app/dailybrief/service internal/adapter/http/dailybrief internal/adapter/repository/postgres/dailybrief internal/bootstrap/dailybrief frontend/src/types/dailyBrief.ts frontend/src/pages/DailyBriefPage.tsx openspec/changes/add-daily-brief-trend-insights/tasks.md
git commit -m "feat: ship daily brief trend insight phase 1"
```

## Self-Review

**Spec coverage**

- Top-of-page module: covered by Task 5.
- `AI`-first narrow rollout: enforced in Task 2 and Task 6.
- Precomputed snapshot through `GET /daily-brief/today`: covered by Tasks 3 and 4.
- `summary + change + implication`: analyzer structure in Task 2.
- Single primary frame: analyzer and scope guard in Tasks 2 and 6.
- Graceful absence / non-blocking behavior: Task 3 keeps refresh non-fatal, Task 4 and Task 5 handle nil snapshot.

**Placeholder scan**

- No placeholder markers left.
- Every code-bearing step includes a concrete code block or command.

**Type consistency**

- Repository name is consistently `TrendInsightSnapshotRepository`.
- Read-model field is consistently `TrendInsight`.
- Trend package output is consistently `TrendInsightSnapshot`.
