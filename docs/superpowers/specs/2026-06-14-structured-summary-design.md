# Structured Work-Memory Summary Design

Date: `2026-06-14`

## Background

The current conversation summary compression path in `internal/app/rag/core/history`
still treats summary generation as a single free-form text step:

- read recent messages
- build one text prompt
- ask the LLM for one short Chinese summary
- persist that summary into `t_conversation_summary.content`
- inject `content` back into prompt preparation through `LoadLatestSummary()`

This works as a minimal closure, but it leaves three practical problems:

1. summary semantics are underspecified, so the model may optimize for "short"
   rather than "useful for later turns"
2. `summary-max-chars: 200` is too aggressive for longer technical and task-led
   conversations
3. quality control is weak; a poor summary can still flow into later prompt
   assembly as long as the text is non-empty

The goal of this design is to upgrade summary compression from "free-form short
text" to "structured work memory", while preserving the current chat read path
and keeping rollout risk low.

## Goals

- Make summary generation preserve future-useful state instead of only producing
  a generic short recap.
- Introduce structured summary data as the semantic source of truth.
- Keep rendered `content` for backward compatibility with the current read path.
- Replace one-size-fits-all length control with tiered summary budgets.
- Add lightweight service-side validation so obviously bad summaries do not
  immediately pollute later turns.

## Non-Goals

- Replace the existing summary read path in the same phase.
- Fully redesign token budget or pinned-history logic in this phase.
- Introduce segment-summary / master-summary layering in this phase.
- Add heavy semantic evaluation or a second LLM-based verifier in v1.

## Current Constraints

- `LoadLatestSummary()` currently reads `ConversationSummary.Content` and
  returns it as a system message.
- Existing prompt budget and pinning logic still expect a rendered text summary.
- Summary lifecycle fields already exist and should continue to be used:
  `summary_version`, `covered_from_message_id`, `covered_to_message_id`,
  `source_message_count`, `quality_status`, `last_rebuild_reason`.
- Rollout should minimize chat main-path risk and remain easy to rollback.

## Recommended Approach

Use a compatibility-first design:

- store a structured summary payload as the new semantic truth
- continue storing rendered text in `content`
- keep current summary loading behavior unchanged in phase 1
- add validation before a new summary becomes the accepted prompt-facing result

This is intentionally a phased migration:

1. compression output changes first
2. storage model expands second
3. validation and budget policy are added around the new output
4. read-path migration is deferred until the new structure proves stable

## Data Model

### Summary Record Shape

Each `ConversationSummary` record keeps both:

- `StructuredSummary`: machine-oriented semantic source
- `Content`: rendered compatibility text for current prompt injection

### Structured Summary Schema

The first schema version is intentionally small:

```json
{
  "schema_version": 1,
  "goal": "Current primary user objective",
  "user_preferences": [
    "Answer in Chinese by default"
  ],
  "constraints": [
    "Keep current read path compatible"
  ],
  "established_facts": [
    "LoadLatestSummary currently reads Content"
  ],
  "recent_progress": [
    "Chose structured summary as source of truth"
  ],
  "open_questions": [
    "Should budget tiering be driven by token count only"
  ]
}
```

### Field Semantics

- `goal`
  - one primary task or objective
  - should describe what the user is actually trying to achieve now
- `user_preferences`
  - stable preferences that matter to later responses
  - should not contain short-lived turn-local requests unless they are likely to
    matter beyond the current turn
- `constraints`
  - boundaries, restrictions, non-negotiables, compatibility requirements,
    safety constraints, or user-stated "do not" conditions
- `established_facts`
  - facts already supported by the conversation
  - must not include guesses, hypotheses, or inferred-but-unconfirmed details
- `recent_progress`
  - recent decisions, completed steps, or newly established state
- `open_questions`
  - unresolved items that will affect the next useful answer or next decision

### Storage Recommendation

Add one JSON-capable field to `t_conversation_summary` for the structured
payload rather than splitting fields into many columns in v1.

Recommended storage shape:

- existing lifecycle fields remain unchanged
- existing `content` remains unchanged
- add `structured_summary_json`

Why JSON first:

- lower migration and rollback risk
- easier schema evolution
- simpler rendering and validation
- avoids over-committing to column-level modeling before field stability is
  proven

## Compression Prompt Contract

### Prompt Objective

The prompt should no longer ask for "a concise summary paragraph".

Instead, it should ask for a structured work-memory JSON object whose purpose is
to preserve state that will matter in later turns.

### Prompt Rules

The prompt should explicitly tell the model:

- the output is for future turn continuity, not for human prose quality
- keep only state that will affect later answers or decisions
- omit greetings, politeness, repetition, and wording noise
- if later messages override earlier ones, keep only the latest valid state
- do not invent facts
- unconfirmed items belong in `open_questions`, not `established_facts`
- output JSON only
- only allowed fields may be emitted

### Per-Field Guidance

The prompt should define:

- `goal`
  - exactly one primary objective
- `user_preferences`
  - stable response or workflow preferences
- `constraints`
  - hard boundaries and compatibility conditions
- `established_facts`
  - confirmed facts only
- `recent_progress`
  - the most relevant recent decisions or completed steps
- `open_questions`
  - unresolved questions affecting the next step

### Incremental Update Behavior

If a previous summary exists, the prompt should ask for summary update behavior
rather than full free-form re-summarization:

- reuse still-valid prior state
- merge new conversation changes
- drop superseded items
- keep the result internally consistent

This reduces drift compared with repeated from-scratch summarization.

## Rendering Strategy

### Principle

The LLM produces the structured object.
The service renders compatibility text.

