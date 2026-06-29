# add-daily-brief Design

## Overview

This design defines the MVP implementation model for `add-daily-brief`.

The feature introduces a dedicated in-app `Daily Brief` product surface that
generates one dated AI and technology briefing artifact per subscribed user per
local day.

The MVP is intentionally narrow:

- external content only
- explicit topic and source subscriptions
- one dedicated page, not a chat-stream projection
- scheduled pre-generation
- automatic retry on failure
- no user-triggered regeneration

The feature is designed as a standalone business domain rather than an
extension of `rag` or `agent`, while still reusing existing scheduling,
frontend, persistence, and model invocation patterns already present in the
repository.

## Goals

This design aims to:

1. define a clean `dailybrief` domain boundary inside the existing repository
2. establish a stable artifact model for one user-day brief
3. support deterministic pre-LLM filtering before summarization
4. support scheduled generation with bounded automatic retry
5. make the read path simple for the frontend page
6. preserve a clean extension path for future delivery channels
7. keep the MVP small enough to implement and operate safely

## Non-Goals

This design does not include:

- user-triggered regeneration
- inferred interests or behavioral personalization
- internal knowledge-base content as a first-class source
- email, IM, or push delivery
- fine-grained engagement loops such as likes, saves, or shares
- versioned published issues for one day
- a generic plugin system for arbitrary third-party feeds

## Existing System Context

The current repository already provides several reusable patterns:

- `internal/app/*` domain-oriented application boundaries
- `internal/adapter/http/*` route and handler organization
- `internal/adapter/repository/postgres/*` domain-specific persistence layout
- existing schedule and job-style runtime loops in `knowledge`
- model invocation and summarization infrastructure in `rag`
- user and settings-related transport surfaces

This design should follow those boundaries instead of introducing a parallel
architecture.

The closest reusable operational pattern is not `rag_chat`; it is the existing
"domain service + schedule loop + persistence + HTTP surface" shape already
used elsewhere in the codebase.

## Design Principles

### 1. Domain First

`Daily Brief` is its own product capability. It should not be modeled as a
conversation, a tool call, or a knowledge document.

### 2. One User, One Day, One Published Artifact

The MVP should expose one current brief per `(user, brief_date)` pair.
Generation may happen multiple times internally through retries, but the
published read model remains singular.

### 3. Deterministic Before Generative

Fetching, normalization, deduplication, filtering, and ranking should be
deterministic. The LLM should operate on an already curated candidate set.

### 4. Schedule First

The product experience should feel proactive. Generation is initiated by the
system on schedule, not by the user opening the page or pressing a button.

### 5. Fail Usably

Partial source failure should still allow a usable brief when enough valid
candidates remain. Full failure should preserve state for retry and user-facing
status display.

### 6. Keep Read Paths Simple

The frontend should read one stable issue record plus its items. Internal retry
history and operational detail should stay behind the service boundary.

## Domain Layout

Recommended repository layout:

```text
internal/app/dailybrief
internal/adapter/http/dailybrief
internal/adapter/repository/postgres/dailybrief
internal/bootstrap/dailybrief
```

Responsibilities:

- `internal/app/dailybrief`
  - domain entities
  - services
  - source contracts
  - candidate pipeline
  - generator
  - scheduler job orchestration
- `internal/adapter/http/dailybrief`
  - page-facing handlers
  - request and response DTOs
- `internal/adapter/repository/postgres/dailybrief`
  - subscription, issue, item, and generation-run repositories
  - transactional publish logic
- `internal/bootstrap/dailybrief`
  - runtime assembly
  - schedule loop startup and shutdown

## Core Components

### SubscriptionService

Owns user subscription configuration:

- enabled
- timezone
- delivery time
- topics
- sources

This is the configuration source of truth.

### SourceCollector

Fetches external source content and normalizes all providers into a shared
candidate shape.

The collector owns source-specific fetching logic, but not final ranking or
brief rendering.

### CandidatePipeline

Applies:

- normalization
- deduplication
- topic filtering
- source filtering
- deterministic ranking

