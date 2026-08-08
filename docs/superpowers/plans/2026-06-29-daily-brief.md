# Daily Brief Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the `Daily Brief` MVP as a standalone product domain that generates one structured AI and technology brief per subscribed user per local day, publishes it into a dedicated in-app page, and retries failed scheduled generation automatically without adding a user-triggered regenerate flow.

**Architecture:** Add a new `dailybrief` domain that follows the repository's existing `domain + port + service + repository + http + bootstrap + schedule` shape. Reuse the current Postgres migration flow, Gin route registration, runtime startup/shutdown assembly, model invocation infrastructure, and frontend authenticated page layout. Keep `Daily Brief` separate from `rag` chat semantics and separate from the new `agent` runtime.

**Tech Stack:** Go, Gin, GORM, Postgres SQL migrations, existing AI runtime, React, Vite, TypeScript, existing axios API layer

---

## Scope Guardrails

- `Daily Brief` stays a standalone business domain under `internal/app/dailybrief`.
- MVP supports only curated external technology and AI sources.
- MVP exposes only these user APIs:
  - `GET /daily-brief/today`
  - `GET /daily-brief/issues`
  - `GET /daily-brief/subscription`
  - `PUT /daily-brief/subscription`
- MVP does not add a user-triggered regenerate endpoint or button.
- Retries are internal-only and apply only to failed scheduled runs.
- The published read model stays singular per `(user_id, brief_date)`.
- Source registry remains code-owned for MVP. Users select from approved source keys rather than entering arbitrary feed URLs.

## File Map

### Existing files that should be extended

- `cmd/server/main.go`
  - Create and close the new runtime.
  - Register authenticated `daily-brief` routes.
- `configs/application.yaml`
  - Add runtime knobs for scan interval, run timeout, lock TTL, retry policy, and generation limits.
- `internal/framework/config/config.go`
  - Add a top-level `DailyBriefConfig` instead of nesting the feature under `rag`.
- `frontend/src/router.tsx`
  - Register the authenticated `Daily Brief` page route.
- `frontend/src/components/layout/Sidebar.tsx`
  - Add a stable navigation entry to `Daily Brief`.

### New backend directories expected

- `internal/app/dailybrief/domain`
- `internal/app/dailybrief/port`
- `internal/app/dailybrief/service`
- `internal/app/dailybrief/schedule`
- `internal/adapter/repository/postgres/dailybrief`
- `internal/adapter/http/dailybrief`
- `internal/bootstrap/dailybrief`

### New frontend files expected

- `frontend/src/pages/DailyBriefPage.tsx`
- `frontend/src/services/dailyBriefService.ts`
- `frontend/src/types/dailyBrief.ts`

### New testdata expected

- `testdata/dailybrief/sources/`
  - RSS and Atom fixtures
  - HTML fixtures for sources that do not expose a stable feed surface
  - structured generation input and output fixtures

### New migration expected

- `internal/adapter/repository/postgres/migrations/20260629xxxxxx_create_daily_brief_tables.sql`

## Recommended Execution Order

1. Freeze contracts, enums, and configuration first.
2. Create schema and repository primitives before writing orchestration.
3. Build the read path and subscription lifecycle before the scheduler.
4. Implement source normalization and deterministic ranking before any LLM prompt work.
5. Add generation orchestration, locking, and retry behavior.
6. Wire HTTP routes and runtime startup only after domain services are testable.
7. Add the frontend page after the backend contract is stable.
8. Finish with observability, fixtures, and end-to-end verification.

This sequence keeps the highest-risk logic isolated and reduces churn between backend and frontend.

## Task 1: Freeze MVP Contracts, Catalogs, and Configuration

**Files:**
- Modify: `internal/framework/config/config.go`
- Modify: `configs/application.yaml`
- Create: `internal/app/dailybrief/domain/source_catalog.go`
- Create: `internal/app/dailybrief/domain/topic_catalog.go`
- Create: `internal/app/dailybrief/domain/status.go`
- Create: `internal/app/dailybrief/domain/constants_test.go`
- Test: `internal/framework/config/config_test.go`

- [ ] Add a top-level `DailyBriefConfig` to `config.Config` with only runtime knobs, not per-source user data.

