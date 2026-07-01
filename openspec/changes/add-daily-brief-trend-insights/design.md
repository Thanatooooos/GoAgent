# add-daily-brief-trend-insights Design

## Overview

This design adds a `Trend Insight` interpretation layer to `Daily Brief`.

The module appears at the top of the existing page and answers a different
question from the rest of the brief:

- the normal brief answers "what happened"
- the trend insight answers "what recently changed in this field"

The design deliberately separates:

- internal comparison mechanics
- structured evidence and aggregation
- user-facing natural language output

The first release should optimize for clarity, stability, and evidence-backed
judgment rather than real-time freshness or broad cross-domain depth.

## Goals

This design aims to:

1. define the user-facing `Trend Insight` module shape
2. define the internal analysis pipeline for recent domain change
3. define reusable general analysis axes plus one richer `AI` extension
4. define update cadence and comparison-window responsibilities
5. define one stable read contract for the frontend
6. keep the implementation precomputed and explainable

## Non-Goals

This design does not include:

- live real-time trend streaming
- arbitrary custom axis authoring per topic
- full multi-topic switching in the first release
- a general BI dashboard with charts-first UX
- replacing deterministic aggregation with pure free-form LLM judgment

## Product Experience

### Placement

`Trend Insight` appears at the top of the `Daily Brief` page, above the normal
issue headline and top summary.

The placement is intentional: users should first see the recent change in the
field, then consume the day's concrete items.

### User-Facing Content

The module should present:

- one main interpretation paragraph
- two or three compact signal cards
- one light background sentence
- one optional evidence expand area

The module should **not** visibly expose internal time-window labels such as
`7d`, `30d`, or `vs prev period`.

Instead, user-facing wording should feel natural:

- "Recently..."
- "Over this stretch..."
- "This now looks like..."
- "This is not only a one-off spike..."

### Example Shape

Example rendering:

- Main paragraph:
  - "Recently, AI has shifted away from pure model-release theater toward a
    mix of open-weight supply, inference-cost competition, and stronger agent
    tooling focus. Open releases are more visible than before, while benchmark
    talk is no longer the only organizing storyline. At the same time,
    deployment and governance constraints are appearing often enough to shape
    the field's center of gravity rather than just its edges."
- Signal cards:
  - "Open-weight releases are more prominent"
  - "Agent tooling is now a primary storyline"
  - "Governance pressure is no longer background noise"
- Background sentence:
  - "This recent swing also aligns with the broader direction of the last
    stretch, rather than looking like a one-day anomaly."
- Evidence drawer:
  - representative items grouped by signal

The paragraph is the primary content surface. The signal cards are supporting
scan-friendly distillations rather than the main event.

## Package Boundary

Trend analysis should not continue growing the existing
`internal/app/dailybrief/service` package.

Recommended layout:

```text
internal/app/dailybrief/domain
internal/app/dailybrief/service
internal/app/dailybrief/trend
internal/adapter/repository/postgres/dailybrief
internal/adapter/http/dailybrief
internal/bootstrap/dailybrief
```

Boundary intent:

- `domain`
  - core `Daily Brief` entities such as subscription, issue, item, and
    generation run
- `service`
  - existing brief generation, subscription, read, and publish flows
- `trend`
  - one analysis subdomain for observation labeling, aggregation, signal
    detection, evidence selection, synthesis, and snapshot assembly

`trend` should be a sibling package to `service`, not a nested `service/trend`
subtree.

## Topic Selection

Phase 1 should render one primary trend insight per page load.

Recommended selection policy:

1. start from the user's subscribed topics
2. collapse them into top-level domains
3. rank domains by recent visible activity and signal strength
4. choose the strongest eligible domain

The system should suppress the module when:

- the user has no eligible domain subscription
- recent evidence is below the sample threshold
- the content mix is too weak to support a meaningful claim

## Internal Analysis Model

The analysis pipeline should follow five stages:

1. observation labeling
2. window aggregation
3. change detection
4. evidence selection
5. synthesis into one stable snapshot

### 1. Observation Labeling

Each normalized candidate should be converted into a structured
`TrendObservation`, and trend evidence should prefer already visible published
brief items whenever possible.

Recommended fields:

- `topic`
- `source`
- `published_at`
- `brief_date`
- `event_type`
- `value_chain_position`
- `openness`
- `actor_type`
- `change_nature`
- `region_scope`
- optional domain-specific facets

This labeling may use mixed techniques:

