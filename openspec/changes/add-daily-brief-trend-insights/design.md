# add-daily-brief-trend-insights Design

## Overview

This design adds a `Trend Insight` interpretation layer to `Daily Brief`.

The module appears at the top of the existing page and answers a different
question from the rest of the brief:

- the normal brief answers "what happened"
- the trend insight answers "what this field currently looks like, what
  recently changed, and what that change suggests"

The design deliberately separates:

- internal comparison mechanics
- structured evidence and aggregation
- user-facing natural language output

The first release should optimize for clarity, stability, and evidence-backed
judgment rather than real-time freshness or broad cross-domain depth.

## Goals

This design aims to:

1. define the user-facing `Trend Insight` module shape
2. define the internal analysis pipeline for recent domain state and change
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

The main paragraph should combine three layers into one natural piece of
analysis:

- `summary`
  - what the field currently looks like over the recent stretch
- `change`
  - what shifted relative to the previous comparable stretch
- `implication`
  - what that shift likely means for how the field should now be read

### Example Shape

Example rendering:

- Main paragraph:
  - "Recently, AI no longer looks like a field organized only around flagship
    model launches. It now reads more like a mix of open-weight supply,
    inference-cost competition, agent tooling, and practical deployment
    concerns. Compared with the previous stretch, open releases and
    engineering-oriented signals are more prominent, while pure benchmark
    storytelling is less dominant. That suggests the center of gravity is
    moving from 'who is strongest' toward 'what is usable, affordable, and
    easier to fit into real workflows.'"
- Signal cards:
  - "Open-weight releases are more prominent"
  - "Agent tooling is now a primary storyline"
  - "Governance pressure is no longer background noise"
- Background sentence:
  - "This recent swing also aligns with the broader direction of the last
    stretch, rather than looking like a one-day anomaly."
- Evidence drawer:
  - representative items grouped by signal

The paragraph is the primary content surface. It should read like a compact
domain judgment rather than a headline expansion. The signal cards are
supporting scan-friendly distillations rather than the main event.

### Tone and Expression

Phase 1 should adopt a `light-editorial, analysis-first` writing style.

This means:

- the paragraph should feel like a concise editorial judgment
- the judgment must still be grounded in structured signals and evidence
- the language should sound confident but not overclaim

Recommended paragraph shape:

1. sentence one summarizes what the field currently looks like
2. sentence two explains what shifted relative to the prior stretch
3. sentence three interprets what that shift likely means

Recommended language characteristics:

- use phrasing such as "is starting to look more like", "is shifting toward",
  "is becoming more visible", and "is no longer the only center of gravity"
- prefer comparative and directional language over absolute declarations
- preserve room for uncertainty when evidence is mixed

Avoid:

- overly dramatic certainty such as "clearly", "completely", or "has already
  proven"
- generic analyst filler that reads like a template-generated weekly report
- unsupported causal claims when the system only observes correlation and
  salience change

The signal cards should stay shorter and more direct than the main paragraph.
The evidence drawer should become more neutral and factual, rather than
repeating the editorial tone of the top-level judgment.

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

The analysis pipeline should follow six stages:

1. observation labeling
2. recent-state summarization
3. window aggregation
4. change detection
5. evidence selection
6. synthesis into one stable snapshot

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

### 2. Recent-State Summarization

Before asking what changed, the system should first establish what the domain
currently looks like.

This stage should summarize the recent window itself and answer questions such
as:

- which event types are currently dominant
- which capability or value-chain layers are most visible
- which actor groups are most active
- which competitive themes now organize the field

This summary is not yet a historical comparison. It is a structured description
of the recent domain state.

Suggested outputs:

- dominant facets
- secondary supporting facets
- concentration score
- candidate "current storyline" labels

This stage should materialize one internal object:

```ts
type CurrentStateSummary = {
  summary: string
  dominantThemes: string[]
  supportingThemes: string[]
  dominantFacets: string[]
  concentrationScore: number | null
}
```

Its job is to describe the recent domain state without yet making a historical
comparison.

### 3. Window Aggregation

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

### 4. Change Detection

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

This stage should materialize one internal object:

```ts
type DomainChangeSet = {
  risingThemes: string[]
  fadingThemes: string[]
  emergingThemes: string[]
  persistentThemes: string[]
  primaryShift: string | null
}
```

The goal is not to preserve every measurable delta. The goal is to keep only
the changes that are strong enough to support user-facing interpretation.

The combination of recent-state summarization and change detection is
important:

- `summary` prevents the module from sounding like isolated delta reporting
- `change` prevents the module from becoming a generic static overview

The product should do both.

### 5. Evidence Selection

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

### 6. Snapshot Synthesis

The final synthesis step should produce one `TrendInsightSnapshot`.

The LLM's role here is:

- summarize the current domain state
- summarize structured change
- explain why the combined state-and-change judgment matters
- phrase the judgment naturally as one short analytical paragraph plus concise
  supporting signal labels

The LLM's role is **not**:

- infer trend directly from unconstrained raw content
- invent unsupported structure
- bypass sample-size and evidence thresholds

The synthesis step should use one additional internal object:

