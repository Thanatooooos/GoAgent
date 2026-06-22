# Summary Eval Repair Design

Date: `2026-06-21`

## Background

The `summary` offline evaluation suite now runs end to end and produces stable
artifacts under `testdata/evals/summary/`.

The latest full run on `2026-06-21` shows:

- `12` samples total
- `2` passed
- `10` failed
- `pass_rate = 16.7%`
- `critical_failure_rate = 41.7%`

The current failure shape is informative:

- downstream equivalence passed on `9/12` samples
- field-level summary judgment passed on only `2/12` samples
- the weakest fields are `recent_progress` and `open_questions`

This means the system often retains enough information to answer a follow-up
question roughly correctly, but still fails to produce a stable structured
work-memory summary.

The immediate goal of this repair phase is not to redesign the whole summary
system. The goal is to improve suite pass rate quickly without lowering the
quality bar or weakening critical regression detection.

## Repair Goal

This repair phase optimizes for:

- higher `summary` suite pass rate first
- no evaluator "score laundering"
- no removal of critical-failure protection

Primary success target:

- raise total passing samples from `2/12` to at least `6/12`

Secondary expectation:

- reduce dangerous downstream drift where possible
- but not by relaxing the drift gate

## Non-Goals

This repair phase does not:

- redesign the structured summary schema
- replace the current summary generation architecture
- remove `critical_contract` hard gates
- downgrade dangerous downstream drift from a critical signal
- optimize for perfect score across all current samples

## Current Failure Pattern

The latest run indicates four dominant failure classes.

### 1. Active-scope drift

The summary frequently loses the current active goal or current execution
boundary, especially when the source dialogue contains:

- planning plus sidebar discussion
- earlier options later superseded
- "not now" decisions
- stage boundaries such as "do not implement yet"

Typical failing samples:

- `state_override_phase_scope`
- `goal_drift_multi_topic_discussion`
- `goal_drift_sidebar_debug`
- `state_override_priority_flip`

### 2. Missing `recent_progress`

The summary often preserves high-level facts but loses the latest confirmed
change that matters most to the next action, such as:

- priority changes
- confirmed environment combinations
- narrowed scope
- newly locked constraints

### 3. Missing `open_questions`

The summary often drops unresolved but action-relevant items, especially:

- unverified hypotheses
- pending validation steps
- candidate approaches not yet approved
- unresolved workflow constraints

### 4. Fact / proposal / suspicion confusion

The summary sometimes writes an unconfirmed candidate or hypothesis too close
to an established fact, especially in architecture and diagnosis samples.

## Recommended Repair Approach

Use a bounded three-layer repair strategy:

1. strengthen summary generation prompt and field contract
2. add conservative post-processing for structure and field placement
3. make evaluator reporting easier to interpret without lowering standards

This is the narrowest approach likely to improve pass rate meaningfully in one
repair cycle.

## Layer 1: Prompt Repair

### Objective

The prompt should guide the model to classify conversation state correctly
before it writes fields.

The model should optimize for "stable future-useful work memory", not for
"short readable summary text".

### Required Prompt Rules

The prompt should explicitly require the model to decide, for each candidate
piece of information:

1. is it confirmed, unresolved, or superseded
2. which field owns it
3. would losing it harm the next decision or next useful reply

### Field Contract Adjustments

#### `goal`

The prompt should say:

- keep only the current active objective
- do not promote side discussions, earlier abandoned directions, or future
  possible work into the main goal

#### `constraints`

The prompt should say:

- include currently effective boundaries on later work
- include explicit "do not do X now" and "do not enter stage Y yet" items
- include temporary execution boundaries when they clearly govern the next step

#### `established_facts`

The prompt should say:

- include confirmed state only
- do not place proposals, guesses, suspicions, or pending validations here

#### `recent_progress`

The prompt should say:

- capture newly confirmed or newly changed state
- prioritize:
  - priority changes
  - scope changes
  - constraint tightening
  - selected environment combinations
  - newly confirmed delivery expectations

#### `open_questions`

The prompt should say:

- include unresolved items that affect the next action or next answer
- if something is still unverified, pending, or candidate-only, prefer
  `open_questions` over `established_facts`

### Prompt Heuristics

The prompt should add lightweight heuristics such as:

- if later turns override earlier turns, preserve only the latest valid state
- if the conversation says "not yet", preserve that as a constraint
- if the conversation says "still not confirmed", preserve that as an open
  question
- if the conversation says "already confirmed", preserve that as recent
  progress or established fact depending on whether it is new state or stable
  background

## Layer 2: Conservative Post-Processing

### Objective

Post-processing should improve structural stability without inventing new
meaning.

It should remain conservative:

- no semantic rewriting of the summary
- no new facts
- only field repair, demotion, duplication across compatible fields, and schema
  completion

### Allowed Repairs

#### 1. Schema completion

Guarantee that all required fields exist with stable types.

This directly targets:

- `schema validation failed`
- `required summary fields missing`

#### 2. Fact demotion

If an item in `established_facts` contains unresolved markers such as:

- `possible`
- `suspected`
- `candidate`
- `pending confirmation`
- `pending review`

then demote it out of `established_facts`.

Preferred destination:

- `open_questions` when the unresolved nature matters to later work

#### 3. Constraint promotion

If the generated output clearly expresses a currently active boundary such as:

- `do not enter implementation yet`
- `deliver the first summary-eval draft this week`
- `Phase 1 covers summary and rewrite only`

but places it only in `goal` or `recent_progress`, duplicate it into
`constraints`.

This is allowed because it does not invent new meaning; it only repairs field
placement for evaluator-aligned structured memory.

#### 4. Recent-progress backfill

If `recent_progress` is empty, but the generated summary already contains clear
"newly confirmed / newly changed" language, extract one or more such items into
`recent_progress`.

Good candidates include:

- changed priorities
- confirmed selections
- newly settled workflow decisions
- narrowed scope

#### 5. Open-question backfill

If `open_questions` is empty, but the generated output includes unresolved
content, extract it into `open_questions`.

Good candidates include:

- pending verification
- unconfirmed root cause
- candidate approaches awaiting review
- not-yet-decided scoring or threshold policy

### Explicit Limits

Post-processing must not:

- infer new open questions absent from generated content or source evidence
- convert weakly implied information into hard constraints
- rewrite ambiguous content into stronger claims
- hide dangerous drift by forcing a pass-like shape

## Layer 3: Evaluator Alignment

### Objective

Keep evaluator standards unchanged, but improve diagnosis clarity so repair work
targets the real failure source.

### What Stays Unchanged

- `critical_contract` remains a hard gate
- dangerous downstream drift remains a hard gate
- field-level fidelity and usefulness remain required

### What Should Improve

Diagnostic-only signals should remain visible, but they should not dominate the
repair narrative when a stronger failure source exists.

Specifically:

- preserve messages such as
  `required summary fields missing (diagnostic only; judge owns final verdict)`
- but separate them clearly from:
  - critical failures
  - field-level semantic failures
  - downstream dangerous drift

This is a reporting and prioritization improvement, not a standards change.

## Sample-Driven Repair Priorities

To reach `6/12` passes quickly, the first repair cycle should focus on the
samples most likely to improve through prompt classification plus conservative
field repair.

### First-Priority Samples

#### `state_override_phase_scope`

Needed repair:

- preserve "not entering implementation yet" as `constraints`
- preserve "spec/design/tasks first" as `recent_progress`
- preserve weak-model sample-authoring concern as `open_questions`

#### `critical_entity_component_version`

Needed repair:

- preserve confirmed environment combination as `recent_progress`
- preserve unresolved connectivity and baseline validation as
  `open_questions`

#### `goal_drift_multi_topic_discussion`

Needed repair:

- preserve only the active short-term goal
- keep planning-stage boundary in `constraints`
- keep unresolved scoring-contract detail in `open_questions`

#### `fact_vs_open_question_uncertain_root_cause`

Needed repair:

- do not convert suspicion into fact
- preserve "root cause still unconfirmed" as `open_questions`
- preserve "do not treat guesses as confirmed" as `constraints`
- preserve observed timeout/high-latency symptoms as `recent_progress`

#### `state_override_priority_flip`

Needed repair:

- preserve current delivery requirement as `constraints`
- preserve priority escalation as `recent_progress`

#### `long_dialogue_dense_constraint_stack`

Needed repair:

- preserve current effective constraints under long-dialog compression
- preserve narrowed and updated constraints in `recent_progress`

### Second-Priority Samples

These should be addressed after the first pass-rate lift:

- `critical_entity_error_code`
- `critical_entity_config_key`
- `fact_vs_open_question_proposed_architecture`
- `goal_drift_sidebar_debug`

These are more likely to need stronger entity handling or deeper prompt
precision.

## Implementation Boundaries

This design expects changes only in:

- summary generation prompt or prompt assembly
- summary parsing / repair / validation helpers
- summary evaluator reporting clarity where needed

This design does not require:

- schema migration
- database model changes
- changing the sample contract
- weakening evaluator gates

## Verification Plan

Verification should proceed in this order.

### 1. Focused tests

Add or update focused tests for:

- unresolved-item demotion from `established_facts` to `open_questions`
- constraint promotion from active boundary statements
- recent-progress backfill
- schema completion behavior

### 2. Sample-targeted suite checks

Re-run the `summary` suite and inspect at least these samples first:

- `state_override_phase_scope`
- `critical_entity_component_version`
- `goal_drift_multi_topic_discussion`
- `fact_vs_open_question_uncertain_root_cause`
- `state_override_priority_flip`
- `long_dialogue_dense_constraint_stack`

### 3. Full regression check

Run the full `summary` suite again and confirm:

- no increase in dangerous downstream drift count
- total passes reach at least `6/12`
- `recent_progress` and `open_questions` field scores improve materially

## Risks

### Risk: Post-processing becomes hidden rewriting

Mitigation:

- limit repair to demotion, promotion, duplication, and schema completion
- never invent new facts

### Risk: Prompt grows too complicated

Mitigation:

- keep prompt rules tied directly to current failure classes
- avoid adding generic style instructions

### Risk: Pass rate rises only through easier diagnostics

Mitigation:

- leave critical gates unchanged
- judge success using sample passes, not reduced warning volume

## Final Recommendation

Proceed with a bounded repair cycle focused on:

- prompt-side state classification
- conservative field repair
- clearer evaluator diagnosis

The first target is practical rather than perfect:

- improve `summary` suite passes from `2/12` to at least `6/12`
- do so without removing critical protections
- prioritize stable structured work memory over cosmetic summary quality
