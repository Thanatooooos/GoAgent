# add-daily-brief-trend-insights Proposal

## Summary

This change adds a top-of-page `Trend Insight` module to `Daily Brief`.

The new module does not explain how the user's interests changed. It explains
how the subscribed domain itself is changing recently, using the incoming
external content stream as the evidence base.

Examples of the intended experience:

- AI recently shifted toward open-weight model releases and inference-cost
  competition.
- Agent tooling is no longer a side topic and is becoming a primary storyline.
- Policy and governance coverage is now strong enough to change the field's
  balance, not just add background noise.

The system should compute these changes internally from a short-window
comparison, calibrate them against a longer background window, and present them
to the user as natural language judgments with supporting evidence.

Phase 1 focuses on:

- one `Trend Insight` module at the top of the `Daily Brief` page
- one primary domain insight per page load
- internal short-window change detection
- longer-window background calibration
- evidence-backed signals rather than opaque prose
- one first-class deep template for `AI`

## Why

The current `Daily Brief` already delivers:

- curated topic subscriptions
- scheduled daily artifact generation
- structured sections and item lists
- stable page-based reading

However, it still behaves mainly as a high-quality daily roundup.

That leaves an important product gap:

- users can see what happened today
- users cannot quickly see what has changed in the domain recently
- users must infer structural shifts by reading many issues manually
- the product has no strong "analysis" layer that differentiates it from a
  normal summary surface

This change adds that missing layer. It turns `Daily Brief` from "today's
brief" into "today's brief plus what this domain is now turning into."

## Goals

This change aims to:

1. add a first-class `Trend Insight` surface at the top of the `Daily Brief`
   page
2. analyze domain change rather than user preference drift
3. compute change from recent incoming content using explicit comparison
   windows
4. keep user-facing wording natural and non-technical
5. back each major judgment with representative evidence items
6. use a reusable cross-domain analysis model plus one richer `AI` extension
7. keep the first release stable, explainable, and precomputed

## Non-Goals

This change does not include:

- explaining only how the user's reading preferences changed
- a generic market-wide analytics dashboard across every domain
- a real-time streaming trend wall or live ticker
- arbitrary user-created taxonomies for each topic
- requiring users to learn explicit `7d` / `30d` comparison terminology
- exposing raw scoring internals or statistical formulas in the UI
- replacing the normal `Daily Brief` artifact with a trend-only product
- broad support for deeply customized domain templates beyond `AI` in Phase 1

## Proposed Changes

### 1. Add a Top-of-Page Trend Insight Module

The `Daily Brief` page should start with a trend interpretation block before
the normal headline, summary, and section list.

This module should communicate:

- what recently changed in the subscribed domain
- which signals rose, faded, or emerged
- whether the recent shift aligns with or diverges from the broader background
- why the system believes this judgment

The product intent is to make the first screen answer:

> "What is this field recently turning into?"

### 2. Analyze Domain Change, Not Only Preference Change

The core object of analysis is the domain itself.

For example, in `AI`, the module should detect field changes such as:

- open-weight releases increasing
- closed API launches decreasing
- agent tooling rising
- policy, governance, and cost pressure gaining importance

The user's subscription and behavior still matter, but only to decide:

- which domain to prioritize
- which angle to emphasize
- how to phrase the "why it matters" explanation

The system should not mistake "the user clicked different things" for "the
field structurally changed."

### 3. Use Internal Time Windows But Natural External Language

The system should compare recent content internally using a primary short
window and a secondary longer background window.

Default direction:

- primary recent-change detection uses a short comparison window
- background calibration uses a longer comparison window

But the UI should avoid direct labels such as:

- `7d vs prev 7d`
- `30d vs prev 30d`

Instead it should use natural language such as:

- "Recently..."
- "Over this stretch..."
- "This is not only a one-off spike..."

This keeps the product feeling like analysis, not a raw analytics console.

### 4. Introduce Reusable Analysis Axes

The system should not require a fully custom hand-built taxonomy for every
topic.

Instead it should use:

- a reusable set of general analysis axes across domains
- a small number of domain-specific extension packs for priority topics

The reusable axes should capture cross-domain questions such as:

- what kind of event is this
- where in the value chain it sits
- how open or closed it is
- which actor type is driving it
- what kind of change it represents
- which region or scope it belongs to

### 5. Ship One Rich AI Template First

Phase 1 should treat `AI` as the first deep domain template.

`AI` should extend the general axes with field-specific dimensions such as:

- foundation model vs agent vs tooling vs infra vs application
- open weights vs API-only vs closed product vs benchmark/report
- capability race vs cost race vs deployment race vs governance pressure

The goal is to make the first release feel insight-rich in `AI`, not shallow
everywhere.

### 6. Precompute Trend Snapshots

Trend insight should not be assembled from scratch on every page request.

The system should:

1. label incoming brief candidates or published items with analysis facets
2. aggregate recent and background windows
3. synthesize one stable insight snapshot
4. serve the latest snapshot in the `Daily Brief` read path

This keeps the page responsive and keeps one day's interpretation stable.

### 7. Require Evidence-Backed Judgments

The module must not emit unsupported thematic claims.

Every major signal should be supported by:

- representative items, preferably published brief items
- recent counts or share movement
- explicit comparison to a prior period

If the sample is too small or too noisy, the system should soften or suppress
the claim rather than hallucinate certainty.

## Acceptance Criteria

This change is complete only when all of the following are true:

1. The `Daily Brief` page includes a top-of-page trend module before the normal
   brief artifact content.
2. The trend module explains recent domain change rather than only user
   preference change.
3. The system computes recent change using an internal short comparison window.
4. The system uses a longer background window to calibrate whether the recent
   change looks like noise or a larger shift.
5. The UI does not need to expose explicit `7d` or `30d` labels to users.
6. The module returns one analytical paragraph plus structured signals and
   evidence, not only one opaque blob.
7. The system supports a reusable set of general analysis axes across domains.
8. The system supports one richer `AI`-specific extension template in Phase 1.
9. The page can show one evidence-backed insight even when the normal brief is
   otherwise unchanged.
10. The system suppresses or downgrades claims when sample size is too small
    for a stable recent-change judgment.

## Risks

The main risks are:

- recent-window analysis may become noisy if thresholds are too loose
- fully LLM-driven trend judgment may sound plausible but be weakly grounded
- topic selection may feel arbitrary when a user subscribes to many domains
- generic axes may be too shallow without at least one strong domain template
- exposing too much methodology may make the UI feel like an internal console

Controls should include:

- explicit short-window and background-window separation
- structured axes before final LLM synthesis
- evidence-backed signals with representative item refs
- sample-size thresholds and confidence downgrade behavior
- Phase 1 focus on one strong `AI` template

## Open Questions

The main implementation-time questions are:

- how to choose the primary domain when a user subscribes to multiple active
  domains with comparable activity
- how often candidate fallback evidence should be used when published issue
  items do not adequately support a surfaced signal
- whether the first release should include user-facing affordances to switch
  domains manually

Default recommendation:

- choose one primary domain automatically in Phase 1
- point evidence to published or otherwise user-visible items when possible
- keep domain switching out of the first release unless usability testing shows
  it is essential