- rule extraction when signals are deterministic
- LLM schema extraction when semantic interpretation is required
- fallback `unknown` values when confidence is too low

The design must allow partial labeling. Not every item must populate every
axis.

### 2. Window Aggregation

The system should aggregate observations over two window families:

- primary recent-comparison window
- background calibration window

Recommended default responsibilities:

- primary short window:
  - detect recent rises, drops, and new signals
- longer window:
  - determine whether the short-window swing reinforces or contradicts the
    broader direction

The internal default may use a recent short comparison and a longer background
comparison, but those terms remain internal implementation detail.

Aggregation outputs should include:

- count by facet value
- share by facet value
- delta vs prior comparison window
- continuity score across consecutive buckets
- evidence refs for each signal

### 3. Change Detection

The system should not send all raw counts directly into the final LLM prompt.

Instead it should first detect structured signal candidates such as:

- `rising`
- `falling`
- `new`
- `persistent`
- `shift`

Each signal candidate should carry:

- signal title seed
- supporting facet values
- count/share movement
- confidence or stability flags
- representative evidence refs

This keeps synthesis grounded and auditable.

### 4. Evidence Selection

Every major user-facing signal must be attached to representative content.

Evidence selection should favor:

- recency
- clear facet match
- source diversity when possible
- items that are already visible or linkable in the product

Phase 1 should prefer evidence that can be shown to the user directly:

- brief items from recent issues
- otherwise cleanly normalized source items only as fallback

Phase 1 should therefore use this evidence order:

1. recent published `brief item`
2. cleaned candidate-level evidence only when a necessary signal is not well
   represented in published items

### 5. Snapshot Synthesis

The final synthesis step should produce one `TrendInsightSnapshot`.

The LLM's role here is:

- summarize structured change
- explain why it matters
- phrase the judgment naturally as one short analytical paragraph plus concise
  supporting signal labels

The LLM's role is **not**:

- infer trend directly from unconstrained raw content
- invent unsupported structure
- bypass sample-size and evidence thresholds

## General Analysis Axes

Phase 1 should ship one reusable cross-domain axis set.

### Event Type

- `research`
- `release`
- `tooling`
- `policy`
- `capital`
- `incident`

This answers "what kind of event is dominating recently."

### Value Chain Position

- `upstream`
- `midstream`
- `downstream`

This answers whether the field is moving closer to base capability,
platform/tooling, or applied use.

### Openness

- `open`
- `closed`
- `hybrid`
- `unknown`

This supports judgments such as open-weight vs closed-product dominance.

### Actor Type

- `lab`
- `bigtech`
- `startup`
- `community`
- `government`
- `enterprise`

This shows who is currently driving visible movement.

### Change Nature

- `incremental`
- `breakthrough`
- `competitive`
- `standardizing`
- `risk_exposed`
- `adoption_expanding`

This helps interpret whether the field feels exploratory, operational, or
constrained.

### Region Scope

- `china`
- `us`
- `europe`
- `global`
- `cross_border`

This helps explain whether the recent shift is regional or broadly distributed.

## AI-Specific Extension Axes

Phase 1 should include one richer template for `AI`.

### Capability Layer

- `foundation_model`
- `reasoning_model`
- `multimodal`
- `agent`
- `tooling_framework`
- `infra_serving`
- `application`

### Release Form

- `open_weights`
- `open_source_stack`
- `api_only`
- `closed_product`
- `benchmark_or_report`

### Competitive Theme

- `capability_race`
- `cost_race`
- `speed_race`
- `context_race`
- `agent_experience`
- `enterprise_adoption`
- `safety_governance`

### Adoption Direction

- `research_demo`
- `developer_tool`
- `enterprise_solution`
- `consumer_feature`
- `industry_workflow`

### Constraint Source

- `compute`
- `data`
- `policy`
- `copyright`
- `distribution`
- `reliability`

These extensions should allow the system to say things that feel native to the
field, such as:

- open-weight releases are gaining share
- cost competition is becoming a stronger storyline
- agent tooling is moving from niche to primary
- enterprise adoption is rising alongside governance scrutiny

## Update Cadence

Trend insight should be computed on a schedule, not on every page request.

Recommended cadence:

- content ingestion continues on the normal `Daily Brief` pipeline cadence
- recent-window trend snapshots refresh on a daily schedule
- the longer background snapshot also refreshes daily
- the page reads the latest stable snapshot

This keeps one day's interpretation stable and makes the product feel
editorially coherent rather than jittery.

The internal recent-comparison window is the primary signal driver. The longer
background window only calibrates whether the recent shift appears transient or
structural.