- [ ] Use a config shape similar to:
  - `daily-brief.schedule.scan-delay-ms`
  - `daily-brief.schedule.run-timeout-ms`
  - `daily-brief.schedule.lock-seconds`
  - `daily-brief.schedule.batch-size`
  - `daily-brief.retry.max-attempts`
  - `daily-brief.retry.backoff-minutes`
  - `daily-brief.generation.max-candidates`
  - `daily-brief.generation.max-items`
  - `daily-brief.generation.prompt-version`
  - `daily-brief.generation.model`

- [ ] Keep the curated source registry in code, not in YAML, so MVP source additions remain reviewed code changes rather than open runtime configuration.

- [ ] Define stable source keys and topic keys centrally. The subscription API and database should persist these keys rather than free-text names.

- [ ] Encode the "no user-triggered regeneration" rule directly into the API surface and service vocabulary so it does not accidentally reappear during implementation.

- [ ] Define the first source acquisition posture explicitly:
  - RSS or Atom first where available
  - lightweight HTML parsing only where feed access is not enough
  - no general crawler framework
  - no arbitrary user-provided source URLs

Run:

```bash
go test ./internal/framework/config ./internal/app/dailybrief/domain -v
```

Expected:

- Config parsing loads sane defaults for the new `daily-brief` section.
- Source and topic catalogs reject unknown keys consistently.

## Task 2: Create Schema, Models, and Repository Contracts

**Files:**
- Create: `internal/adapter/repository/postgres/migrations/20260629xxxxxx_create_daily_brief_tables.sql`
- Create: `internal/adapter/repository/postgres/dailybrief/models/subscription_model.go`
- Create: `internal/adapter/repository/postgres/dailybrief/models/issue_model.go`
- Create: `internal/adapter/repository/postgres/dailybrief/models/item_model.go`
- Create: `internal/adapter/repository/postgres/dailybrief/models/generation_run_model.go`
- Create: `internal/adapter/repository/postgres/dailybrief/subscription_repo.go`
- Create: `internal/adapter/repository/postgres/dailybrief/issue_repo.go`
- Create: `internal/adapter/repository/postgres/dailybrief/item_repo.go`
- Create: `internal/adapter/repository/postgres/dailybrief/generation_run_repo.go`
- Create: `internal/adapter/repository/postgres/dailybrief/transaction.go`
- Create: `internal/app/dailybrief/port/repository.go`
- Test: `internal/adapter/repository/postgres/dailybrief/*_test.go`

- [ ] Create the four primary tables described in the design:
  - subscription
  - issue
  - item
  - generation run

- [ ] Enforce unique `(user_id, brief_date)` on issues.

- [ ] Add the indexes needed for the MVP read and scheduling paths:
  - subscription by `enabled`
  - issue by `(user_id, brief_date)`
  - issue by `(user_id, status)`
  - generation run by `(user_id, brief_date, started_at desc)`
  - item by `(issue_id, section_key, rank)`

- [ ] Add operational lock fields to `subscription` for schedule lease ownership, mirroring the existing knowledge schedule locking pattern:
  - `lock_owner`
  - `lock_until`

- [ ] Keep lock fields internal-only. They must not surface in user-facing DTOs.

- [ ] Add repository methods for:
  - get and upsert subscription by user
  - get issue by user and date
  - list issue dates or issue history for user
  - replace issue items transactionally
  - create generation run
  - list failed runs eligible for retry
  - conditional subscription lock acquire, renew, and release

- [ ] Prefer repository-level conditional updates over in-memory race checks.

Run:

```bash
go test ./internal/adapter/repository/postgres/dailybrief -v
go test ./internal/adapter/repository/postgres/... -run DailyBrief -v
```

Expected:

- Migration creates all required tables.
- Unique `(user_id, brief_date)` is enforced.
- Lock acquire and release semantics are guarded by ownership.

## Task 3: Add Domain Entities and Core Services