```ts
type InterpretationFrame = {
  primary: string
  secondary: string | null
  summaryFocus: string
  changeFocus: string
  implicationFocus: string
  tone: "light_editorial_analysis"
  cautionLevel: "high" | "medium" | "low"
}
```

`InterpretationFrame` should be derived before free-form writing begins. Its
job is to constrain what kind of explanation the model is allowed to produce.

Phase 1 should use a small bounded frame set rather than open-ended
interpretation categories.

Recommended frame set:

- `capability_to_utility`
  - the field is shifting from raw capability competition toward usability,
    deployment, cost, and workflow fit
- `open_to_closed_balance_shift`
  - the field is rebalancing between open supply and closed platform control
- `experimentation_to_adoption`
  - the field is moving from demos and exploration toward real usage and
    business absorption
- `acceleration_to_constraint`
  - the field is still advancing, but external constraints such as policy,
    reliability, copyright, or compute are now entering the main storyline
- `fragmentation_to_structure`
  - the field is becoming more legible, with clearer dominant themes and less
    purely fragmented motion

Phase 1 should choose:

- exactly one `primary` frame
- at most one `secondary` frame

The primary frame should carry the main implication sentence. The secondary
frame may add nuance, but should not compete with the primary explanation.

Recommended frame triggers:

- `capability_to_utility`
  - stronger `tooling_framework`, `infra_serving`, `developer_tool`,
    `enterprise_solution`, or applied workflow signals
  - weaker relative dominance of pure `benchmark_or_report` or flagship model
    theater
- `open_to_closed_balance_shift`
  - strong movement in `open_weights`, `open_source_stack`, `api_only`, or
    `closed_product`
- `experimentation_to_adoption`
  - rising `enterprise_solution`, `industry_workflow`, or `consumer_feature`
  - declining dominance of `research_demo`
- `acceleration_to_constraint`
  - rising `policy`, `risk_exposed`, `compute`, `copyright`,
    `distribution`, or `reliability`
- `fragmentation_to_structure`
  - stronger persistence, clearer concentration, and lower thematic scatter in
    the recent state summary

The implementation should prefer explicit mapping logic from structured signals
to frames. The LLM may help phrase the chosen frame, but should not invent a
new frame family at runtime.

### Frame Priority and Conflict Resolution

Multiple frames may appear valid at the same time. Phase 1 should therefore
define a stable selection rule rather than letting the writer combine every
plausible interpretation.

Recommended selection procedure:

1. score each frame from structured signals
2. suppress frames below the confidence floor
3. select the highest-confidence frame as `primary`
4. allow one `secondary` frame only when it adds real nuance without
   reframing the whole paragraph
5. otherwise collapse to one-frame output

Recommended priority order when scores are close:

1. `capability_to_utility`
2. `experimentation_to_adoption`
3. `open_to_closed_balance_shift`
4. `acceleration_to_constraint`
5. `fragmentation_to_structure`

This ordering reflects product value for Phase 1:

- prefer frames that explain where practical attention is moving
- prefer frames that help users understand what to watch next
- use structure-only frames as support, not as the first interpretation when a
  more concrete shift is available

Recommended secondary-frame rules:

- `fragmentation_to_structure`
  - may support almost any primary frame when the field is becoming more
    legible
- `open_to_closed_balance_shift`
  - may support `capability_to_utility` when openness changes are important but
    not the main implication
- `acceleration_to_constraint`
  - may support `experimentation_to_adoption` when adoption is rising under
    visible operational or policy pressure

Recommended exclusion rules:

- do not pair `capability_to_utility` and `experimentation_to_adoption` as
  equal co-primaries
  - choose the one with stronger movement and absorb the other as wording
- do not use `fragmentation_to_structure` as primary when a stronger causal or
  directional frame is available
- do not emit a `secondary` frame when evidence only weakly supports it

When conflict remains unresolved after scoring, the system should prefer:

- the frame with clearer evidence diversity
- then the frame with stronger persistence across adjacent buckets
- then the earlier frame in the priority order above

The final paragraph should still read as one coherent judgment. It should not
sound like a stitched list of multiple independent analyses.

Recommended synthesis order:

1. build `CurrentStateSummary`
2. build `DomainChangeSet`
3. choose `InterpretationFrame`
4. generate the final paragraph and supporting labels

This keeps the system in a `structure first, wording second` mode.

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
  currentSummary: TrendCurrentSummary | null
  signals: TrendSignal[]
  background: TrendBackground | null
  evidenceGroups: EvidenceGroup[]
  methodology: TrendMethodology | null
}

type TrendCurrentSummary = {
  summary: string
  dominantThemes: string[]
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

The outward read model may stay compact, but the internal pipeline should
distinguish:

- `CurrentStateSummary`
  - what the field currently looks like
- `DomainChangeSet`
  - what changed relative to the prior stretch
- `InterpretationFrame`
  - how the final explanation should be shaped and limited

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

The snapshot should preserve enough structure to distinguish:

- what the system thinks the field currently looks like
- what the system thinks changed
- how those combine into the final displayed paragraph

The implementation should therefore avoid sending raw item lists directly into
the final writing prompt. It should send a bounded interpretation payload built
from `CurrentStateSummary`, `DomainChangeSet`, evidence references, and tone
constraints.

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
