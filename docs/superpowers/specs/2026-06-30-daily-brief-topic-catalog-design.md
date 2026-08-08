# Daily Brief Topic Catalog Safe Rollout Design

## Background

The current Daily Brief implementation uses both `topics` and `sources` as first-class subscription inputs:

- `topics` are persisted in subscriptions and used to filter candidates.
- `sources` are persisted in subscriptions and used directly by collection, page-state resolution, scheduling, and generation.

The original topic-tree proposal is directionally correct, but it is not yet safe to land as written because it removes user-facing `sources` without fully redefining the runtime contract that currently depends on them.

This document defines a safe rollout design that preserves runtime stability while moving the product model to topic-first subscriptions.

## Goals

- Let users subscribe by curated topic only.
- Support hierarchical topic expansion over time, including future L3 topics.
- Preserve the current execution model where generation runs against a concrete set of source keys.
- Make topic/source evolution explicit and predictable.
- Avoid hidden behavior changes during ordinary read or generation requests.

## Non-Goals

- User-defined topics.
- User-defined source URLs or arbitrary RSS input.
- Removing `sources` from the runtime model in the first rollout.
- Automatic live migration of all existing subscriptions on every read.

## Product Decisions

### Subscription Intent

- `subscription.topics[]` is the user's real subscription intent.
- Only selectable leaf topic keys may be stored in `subscription.topics[]`.
- Users do not directly edit `subscription.sources[]`.

### Source Ownership

- Source keys remain platform-owned internal identifiers.
- Feed URLs, formats, and parser choices remain code-owned in the source catalog.
- The platform remains the single authority for topic-to-source bindings.

### Topic Expansion

- The platform may add new official topics or deeper topic layers later.
- Users may expand their subscription by selecting newly exposed official leaf topics.
- Users may not create custom topics or custom sources.

## Data Model

### Topic Catalog

The topic catalog is modeled as a general tree, not a fixed two-level enum.

Recommended node shape:

```go
type TopicNode struct {
    Key         string
    ParentKey   string
    DisplayName string
    Description string
    SortOrder   int
    Selectable  bool
    Enabled     bool
    Children    []TopicNode
}
```

Rules:

- `Selectable == true` means the node is a leaf topic that may be saved to subscriptions.
- Non-leaf nodes are navigation-only and must not be saved into subscriptions.
- `Enabled == false` means the node is visible but not currently subscribable.
- The structure must support future L3+ nodes without changing API shape.

### Subscription

`Subscription` keeps both fields, but with different semantics:

- `topics[]`: user-owned truth
- `sources[]`: system-owned execution snapshot

The persisted `sources[]` field is not user intent. It is derived state written by the backend when a subscription is saved or explicitly recomputed.

### Source Binding

`source_feed_catalog` remains the source of truth for feed definitions and topic bindings.

The catalog must support:

- source key validation
- leaf-topic-to-source expansion
- lookup of whether a leaf currently has any bound sources

## Runtime Contract

### Core Principle

`topics` are subscription truth, and `sources` are the persisted execution snapshot derived from the current topic/source binding graph.

### Save-Time Behavior

When a user saves a subscription:

1. Validate the submitted topic keys.
2. Reject any topic that is unknown, non-leaf, non-selectable, or disabled.
3. Expand topic keys to source keys using the current catalog.
4. Deduplicate and sort the resulting source keys.
5. Persist both `topics[]` and derived `sources[]`.

This is the primary snapshot refresh path.

### Read-Time Behavior

Ordinary subscription reads do not recompute `sources[]`.

`GET /daily-brief/subscription` returns the stored subscription state. This keeps reads stable and explainable. A user who did not save anything should not see their effective subscription drift because the catalog changed in the background.

### Generation-Time Behavior

Generation continues to use persisted `sources[]` as its collection input.

This preserves current operational behavior:

- collection remains reproducible
- scheduling remains stable
- page-state logic remains stable
- debugging can answer "which sources were actually used for this subscription"

### Explicit Refresh Behavior

The system must support an explicit snapshot recompute path for cases such as:

- platform adds a new source under an existing topic
- platform restructures topic leaves
- operations needs to bulk refresh subscriptions

This recompute path is an explicit action, not an implicit side effect of reads or generation requests.

## API Design

### GET `/daily-brief/topic-catalog`

Returns the recursive topic tree.

Each node should include:

- `key`
- `displayName`
- `description`
- `selectable`
- `enabled`
- `children`

Leaf nodes should also include:

- `hasSources`

UI behavior is driven from this API rather than hard-coded topic lists.

### GET `/daily-brief/subscription`

Returns the user-editable subscription model centered on:

- `enabled`
- `timezone`
- `deliveryTimeLocal`
- `topics`

`sources` should no longer be part of the editable subscription form contract.

Optional future enhancement:

- add read-only `resolvedSources[]` for debugging or operator visibility

This is not required for the first rollout.

### PUT `/daily-brief/subscription`

Accepts:

- `enabled`
- `timezone`
- `deliveryTimeLocal`
- `topics`

It does not accept user-provided `sources`.

The backend derives `sources[]` before persistence.

## Frontend Design

### Subscription Form

The `/brief` subscription experience becomes topic-first:

- remove source checkboxes from the subscription form
- load topic catalog from the backend
- render category navigation from the catalog tree
- allow selection only on enabled leaf nodes

### Disabled Topics

If a leaf exists but has no bound sources yet:

- it may still appear in the catalog
- it should not be selectable
- the UI should explain that it is not yet available

### Expansion Model

The first UI may render a two-level experience, but frontend state and rendering logic should tolerate deeper trees so future L3 rollout does not require a contract rewrite.

## Refresh Strategy

The system changes a subscription snapshot only in these situations:

1. user saves the subscription
2. an explicit backend recompute action runs

The system does not change a snapshot during:

- ordinary subscription reads
- page-state reads
- generation requests

This rule is required for predictability and operational safety.

## Topic Granularity Decision

The original proposal merged:

- `ai-models`
- `ai-research`
- `ai-tools`

into a single `tech.ai` leaf.

That merge is product-simplifying but operationally risky because the current system deliberately separates model blogs, research feeds, and tools/papers sources. A single merged leaf may reduce section quality and increase content mixing inside one generated section.

Safe recommendation:

- keep the catalog tree generalized
- allow `tech.ai` as a navigation node later if desired
- preserve finer leaf-level execution granularity where it improves generation quality

Possible safer initial mapping:

- `tech.ai.models`
- `tech.ai.research`
- `tech.ai.tools`
- `tech.dev`
- `tech.startups`

This keeps the architecture future-proof without forcing an early semantic merge that may hurt brief quality.

## Validation Rules

The implementation should separate three concepts that were previously blurred together:

- known topic key
- leaf topic key
- selectable/enabled topic key

Recommended validation helpers:

- `IsTopicKeyKnown`
- `IsLeafTopicKey`
- `IsTopicKeySelectable`

This avoids using one "supported" check for incompatible concerns such as:

- source catalog validation
- subscription save validation
- generated artifact validation

## Impact on Existing Runtime

This design intentionally minimizes first-rollout runtime churn:

- collection still uses `sources[]`
- topic filtering still uses `topics[]`
- scheduling does not need to become topic-expansion-aware
- page-state logic can continue using the persisted execution snapshot

The user experience changes first; the runtime contract changes only where needed to make that experience safe.

## Required Revisions to the Original Topic-Tree Proposal

The original proposal should be revised in these ways:

### 1. Reword the subscription model

Replace "only topics matter" wording with:

> Users subscribe by topic. The backend persists the selected leaf topics and a system-derived source snapshot used for execution.

### 2. Clarify API ownership

Update the API section so:

- `GET /daily-brief/topic-catalog` becomes the catalog source for UI
- `PUT /daily-brief/subscription` accepts only topics and schedule fields
- `sources` are backend-derived, not user-provided

### 3. Add explicit snapshot rules

Add a dedicated subsection that states:

- save recomputes snapshot
- read does not recompute snapshot
- generation uses stored snapshot
- explicit recompute is a separate operator path

### 4. Narrow the initial migration claim

Do not claim that changing only `topic_catalog.go`, `source_feed_catalog.go`, and `constants_test.go` is sufficient.

The proposal must acknowledge impact on:

- subscription DTOs and validation
- topic catalog domain helpers
- source binding helpers
- frontend topic rendering
- removal of source selection UI
- prompt hints
- tests across domain, service, HTTP, and frontend

### 5. Revisit initial AI topic merging

Mark the `tech.ai` merge as a product option requiring validation, not an unquestioned first-step migration.

## Rollout Recommendation

### Phase 1

- Introduce hierarchical topic catalog
- Keep runtime `sources[]`
- Move UI to topic-only subscription
- Derive and persist `sources[]` on save

### Phase 2

- Add more topic branches and additional official sources
- Add explicit snapshot recompute tooling

### Phase 3

- Reevaluate whether some parent nodes should become navigation-only with deeper leaf execution keys

## Success Criteria

The design is considered safe to implement when all of the following are true:

- users only choose topics
- only enabled leaf topics can be saved
- backend always persists a deterministic source snapshot
- generation uses that snapshot directly
- existing subscriptions do not silently drift on reads
- topic catalog can grow deeper without rewriting the API contract

## Summary

The safe rollout path is not to delete `sources`, but to demote them from user-owned configuration to backend-owned execution snapshot.

That gives the product a cleaner topic-first model while preserving the current collection and generation stability of Daily Brief.
