# add-daily-brief-trend-insights Tasks

## Scope

This change delivers:

- one `Trend Insight` module for the top of the `Daily Brief` page
- one reusable general-axis analysis model
- one richer `AI` domain extension
- recent-change snapshot generation plus longer-window calibration
- one stable page read contract

This change does not deliver:

- a live real-time dashboard
- broad domain-template support for every topic
- user-authored analysis taxonomies
- implementation of every future domain switch or chart view

## 1. Freeze Product Contract

- [ ] Define the `Trend Insight` page placement and content hierarchy.
- [ ] Confirm that the normal brief remains visible below the trend module.
- [ ] Confirm that user-facing copy must avoid explicit `7d` / `30d` labels.
- [ ] Confirm that the first page load returns at most one primary domain
      insight.
- [ ] Confirm that the top module is centered on one analytical paragraph with
      supporting signals, not on a single short headline.

Verification:

- proposal, design, and spec delta agree on the same product shape
- no contract text describes the feature as user-preference drift analysis

## 2. Define Analysis Contracts

- [ ] Define the reusable general analysis axes.
- [ ] Define allowed `unknown` / partial-label behavior.
- [ ] Define the `AI` extension axes for Phase 1.
- [ ] Define signal kinds: `rising`, `falling`, `new`, `persistent`, `shift`.
- [ ] Define evidence-group and methodology contracts.
- [ ] Define the sibling `dailybrief/trend` package boundary and its public
      entrypoints.

Verification:

- all required fields have one stable name
- AI extensions are layered on top of the general axes rather than replacing
  them

## 3. Define Window and Snapshot Semantics

- [ ] Define the recent-comparison window as the primary internal signal source.
- [ ] Define the longer window as background calibration rather than primary UI
      emphasis.
- [ ] Define sample-size thresholds and suppression rules.
- [ ] Define stability expectations for one day's snapshot.

Verification:

- the design clearly separates internal windows from user-facing wording
- low-sample and conflicting-signal cases have explicit downgrade behavior

## 4. Define Persistence and Read Models

- [ ] Define `TrendObservation`.
- [ ] Define `TrendInsightSnapshot`.
- [ ] Define `background_json` as part of the same snapshot record.
- [ ] Define how evidence refs link back to visible content.
- [ ] Define the latest-snapshot read rule for the page.

Verification:

- the page can render from a stable snapshot without triggering live synthesis
- the normal brief page can still load when no trend snapshot exists

## 5. Define API and Frontend Contracts

- [ ] Extend the `GET /daily-brief/today` contract with `trendInsight`.
- [ ] Define frontend empty/degraded/absent behavior for the trend module.
- [ ] Define evidence expansion behavior.
- [ ] Define optional methodology display behavior.
- [ ] Define evidence priority as `brief item` first and candidate fallback
      second.

Verification:

- the contract supports rendering the module without exposing internal window
  labels
- the page remains backward-compatible when `trendInsight` is absent

## 6. Define Rollout and Validation Plan

- [ ] Define a Phase-1 rollout focused on `AI`.
- [ ] Define required unit, trend-package, HTTP, and frontend tests.
- [ ] Define snapshot tuning inputs: thresholds, wording, evidence quality.
- [x] Execute `openspec validate add-daily-brief-trend-insights --strict`.

Completion criteria:

- the OpenSpec package is internally consistent
- the design supports one stable first release without forcing implementation
  of every future domain