The pipeline produces a small, high-value candidate set for generation.

### BriefGenerator

Consumes ranked candidates and produces one structured brief artifact:

- headline
- top summary
- sections
- items
- why-it-matters context

### IssueService

Owns the user-day published brief lifecycle:

- create placeholder issue state
- publish final issue content
- replace item set transactionally
- read today's issue
- read historical issue by date

### GenerationRunService

Owns operational run records:

- scheduled run creation
- retry run creation
- status updates
- source stats
- error capture
- token and model metadata

### ScheduleJob

Scans enabled subscriptions and decides whether a user should receive today's
brief generation attempt.

## Data Model

The MVP should use four primary entities.

### Subscription

Stores per-user configuration:

- `user_id`
- `enabled`
- `timezone`
- `delivery_time_local`
- `topics_json`
- `sources_json`
- `created_at`
- `updated_at`

### Issue

Represents the one published brief for one user-day:

- `user_id`
- `brief_date`
- `status`
- `headline`
- `top_summary`
- `sections_json`
- `item_count`
- `published_run_id`
- `generated_at`
- `published_at`
- `created_at`
- `updated_at`

Required uniqueness:

```text
(user_id, brief_date)
```

This keeps the read model simple and avoids version management in the MVP.

### Item

Represents one displayed entry inside an issue:

- `issue_id`
- `section_key`
- `rank`
- `title`
- `summary`
- `why_it_matters`
- `url`
- `source`
- `topic`
- `published_at`
- `metadata_json`

### GenerationRun

Represents one generation attempt:

- `user_id`
- `brief_date`
- `trigger_type`
- `status`
- `started_at`
- `finished_at`
- `error_message`
- `source_stats_json`
- `model`
- `prompt_version`
- `token_usage_json`

Multiple runs may exist for one `(user_id, brief_date)`, but only one issue is
published for that day.

### Optional Candidate Persistence

Candidate-level persistence is optional in the MVP.

If added, it should remain an operational table, not part of the primary read
model. It is useful for debugging ranking and selection quality, but should not
block Phase 1 delivery.

## State Model

### Issue Status

Recommended statuses:

- `generating`
- `ready`
- `failed`

### GenerationRun Status

Recommended statuses:

- `running`
- `succeeded`
- `degraded`
- `failed`

### Failure Semantics

- If enough valid content remains after partial source failure:
  - `run = degraded`
  - `issue = ready`
- If no valid brief can be produced:
  - `run = failed`
  - `issue = failed`

Keeping a failed issue record is preferred over having no record at all,
because the page can distinguish "not generated yet" from "generation failed."

## Generation Flow

The generation pipeline should follow one user-day idempotent chain:

1. select eligible subscriptions
2. acquire a generation lease for `(user_id, brief_date)`
3. create a `GenerationRun(running)`
4. create or update `Issue(generating)`
5. collect candidates from configured sources
6. normalize, deduplicate, filter, and rank candidates
7. send the final candidate subset into structured LLM generation
8. publish issue content transactionally
9. finalize the run as `succeeded`, `degraded`, or `failed`

The candidate subset sent to the model should be intentionally limited. The
goal is a compact, high-signal brief, not exhaustive aggregation.

## Scheduling Model

The MVP should use a periodic scan model rather than per-user cron fan-out.

Recommended behavior:

- a schedule loop runs every `1-5` minutes
- it scans enabled subscriptions
- it resolves each user's local date and local delivery time using stored
  timezone
- it decides whether today's generation window has opened
- it skips users who already have a successful issue for the same day

This avoids a more complex distributed cron model while still supporting
user-local delivery semantics.

## Lease and Idempotency

The system needs three layers of protection.

### 1. Issue Uniqueness

`Issue` remains unique by `(user_id, brief_date)`.

### 2. Generation Lease

The scheduler or service layer should acquire a lease before executing the
generation pipeline so that concurrent workers do not generate the same
user-day brief simultaneously.

### 3. Run Attempt Model

Retries create new `GenerationRun` records rather than mutating historical
attempts. All attempts still converge on updating the same single published
issue.