**Files:**
- Create: `internal/app/dailybrief/domain/subscription.go`
- Create: `internal/app/dailybrief/domain/issue.go`
- Create: `internal/app/dailybrief/domain/item.go`
- Create: `internal/app/dailybrief/domain/generation_run.go`
- Create: `internal/app/dailybrief/domain/candidate.go`
- Create: `internal/app/dailybrief/service/subscription_service.go`
- Create: `internal/app/dailybrief/service/issue_service.go`
- Create: `internal/app/dailybrief/service/generation_run_service.go`
- Create: `internal/app/dailybrief/service/read_service.go`
- Test: `internal/app/dailybrief/service/*_test.go`

- [ ] Model domain transitions explicitly:
  - `Issue`: `generating -> ready|failed`
  - `GenerationRun`: `running -> succeeded|degraded|failed`

- [ ] Validate subscription writes at the service layer:
  - timezone must be present
  - delivery time must be valid
  - topics must be a subset of the catalog
  - sources must be a subset of the catalog

- [ ] Define a clear read service that serves exactly what the page needs:
  - today's issue plus status
  - issue by selected date
  - current subscription config

- [ ] Keep the page-facing service decoupled from generation-run internals so the UI does not need to understand retries.

- [ ] Add a single response-friendly issue shape that includes:
  - metadata
  - headline
  - top summary
  - sections
  - items
  - last generated timestamp

Run:

```bash
go test ./internal/app/dailybrief/service -v
```

Expected:

- Subscription validation is deterministic.
- Issue and generation run transitions reject invalid state changes.

## Task 4: Implement Source Collection and Deterministic Candidate Pipeline

**Files:**
- Create: `internal/app/dailybrief/port/source.go`
- Create: `internal/app/dailybrief/service/source_registry.go`
- Create: `internal/app/dailybrief/service/source_collector.go`
- Create: `internal/app/dailybrief/service/candidate_pipeline.go`
- Create: `internal/app/dailybrief/service/dedup.go`
- Create: `internal/app/dailybrief/service/ranking.go`
- Create: `internal/app/dailybrief/service/topic_filter.go`
- Create: `internal/app/dailybrief/service/http_client.go`
- Create: `internal/app/dailybrief/service/source_*.go`
- Test: `internal/app/dailybrief/service/source_*_test.go`
- Test: `internal/app/dailybrief/service/candidate_pipeline_test.go`
- Testdata: `testdata/dailybrief/sources/*`

- [ ] Normalize all fetched content into one candidate shape before ranking.

- [ ] Implement the first source set in a stability-first order:
  - Hacker News
  - arXiv `cs.AI`, `cs.CL`, `cs.LG`
  - official blogs from OpenAI, Anthropic, Google DeepMind, Meta AI
  - The Decoder
  - VentureBeat AI
  - TechCrunch AI
  - GitHub Trending
  - Papers with Code

- [ ] Prefer fixture-driven parser tests over live network tests.

- [ ] Implement deterministic deduplication using:
  - canonical URL normalization
  - provider external ID when available
  - conservative title similarity fallback

- [ ] Implement deterministic ranking with visible heuristics:
  - freshness
  - topic match
  - source priority
  - de-duplication confidence
  - content richness

- [ ] Limit the candidate set before the LLM step. Do not send the full fetch volume into summarization.

- [ ] Preserve enough candidate metadata for sectioning and "why it matters" generation later.

Run:

```bash
go test ./internal/app/dailybrief/service -run "Source|Candidate|Dedup|Ranking" -v
```

Expected:

- Source parsers succeed against local fixtures.
- Dedup logic collapses obvious duplicates across feeds.
- Ranking output is stable for the same candidate input.

## Task 5: Implement Structured Brief Generation and Transactional Publish

**Files:**
- Create: `internal/app/dailybrief/port/generator.go`
- Create: `internal/app/dailybrief/service/brief_generator.go`
- Create: `internal/app/dailybrief/service/prompt_builder.go`
- Create: `internal/app/dailybrief/service/output_schema.go`
- Create: `internal/app/dailybrief/service/publisher.go`
- Create: `internal/app/dailybrief/service/generation_orchestrator.go`
- Test: `internal/app/dailybrief/service/brief_generator_test.go`
- Test: `internal/app/dailybrief/service/generation_orchestrator_test.go`

- [ ] Reuse the existing AI runtime instead of introducing a feature-specific model client.

