# add-daily-brief Proposal

## Summary

This change introduces a first-productized `Daily Brief` feature for `goagent`
as a standalone in-app briefing surface rather than an extension of the normal
chat flow.

The MVP delivers:

- one dedicated `Daily Brief` page in the product
- explicit user subscriptions for topics and sources
- daily brief generation focused on external AI and technology content
- scheduled pre-generation for each subscribed user
- automatic retry when scheduled generation fails
- historical brief viewing by date

The goal is not to build a full recommendation platform in one step. The goal
is to establish one reliable daily briefing loop that is easy to operate,
explain, and extend.

## Why

The current repository already has several ingredients that make a `Daily
Brief` feasible:

- user and settings surfaces
- HTTP and frontend application structure
- scheduling patterns in existing domains
- RAG, summarization, and model invocation infrastructure
- existing work around long-term preference and structured summaries

However, there is no product surface today that proactively organizes external
AI and technology updates into a stable, user-facing daily artifact.

Without a dedicated briefing domain:

- relevant external updates stay fragmented across search and chat flows
- users must repeatedly ask for the same daily roundup
- there is no subscription model, generation history, or failure recovery path
- future delivery channels such as email or IM would have no clean producer

This change turns the system from a purely reactive assistant into a product
that can also deliver recurring value on its own schedule.

## Goals

This change aims to:

1. establish `Daily Brief` as a standalone product surface with its own page,
   storage model, and generation lifecycle
2. support explicit per-user subscriptions for topics and approved sources
3. generate one in-app brief per subscribed user per local day
4. pre-generate briefs on a schedule and automatically retry failed generation
   attempts
5. focus the MVP on external AI and technology content from a narrow, curated
   source set
6. keep the generation loop observable, retryable, and historically queryable
7. preserve a clean extension path for future delivery channels without mixing
   brief history into the normal chat conversation model

## Non-Goals

The MVP does not include:

- automatic interest inference from user history or behavior
- personalized ranking learned from clicks or long-term engagement
- email, Feishu, push notification, or other external delivery channels
- a broad open-ended source marketplace
- arbitrary user-defined feeds or scraping rules
- a full recommendation engine or social/newsfeed product surface
- mixing brief entries into the default chat conversation stream
- deep in-brief feedback loops such as likes, saves, shares, or annotations
- user-triggered regeneration of the current day's brief
- internal knowledge-base-driven brief generation as a first-class source in
  this change

## Proposed Changes

### 1. Dedicated Daily Brief Domain

The system should introduce a standalone `dailybrief` business domain instead
of placing the feature inside `rag` or `agent`.

This domain owns:

- subscription settings
- brief issue lifecycle
- brief item persistence
- generation run tracking
- source ingestion orchestration
- scheduled generation and automatic retry entrypoints

The product meaning of a brief is not "a conversation" and not "a knowledge
document." It is a dated, generated briefing artifact for one user.

### 2. Subscription-First Personalization

The MVP should use explicit subscriptions rather than inferred interests.

Each user can configure:

- whether daily briefing is enabled
- timezone
- preferred local delivery time
- selected topics
- selected sources

The first supported content posture is "external AI and technology updates,"
with the initial source set limited to:

- Hacker News
- GitHub Trending
- arXiv `cs.AI`, `cs.CL`, `cs.LG`
- Papers with Code
- official blogs from OpenAI, Anthropic, Google DeepMind, and Meta AI
- The Decoder
- VentureBeat AI
- TechCrunch AI

### 3. Page-First Product Surface

The MVP should ship a dedicated `Daily Brief` page rather than rendering briefs
as ordinary chat messages.

The page should support:

- viewing today's brief
- viewing historical briefs by date
- seeing generation state such as ready, generating, failed, or empty
- opening source links for individual items
- updating subscription settings

This keeps the brief lifecycle separate from chat semantics and gives the
feature a stable product identity.

### 4. Scheduled Pre-Generation With Automatic Retry

The generation model should be schedule-first:

- background scheduled pre-generation is the default path
- failed generation may be retried automatically by the system