This gives the system operational history without complicating the read path.

## Publish Semantics

Publishing must be transactional.

The following write operations should succeed or fail together:

- update `Issue`
- replace its current `Item` set
- link `published_run_id`

The system must not allow half-published state such as:

- a new issue summary with stale items
- replaced items without updated issue metadata
- a successful run record pointing at unpublished content

## Retry Model

The MVP should not expose user-triggered regeneration.

Retry exists only as an internal recovery mechanism for failed scheduled
generation.

Recommended policy:

- retry only failed generation attempts
- allow `2-3` retry attempts per user-day
- use increasing backoff such as `10m / 30m / 2h`
- stop retrying after the day-level validity window closes

This keeps load bounded while still handling transient failures.

## Failure Classification

Recommended classes:

- `source_partial_failure`
- `source_total_failure`
- `generation_failure`
- `publish_failure`

Expected outcomes:

- `source_partial_failure`
  - if enough content remains, publish `ready` and mark run `degraded`
- `source_total_failure`
  - mark run `failed`, mark issue `failed`
- `generation_failure`
  - mark run `failed`, mark issue `failed`
- `publish_failure`
  - mark run `failed`, preserve transactional integrity so the issue never
    lands in half-updated state

## API Design

The MVP page surface should remain narrow.

Recommended endpoints:

- `GET /daily-brief/today`
- `GET /daily-brief/issues?date=YYYY-MM-DD`
- `GET /daily-brief/subscription`
- `PUT /daily-brief/subscription`

No regeneration endpoint is required in Phase 1.

## Page Model

The first page should be optimized for "today first" reading, with lightweight
historical navigation.

Recommended sections:

- header with brief date, status, and last generated time
- headline and top summary
- sectioned item list
- history-by-date control
- subscription settings surface

## User-Facing States

The page should distinguish:

- `empty`
  - not subscribed, or generation window has not opened yet
- `generating`
  - scheduled generation is currently running
- `failed`
  - today's generation failed and may be retried automatically
- `ready`
  - today's issue is available

The page should not expose internal retry counters or raw backend errors in the
MVP.

## Observability

The MVP should emit enough signals to support rollout and debugging.

### Metrics

Recommended metrics:

- subscribed user count
- daily successful issue count
- daily failed issue count
- retry attempt count
- source fetch success rate by source
- candidate count before and after filtering
- final item count
- generation latency
- token usage

### Logs

Recommended log dimensions:

- `user_id`
- `brief_date`
- `run_id`
- `trigger_type = scheduled | retry`
- `status`
- `failed_sources`
- `candidate_count`
- `selected_count`

## Testing Strategy

### Unit Tests

- source normalization
- deduplication
- topic filtering
- deterministic ranking
- brief schema validation

### Repository Tests

- subscription CRUD
- issue uniqueness by `(user_id, brief_date)`
- item replacement transaction
- generation run persistence

### Job and Orchestration Tests

- schedule window eligibility
- lease acquisition prevents duplicate work
- partial source failure still yields a degraded success when enough candidates
  remain
- failed issue transitions into retry attempts
- bounded retry stops after capped attempts

### HTTP and Frontend Tests

- `today`
- `history by date`
- `subscription get`
- `subscription put`
- page rendering for `empty`, `generating`, `failed`, and `ready`

## Rollout Notes

The MVP rollout should favor operational simplicity:

1. land domain model and read/write APIs
2. land source collection and candidate pipeline
3. land scheduled generation for one curated source set
4. add automatic retry and observability
5. tune ranking and prompt structure after real traces

## Open Questions

The following questions remain acceptable as implementation-time choices:

- whether candidate-level persistence is worth the extra storage cost in Phase 1
- how aggressively to persist raw source text for debugging
- whether one or more source groups should be temporarily disabled during early
  rollout if their fetch reliability is poor

Default recommendation:

- candidate persistence remains optional
- persist final issue content and run metadata as required artifacts
- keep the first rollout source set curated and reversible rather than trying
  to support every source equally from day one