## Sample Thresholds and Confidence

The system must not emit high-confidence change claims on weak samples.

Phase 1 should enforce:

- minimum observation count for the chosen domain
- minimum supporting evidence count for each surfaced signal
- downgrade or suppression when data is too sparse

Recommended behaviors:

- if the domain is below threshold, omit the module
- if one signal is below threshold, suppress that signal only
- if recent-window change conflicts strongly with background and the sample is
  unstable, use softer language

## Read Model

The frontend should consume one stable `TrendInsight` object.

Suggested shape:

```ts
type TrendInsight = {
  topic: string
  generatedAt: string
  interpretation: string
  signals: TrendSignal[]
  background: TrendBackground | null
  evidenceGroups: EvidenceGroup[]
  methodology: TrendMethodology | null
}

type TrendSignal = {
  kind: "rising" | "falling" | "new" | "persistent" | "shift"
  title: string
  summary: string | null
  tags: string[]
}

type TrendBackground = {
  summary: string
  alignsWithRecent: boolean
}

type EvidenceGroup = {
  title: string
  summary: string
  items: EvidenceItem[]
}

type EvidenceItem = {
  title: string
  url: string
  source: string
  topic: string
  publishedAt?: string
  briefDate?: string
}

type TrendMethodology = {
  summary: string
  sampleSize: number
}
```

The `methodology.summary` should stay human-readable and non-technical, for
example:

- "This judgment is based on recent domain coverage compared with an earlier
  stretch, using topic structure and representative source items."

The UI does not need to render the methodology by default.

## API Contract

Phase 1 should extend the existing page read path rather than introduce a
fully separate entrypoint for the default experience.

Recommended contract:

- `GET /daily-brief/today`
  - add `trendInsight?: TrendInsight | null`
- `GET /daily-brief/issues?date=YYYY-MM-DD`
  - optional future extension to return historical trend snapshots for that
    date

The page can then load one response and render:

- top trend insight
- current issue content
- existing page state and subscription controls

## Persistence Model

Phase 1 should add two internal concepts.

### TrendObservation

Stores structured labeling for one normalized content item.

Suggested responsibility:

- internal analytic record
- not directly user-facing
- primarily derived from candidate-level normalized content
- may later be linked to published item evidence when a user-visible reference
  exists

### TrendInsightSnapshot

Stores the synthesized insight for one topic and one generation moment.

Suggested fields:

- `topic`
- `interpretation`
- `signals_json`
- `background_json`
- `evidence_json`
- `methodology_json`
- `generated_at`
- `recent_window_ref`
- `background_window_ref`

The page should read the latest snapshot instead of triggering new synthesis.
`background_json` should be persisted as part of the same snapshot instead of
being managed as a separate first-phase read model.

## Failure and Degradation

The feature should fail gracefully.

Expected behaviors:

- if no stable snapshot exists, render the normal brief page without the trend
  module
- if evidence exists but confidence is weak, render a softer interpretation
- if one evidence group is missing, still allow the main insight if the
  remaining support is sufficient

The trend module must never block the normal brief page from loading.

## Testing Strategy

### Unit Tests

- general-axis labeling
- AI-specific facet extraction
- recent-window aggregation
- background calibration logic
- signal suppression on low sample size

### Snapshot and Service Tests

- snapshot synthesis uses only supported signal candidates
- evidence groups contain representative linked items
- low-confidence inputs downgrade wording or suppress output
- the latest snapshot is selected for page reads

### HTTP and Frontend Tests

- page renders with trend insight and issue content
- page renders normally when trend insight is absent
- evidence expand area renders supported groups
- methodology stays optional and non-blocking

## Rollout

Recommended rollout order:

1. define data contracts and persistence model
2. introduce the sibling `trend` package boundary
3. implement general axes
4. implement one `AI` extension template
5. build recent-window aggregation and background calibration
6. synthesize and persist snapshots
7. expose snapshot through `GET /daily-brief/today`
8. render the top-of-page module
9. tune thresholds and wording using real traces

## Open Questions

The remaining product decisions are modest and should not block the overall
direction:

- whether Phase 1 should pin the first rollout only to `AI`
- whether users need a manual "show me another domain" affordance in the first
  UI
- whether methodology should be visible only in the evidence drawer or also in
  a subtle footer line

Default recommendation:

- launch the first polished experience with `AI`
- avoid manual domain switching in Phase 1 unless real usage demands it
- keep methodology hidden by default and available only when the user opens
  supporting detail