The scheduled path should:

- scan enabled subscriptions on a recurring interval
- evaluate each user's local time window using stored timezone and delivery
  time
- generate the current day's brief only when no successful issue exists yet

The retry path should:

- reuse the same generation pipeline
- stay internal to the system rather than introducing a user-facing action
- stop after a bounded retry policy to avoid runaway load

This preserves the "daily push" product feel while still giving users a
recovery path when transient source or model failures occur.

### 5. Structured Brief Artifact

The output should be stored as a structured brief issue rather than a single
opaque blob.

The generated artifact should contain:

- one overall headline
- one top summary
- a small number of sections
- ranked brief items inside each section
- per-item source, URL, summary, and "why it matters" context

The MVP should prefer a compact, high-signal brief over exhaustive coverage.
The target shape is a daily artifact that can be read quickly and trusted, not
a dump of every fetched item.

### 6. Deterministic Pre-LLM Filtering

The generation pipeline should not send raw source volume directly into the
model.

The system should first:

- fetch candidates from the curated source set
- normalize them into one internal candidate shape
- deduplicate by URL, external identifier, and title similarity
- filter by selected topics and source choices
- rank by freshness, topic match, source priority, and product heuristics

Only the highest-value candidate subset should reach the LLM summarization
stage. This keeps cost, latency, and drift under control.

### 7. Run History and Operability

The MVP should persist brief generation runs separately from published brief
issues.

Generation history must make it possible to understand:

- whether generation succeeded or failed
- which trigger path was used: scheduled or retry
- which sources returned content
- whether the brief was generated in a degraded mode
- which model and prompt version were used

This is necessary for future debugging, prompt changes, and delivery expansion.

## Acceptance Criteria

This change is complete only when all of the following are true:

1. There is a dedicated `Daily Brief` product surface separate from the normal
   chat conversation flow.
2. Users can enable or disable `Daily Brief` and configure timezone, delivery
   time, topics, and sources.
3. The MVP supports the curated external source set defined in this proposal.
4. The system can generate one dated brief artifact per subscribed user per
   day.
5. The scheduled path can pre-generate a brief for the current day based on
   user-local schedule settings.
6. Failed generation attempts can be retried automatically without introducing
   a user-facing regeneration control.
7. Users can view today's brief and prior briefs by date.
8. Each brief stores structured sections and ranked items, not just one
   unstructured text blob.
9. The generation pipeline performs deterministic candidate normalization,
   deduplication, filtering, and ranking before LLM summarization.
10. Generation runs are persisted with enough metadata to diagnose failures and
    degraded outputs.
11. A single-source fetch failure does not necessarily fail the entire brief if
    enough valid candidates remain.
12. The feature leaves a clear extension path for future delivery channels such
    as email or IM without redefining the brief artifact model.

## Risks

The main risks of this change are:

- source integrations may be brittle because upstream sites and feeds change
- low-quality ranking may cause the brief to feel noisy or repetitive even when
  generation succeeds
- LLM summarization may over-compress or drift away from the most important
  source facts
- per-user scheduled generation may create operational pressure if not bounded
  carefully
- mixing multiple source types may make the brief feel uneven without a clear
  sectioning policy

Controls should include:

- a curated and intentionally narrow source set for the MVP
- deterministic candidate filtering before model generation
- structured output validation for the brief artifact
- degraded-but-usable generation when partial source fetches fail
- generation run history for debugging and prompt iteration
- bounded automatic retry policy with backoff or capped attempts

## Open Questions

The following implementation questions remain open, but the default direction
in this proposal is already chosen:

- Should the MVP persist source candidates for every run, or only final brief
  items plus run-level source stats
- How much raw source text should be stored for later debugging versus trimmed
  for cost and privacy reasons
- Whether the first page layout should optimize for timeline browsing or for a
  "today first" reading experience

Default recommendation:

- persist final issues, items, and generation runs as required records
- treat candidate-level persistence as optional but recommended if the added
  storage cost is acceptable
- optimize the first page for "today first," with lightweight historical date
  navigation
