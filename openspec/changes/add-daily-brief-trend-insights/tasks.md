# add-daily-brief-trend-insights Tasks

## Scope

Phase 1 should stay intentionally narrow.

This change delivers:

- one top-of-page `Trend Insight` module on `Daily Brief`
- one `AI`-first rollout target
- one short analytical paragraph driven by `summary + change + implication`
- up to two supporting signal cards
- one optional evidence expand area
- one precomputed snapshot read path through `GET /daily-brief/today`

This change does not deliver:

- multi-domain switching in the first UI
- historical trend playback through `GET /daily-brief/issues`
- more than one primary interpretation frame per snapshot
- formal persistence of every intermediate observation record
- full cross-domain template coverage beyond the reusable base plus `AI`

## 1. Freeze Phase-1 Product Contract

- [ ] Confirm the `Trend Insight` module renders at the top of the `Daily Brief`
      page, above the normal issue summary.
- [ ] Confirm the main content is one compact analytical paragraph rather than
      a short headline.
- [ ] Confirm the paragraph combines `summary`, `change`, and `implication`.
- [ ] Confirm Phase 1 surfaces at most two signal cards.
- [ ] Confirm user-facing wording must avoid explicit internal labels such as
      `7d`, `30d`, or `vs previous period`.
- [ ] Confirm the normal brief remains fully usable when the trend module is
      absent.

Verification:

- proposal, design, and spec delta describe the same top-of-page experience
- no product text drifts into user-preference change analysis

## 2. Freeze Phase-1 Analysis Scope

- [ ] Pin the first rollout target to `AI`.
- [ ] Keep one reusable base axis set, but treat only the minimum Phase-1 axes
      as required:
      `event_type`, `openness`, and `actor_type`.
- [ ] Keep `value_chain_position`, `change_nature`, and `region_scope` as
      optional extension axes rather than mandatory first-pass extraction.
- [ ] Confirm the `AI` extension axes remain the primary rich template for
      Phase 1.
- [ ] Define allowed `unknown` / partial-label behavior.

Verification:

- the first implementation can ship with a small stable extraction surface
- AI-specific analysis still has enough structure to feel meaningfully better
  than generic tagging

## 3. Freeze Internal Objects and Generation Chain

- [ ] Define `CurrentStateSummary` as the recent-window state object.
- [ ] Define `DomainChangeSet` as the recent-vs-prior change object.
- [ ] Define `InterpretationFrame` as a single primary interpretation
      constraint for Phase 1.
- [ ] Confirm the generation chain is:
      `state -> change -> frame -> paragraph`.
- [ ] Confirm the writer receives bounded structured input rather than raw
      content lists.

Verification:

- the design clearly separates system judgment from language generation
- each object has one stable responsibility

## 4. Freeze Interpretation Rules

- [ ] Keep the bounded Phase-1 frame set:
      `capability_to_utility`,
      `open_to_closed_balance_shift`,
      `experimentation_to_adoption`,
      `acceleration_to_constraint`,
      `fragmentation_to_structure`.
- [ ] Restrict Phase 1 to exactly one `primary` frame per snapshot.
- [ ] Downgrade `secondary` frame support to a future refinement rather than a
      first-release requirement.
- [ ] Define frame scoring and "pick the highest supported frame" behavior.
- [ ] Define suppression behavior when no frame clears the confidence floor.

Verification:

- the first implementation does not need frame-combination logic
- implication text stays constrained and explainable

## 5. Freeze Window, Cadence, and Confidence Rules

- [ ] Confirm the internal short comparison window is the primary signal source.
- [ ] Confirm the longer window is background calibration only.
- [ ] Confirm trend snapshots refresh on a daily schedule rather than per page
      request.
- [ ] Define minimum sample thresholds for domain-level rendering.
- [ ] Define minimum support thresholds for surfaced signals.
- [ ] Define downgrade or omission behavior for sparse or conflicting evidence.

Verification:

- the page can show stable day-level judgment without visible jitter
- weak evidence cases degrade gracefully instead of forcing a claim

## 6. Freeze Persistence and Read Path

- [ ] Require `TrendInsightSnapshot` in Phase 1.
- [ ] Keep `TrendObservation` persistence optional for a later phase.
- [ ] Persist `background_json` inside the same snapshot record.
- [ ] Persist enough snapshot structure to distinguish current state, detected
      change, and final interpretation.
- [ ] Define latest-snapshot selection rules for page reads.

Verification:

- the page can render from one stable snapshot without triggering live
  synthesis
- Phase 1 storage scope stays small enough to implement quickly

## 7. Freeze API and Frontend Contract

- [ ] Extend `GET /daily-brief/today` with `trendInsight?: TrendInsight | null`.
- [ ] Keep `GET /daily-brief/issues?date=...` trend support out of Phase 1.
- [ ] Keep the frontend read model compact and centered on one paragraph, up to
      two signals, optional background, and optional evidence.
- [ ] Treat methodology display as optional and non-blocking.
- [ ] Keep evidence display simple: each surfaced signal should link to a small
      set of representative items.
- [ ] Prefer `brief item` evidence first and candidate-level evidence only as
      fallback.

Verification:

- the contract supports the intended UI without exposing internal analysis
  machinery
- absent `trendInsight` remains fully backward-compatible

## 8. Define Phase-1 Validation Targets

- [ ] Define required unit tests for base-axis extraction, AI extension
      extraction, recent-state summarization, change detection, frame scoring,
      and low-confidence suppression.
- [ ] Define required snapshot/service tests for stable synthesis inputs and
      latest-snapshot reads.
- [ ] Define required HTTP/frontend tests for module present, module absent,
      and evidence expand behavior.
- [ ] Define tuning inputs for rollout:
      thresholds, wording quality, evidence quality, and confidence floors.
- [x] Execute `openspec validate add-daily-brief-trend-insights --strict`.

Completion criteria:

- the OpenSpec package describes one realistic Phase-1 implementation slice
- the first release can ship as an `AI`-first experience without requiring
  multi-topic generalization work up front