- [ ] Generate a structured output that maps cleanly onto persisted issue and item records:
  - one headline
  - one top summary
  - section list
  - item list with `summary` and `why_it_matters`

- [ ] Validate model output before publish. Treat malformed structured output as generation failure rather than writing a half-valid issue.

- [ ] Publish transactionally:
  - update issue record
  - replace item set
  - attach `published_run_id`
  - set generated and published timestamps

- [ ] Support degraded success when at least one source fails but enough valid candidates remain.

- [ ] Keep prompt version and model name on the generation run for later debugging.

Run:

```bash
go test ./internal/app/dailybrief/service -run "Generator|Publish|Orchestrator" -v
```

Expected:

- Invalid model output never reaches the published issue state.
- Publish replaces items atomically and leaves one stable issue for the day.

## Task 6: Implement Scheduler, Lease, and Automatic Retry

**Files:**
- Create: `internal/app/dailybrief/schedule/job.go`
- Create: `internal/app/dailybrief/schedule/lock_manager.go`
- Create: `internal/app/dailybrief/schedule/eligibility.go`
- Create: `internal/app/dailybrief/schedule/retry_policy.go`
- Create: `internal/app/dailybrief/schedule/processor.go`
- Create: `internal/bootstrap/dailybrief/runtime.go`
- Test: `internal/app/dailybrief/schedule/*_test.go`
- Test: `internal/bootstrap/dailybrief/runtime_test.go`

- [ ] Follow the existing knowledge schedule loop pattern:
  - periodic scan
  - bounded run timeout
  - background goroutine with panic recovery
  - clean shutdown on runtime close

- [ ] Decide eligibility using user-local time and user-local day, not server-local time.

- [ ] Skip generation when a successful issue already exists for the current local day.

- [ ] Acquire a subscription-scoped lease before scheduling work for a user-day.

- [ ] Create a new `GenerationRun` per attempt, but keep the same issue row for the same day.

- [ ] Retry only failed scheduled generation attempts.

- [ ] Apply a bounded retry policy aligned with the design:
  - default `2-3` retry attempts per user-day
  - backoff window such as `10m / 30m / 120m`
  - stop retrying after the user-day window has effectively passed

- [ ] Handle the failure classes separately:
  - source partial failure
  - source total failure
  - generation failure
  - publish failure

- [ ] Ensure a retry never creates a second issue row or a second conflicting uniqueness key.

Run:

```bash
go test ./internal/app/dailybrief/schedule ./internal/bootstrap/dailybrief -v
```

Expected:

- Scheduler produces at most one published issue per user-day.
- Failed runs create new `GenerationRun` attempts while reusing the same issue record.
- Lease ownership prevents concurrent duplicate work.

## Task 7: Add HTTP Handlers and Server Wiring

**Files:**
- Create: `internal/adapter/http/dailybrief/handler.go`
- Create: `internal/adapter/http/dailybrief/routes.go`
- Create: `internal/adapter/http/dailybrief/dto.go`
- Test: `internal/adapter/http/dailybrief/*_test.go`
- Modify: `cmd/server/main.go`

- [ ] Add authenticated user routes for:
  - `GET /daily-brief/today`
  - `GET /daily-brief/issues?date=YYYY-MM-DD`
  - `GET /daily-brief/subscription`
  - `PUT /daily-brief/subscription`

- [ ] Keep request and response shapes page-oriented and stable. Do not expose repository internals such as lock ownership or raw run rows.

- [ ] Return page-friendly states explicitly:
  - `empty`
  - `generating`
  - `failed`
  - `ready`

- [ ] Register the runtime in `cmd/server/main.go` alongside knowledge, ingestion, rag, and user runtimes.

- [ ] Add runtime close ordering so `dailybrief` shuts down gracefully.

Run:

```bash
go test ./internal/adapter/http/dailybrief ./cmd/server -v
```

Expected:

- Authenticated users can read and update subscriptions.
- Today and date-specific issue reads return stable page states.
- No regenerate API exists.

## Task 8: Build the Daily Brief Page and Navigation