Rendering should be handled by a dedicated summary-rendering layer rather than
mixed into compression flow logic.

### Rendered Text Template

Suggested stable template:

```text
对话摘要：
目标：...
用户偏好：
- ...
约束：
- ...
已确认事实：
- ...
最近进展：
- ...
待确认问题：
- ...
```

Rendering rules:

- omit empty sections
- keep section order stable
- prefer higher-value information first

Recommended priority order:

1. `goal`
2. `constraints`
3. `user_preferences`
4. `established_facts`
5. `recent_progress`
6. `open_questions`

This keeps the text output deterministic and easier to budget.

## Budget Strategy

### Why Change

A fixed `summary-max-chars: 200` is too blunt for longer technical
conversations. The summary needs more room when the source dialogue is longer or
more information-dense.

### Tiered Budget Policy

Replace one fixed output budget with tiers.

Initial recommendation:

- `small`
  - rendered text budget: `400`
- `medium`
  - rendered text budget: `600`
- `large`
  - rendered text budget: `800`

### Tier Selection Inputs

Use simple deterministic inputs in v1:

- number of source messages in the compression window
- total source character count or rough token estimate
- presence of high-density signals such as:
  - IDs
  - error strings
  - config keys
  - explicit constraints
  - numeric versions or thresholds

Rule recommendation:

- message/size thresholds decide the baseline tier
- high-density signal detection can only upgrade a tier, not downgrade it

### Structured Output Limits

Do not leave all budget control to the LLM.

Recommended first limits:

- `goal`: exactly 1 item, max around 80 chars
- list fields: each item max around 60-80 chars
- list fields: max 3 items per section in v1

The renderer then applies final total-length trimming if still needed.

## Validation Strategy

Validation should be lightweight but real. The goal is to block obviously bad
summary updates before they replace the prompt-facing accepted result.

### Validation Categories

1. Structural validity
   - JSON parses successfully
   - only allowed fields exist
   - `goal` is non-empty
   - list field types are correct

2. Minimum content quality
   - summary must contain at least one of:
     - `constraints`
     - `established_facts`
     - `recent_progress`
   - reject overly generic outputs such as "the user discussed a problem"

3. Critical-entity preservation
   - extract high-value entities from source messages, including:
     - IDs
     - error text
     - config keys
     - versions
     - important numbers
   - ensure critical entities are not all dropped from
     `constraints`, `established_facts`, or `recent_progress`

4. No hallucinated facts
   - if the summary claims a strong fact absent from source messages, reject it
   - `established_facts` should be held to the strictest standard

### Validation Outcome Policy

If validation passes:

- persist the new record
- set `quality_status=accepted`
- render and store `content`

If validation fails:

- persist the new record for audit if desired, but mark it
  `quality_status=rejected`
- keep the last accepted summary as the prompt-facing source
- record the rejection reason in `last_rebuild_reason`
- emit trace/observability data for later debugging

This keeps bad summaries visible for diagnosis without immediately poisoning the
chat main path.

## Rollout Plan

### Phase A: Structured Write Path

Scope:

- expand domain/model/repository/migration for structured payload storage
- update compression flow to request structured JSON
- validate structured output
- render compatibility `content`
- continue reading `content` in current chat path

Expected result:

- semantic source of truth changes
- chat read path remains behaviorally stable

### Phase B: Budget and Validation Hardening

Scope:

- add summary policy layer for tier selection
- add renderer layer
- add validator layer
- use accepted/rejected quality outcomes consistently

Expected result:

- summary quality becomes service-controlled instead of prompt-luck-driven

### Phase C: Structured-First Read Path

Deferred until the new write path proves stable.

Possible later change:

- `LoadLatestSummary()` renders from structured payload first
- `content` becomes compatibility fallback only

This phase is intentionally out of scope for the initial rollout.

## Components to Change

Likely initial touchpoints:

- `internal/app/rag/core/history/summary_compression.go`
- `internal/app/rag/core/history/service_store.go`
- `internal/app/rag/core/history/summary_job.go`
- `internal/app/rag/domain/conversation_summary.go`
- `internal/adapter/repository/postgres/rag/models/conversation_summary_model.go`
- `internal/adapter/repository/postgres/rag/conversation_summary_repo.go`
- migration for `t_conversation_summary`

Recommended new helper files:

- `summary_schema.go`
- `summary_renderer.go`
- `summary_policy.go`
- `summary_validator.go`

## Error Handling

- invalid JSON output from the LLM should not overwrite the accepted prompt path
- validation failures should be observable and diagnosable
- rendering should never panic on missing optional sections
- async summary jobs should preserve the same acceptance/rejection semantics as
  sync execution

## Testing Strategy

Minimum test set:

1. structured summary output is parsed and stored correctly
2. renderer produces stable compatibility text from structured payload
3. tier policy selects the expected budget for representative inputs
4. validation rejects summaries that:
   - omit critical identifiers
   - convert guesses into facts
   - contain only generic filler
5. existing prompt-injection path continues to work with rendered `content`

## Key Risks

- over-structuring too early could make the prompt brittle
- validation that is too strict may reject useful summaries
- validation that is too weak provides little protection
- if rendered text diverges too far from structured truth, debugging becomes
  harder

The mitigation is to keep schema v1 small, deterministic, and strongly aligned
with current prompt needs.

## Final Recommendation

Proceed with the compatibility-first version:

- structured summary JSON becomes the new semantic truth
- rendered `content` remains the compatibility layer
- tiered budgets replace the fixed 200-char target
- lightweight service-side validation prevents obviously bad summaries from
  becoming prompt input

This delivers the main product gain now, while preserving a safe path toward a
later structured-first read model.