**Files:**
- Create: `frontend/src/types/dailyBrief.ts`
- Create: `frontend/src/services/dailyBriefService.ts`
- Create: `frontend/src/pages/DailyBriefPage.tsx`
- Modify: `frontend/src/router.tsx`
- Modify: `frontend/src/components/layout/Sidebar.tsx`
- Optionally modify: `frontend/src/components/layout/MainLayout.tsx`

- [ ] Add a dedicated authenticated route, for example `/brief`, rather than embedding the brief into `/chat`.

- [ ] Reuse `MainLayout` so the page feels like a first-class product surface, not a detached admin screen.

- [ ] Add one stable sidebar entry for `Daily Brief`.

- [ ] Keep MVP state management page-local unless a cross-page need appears. Avoid adding a new global Zustand store if a page-scoped data flow is enough.

- [ ] Implement the four user-facing states:
  - empty subscription
  - generating
  - failed
  - ready

- [ ] Include these UI regions:
  - brief date and generation status
  - headline and top summary
  - sectioned item list
  - source badges and outbound links
  - date switcher for history
  - subscription settings form

- [ ] Do not add a regenerate button anywhere in the page.

- [ ] Keep the visual treatment intentional and distinct from chat while still respecting the current app shell.

Run:

```bash
cd frontend
npm run build
```

Expected:

- The page builds successfully.
- Routing and sidebar navigation work for authenticated users.
- The page handles all four backend states without layout breakage.

## Task 9: Add Observability, Fixtures, and Final Verification

**Files:**
- Create: `internal/app/dailybrief/service/metrics.go`
- Create: `internal/app/dailybrief/service/metrics_test.go`
- Create or modify: `testdata/dailybrief/*`
- Optionally create: `docs/daily-brief-mvp-notes.md`

- [ ] Add lightweight in-process counters or snapshot metrics for:
  - subscribed users scanned
  - successful runs
  - degraded runs
  - failed runs
  - retry attempts
  - per-source success and failure counts
  - candidate count before and after filtering
  - final item count

- [ ] Add structured logs for:
  - `user_id`
  - `brief_date`
  - `run_id`
  - `trigger_type`
  - `status`
  - failed source keys
  - candidate counts

- [ ] Keep observability internal for MVP. A user-facing metrics page is not required.

- [ ] Add fixture coverage for:
  - source parsing
  - candidate ranking
  - structured generation schema validation
  - retry eligibility

- [ ] Run a final verification pass that covers backend, server wiring, and frontend build.

Run:

```bash
go test ./internal/app/dailybrief/... ./internal/adapter/http/dailybrief ./internal/adapter/repository/postgres/dailybrief ./internal/bootstrap/dailybrief ./cmd/server -v
cd frontend
npm run build
```

Expected:

- The feature is test-covered at the domain, repository, scheduler, and HTTP layers.
- The frontend builds against the final API contract.

## Implementation Notes and Decisions

- Use the existing knowledge schedule pattern as the operational template for scan loop, lease ownership, and graceful shutdown.
- Keep source registry keys code-owned and validated server-side.
- Keep subscription settings explicit and deterministic. Do not infer interests from user chat history in this phase.
- Keep issue history simple. One user-day maps to one issue row, with retries represented only in `GenerationRun`.
- Reuse the same issue row on retry. This is how we avoid uniqueness conflicts when generation is retried after failure.
- If a retry succeeds later, it should update the same issue from `failed` or `generating` into `ready` instead of inserting a second issue.
- Use fixture-driven parser tests and mocked source collectors for stability. Do not depend on live upstream sites in CI.
- Prefer page-local frontend fetching over adding a second app-wide state machine.

## Suggested Commit Checkpoints

1. `feat: add daily brief config and schema`
2. `feat: add daily brief repositories and services`
3. `feat: add daily brief source pipeline and generation`
4. `feat: add daily brief scheduler and runtime wiring`
5. `feat: add daily brief http api and frontend page`

## Exit Criteria

- One subscribed user can receive one dated `Daily Brief` artifact for the current local day.
- Failed scheduled generation is retried automatically within a bounded policy.
- Retries reuse the same issue row and do not create uniqueness conflicts.
- Users can view today's brief, view history by date, and update subscriptions.
- The product exposes no user-triggered regeneration control.
- Backend tests and frontend build complete successfully.
