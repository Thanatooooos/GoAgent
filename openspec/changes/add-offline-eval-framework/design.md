# add-offline-eval-framework Design

## Overview

This design defines the Phase-1 implementation approach for
`add-offline-eval-framework`.

Phase 1 implements a unified offline evaluation framework, but only registers
two evaluators:

- `summary`
- `rewrite`

The framework is shared from day one, but evaluator rollout is intentionally
narrow. This keeps the first delivery manageable while preserving a clean
extension path for future `tool` and final answer evaluation.

## Goals

This design must:

1. define clear boundaries between the shared runner, evaluator logic, judge
   layer, and sample/report contracts
2. evaluate structured summary quality directly against the future structured
   summary contract
3. evaluate summary quality on both intrinsic quality and downstream answer
   stability
4. evaluate rewrite quality together with retrieval impact
5. provide a stable, repeatable offline regression loop
6. keep sample authoring small-scale, manual, and failure-mode-oriented in
   Phase 1

## Non-Goals

This design does not implement:

- `tool` evaluator execution
- final answer evaluator execution
- online evaluation or online experiments
- a dashboard, annotation backend, or sample management platform
- large-scale real-history replay as the primary Phase-1 sample source

## High-Level Architecture

Phase 1 uses four layers:

1. `suite runner`
2. `evaluator`
3. `judge layer`
4. `sample and report contract`

### 1. Suite Runner

The suite runner is responsible only for:

- selecting suites
- loading samples
- dispatching to evaluators
- aggregating results
- emitting final outputs

It must not contain suite-specific business logic.

### 2. Evaluator

Each evaluator owns suite-specific behavior.

Phase 1 evaluators are:

- `summary evaluator`
- `rewrite evaluator`

Each evaluator is responsible for:

- interpreting suite-specific samples
- executing the relevant model-facing stage
- applying suite-specific rule checks
- invoking the judge layer where needed
- producing suite-specific metrics inside the shared output envelope

### 3. Judge Layer

The judge layer is shared infrastructure for semantic judging. It is responsible
for:

- loading prompt templates
- loading rubrics
- invoking the judge model with fixed evaluator config
- parsing judge output
- handling judge parse errors or malformed outputs

It does not decide suite-specific business rules or final pass policy.

### 4. Sample and Report Contract

The sample and report contract defines:

- how samples are organized
- how suite outputs are shaped
- how future evaluators can extend the same framework

The outer shape is shared across suites. Internals may contain suite-specific
fields.

## Directory Layout

Phase 1 should organize evaluation assets under:

```text
testdata/evals/
  summary/
    samples.json
    README.md
  rewrite/
    samples.json
    README.md
  shared/
    judge_prompts/
    rubrics/
```

This structure keeps suite-local assets close to the evaluator while preserving
shared judge assets in one place.

## Shared Runner

The shared runner should support:

- `summary`
- `rewrite`
- `all`

Target command shape:

```powershell
go run ./cmd/eval-runner -suite summary -input testdata/evals/summary/samples.json
go run ./cmd/eval-runner -suite rewrite -input testdata/evals/rewrite/samples.json
go run ./cmd/eval-runner -suite all
```

The runner owns:

- suite selection
- input loading
- suite execution
- result assembly

The runner must not interpret summary or rewrite semantics directly.

## Shared Result Contract

All suites should emit the same top-level result shape:

- `suite`
- `run_metadata`
- `samples`
- `aggregate`
- `artifacts`

### Run Metadata

`run_metadata` should include enough context to make regression results
comparable, such as:

- run time
- evaluator version identifier
- model/config summary
- sample set identifier

### Sample Results

Every sample result should include at least:

- `name`
- `tags`
- `passed`
- `critical_failures`
- `rule_checks`
- `judge_checks`
- `scores`
- `failure_reasons`

Suite-specific fields may be added under `scores`, `judge_checks`, or
`artifacts`.

### Aggregate Results

Every suite should emit:

- `pass_rate`
- `critical_failure_rate`
- dimension-level averages
- tag-level aggregates

Rewrite aggregate output should additionally include before/after retrieval
metrics and uplift summaries.

### Artifacts

Artifacts should preserve enough information for debugging and review, such as:

- raw structured outputs
- baseline and candidate answers
- retrieval before/after details
- judge explanations
- execution metadata

The top-level schema is shared. Suite internals are allowed to differ.

## Shared Judge Layer

The judge layer exists so evaluators do not each invent their own prompt,
parsing, and retry behavior.

It should expose a stable interface that accepts:

- prompt template reference
- rubric reference
- structured input payload
- fixed evaluator model/config

It should return a structured result shape such as:

- `passed`
- `score`
- `missed_items`
- `incorrect_claims`
- `reason`

### Judge Scoring Scale

Phase 1 judge scoring should use a small discrete scale:

- `1`
- `0.5`
- `0`

This is more stable than asking the judge for broad subjective numeric scores.

### Judge Configuration

Judge calls must use fixed evaluator config rather than drifting with the main
chat runtime.

Judge prompts and rubrics should be versioned assets under the shared eval
directories.

## Summary Evaluator

### Scope

The summary evaluator scores the future structured summary output directly. It
does not score the compatibility text rendering as the primary target.

### Execution Boundary

The summary evaluator should reuse the real summary generation logic. It should
not create a separate summary implementation just for evaluation.

The evaluator should wrap the real path with adapters and scoring logic.

### Sample Model

Each summary sample should contain five logical sections:

1. `input`
2. `expected_summary`
3. `critical_contract`
4. `next_turn_eval`
5. `metadata`

#### Input

`input` includes:

- `source_messages`
- optional `previous_summary`

#### Expected Summary

`expected_summary` is not a single gold prose answer. It is a structured set of
coverage requirements per field.

Each present field may contain:

- `must_cover`
- `should_cover`
- `must_not_claim`

Fields may be omitted when not relevant. Empty sections do not need to be
materialized.

Expected summary fields align to the structured summary shape:

- `goal`
- `user_preferences`
- `constraints`
- `established_facts`
- `recent_progress`
- `open_questions`

#### Critical Contract

`critical_contract` defines hard-failure conditions explicitly. It should remain
fully manual in Phase 1.

It may contain:

- `critical_entities`
- `critical_constraints`
- `critical_facts`
- `critical_progress`
- `critical_open_questions`
- `critical_queries`
- `forbidden_claims`

Phase 1 should keep these as explicit arrays, not inferred automatically.

#### Next-Turn Evaluation

`next_turn_eval` defines downstream equivalence checks. It should include one to
three follow-up queries per sample.

Each query may contain:

- `id`
- `query`
- `equivalence_expectations`

#### Metadata

`metadata` is non-scoring information such as:

- authoring notes
- sample provenance
- comments for maintainers

### Quality Model

Summary quality is evaluated through three mandatory layers:

1. `structured fidelity`
2. `structured usefulness`
3. `downstream answer equivalence`

All three are required. None are optional enhancements.

### Structured Fidelity

This answers whether the structured summary is faithful to the original
conversation state.

It should check whether the summary:

- preserves critical goals, constraints, facts, IDs, error strings, or config
  keys
- avoids hallucinated established facts
- removes superseded state when newer turns override older decisions
- avoids forbidden claims

### Structured Usefulness

This answers whether the structured summary is useful as future work memory.

It should check whether the summary preserves:

- the current primary objective
- actionable constraints
- important recent progress
- unresolved questions that affect the next useful response

### Downstream Answer Equivalence

This answers whether summary compression causes downstream answer drift.

For each follow-up query, the evaluator should:

1. generate `full-context answer` from the original source dialogue
2. generate `summary-context answer` from the summary-derived context only
3. compare the two answers for semantic divergence

The comparison should focus on:

- key facts
- key constraints
- current goal understanding
- next-step recommendations
- omission of important information that only full context preserved

This layer measures semantic stability, not surface text similarity.

### Answering Config for Equivalence Checks

Downstream answer equivalence should:

- reuse the existing answer prompt body
- freeze evaluator model/config
- disable retrieval
- disable tool usage
- disable external compensation paths

This isolates the compression node and avoids masking summary defects through
other retrieval or tool behavior.

### Summary Rule Checks

The summary rule layer should remain deliberately narrow and deterministic.

It should cover:

- schema validity
- required field presence
- `must_not_claim` and `forbidden_claims`
- critical entity retention
- state override correctness
- minimum signal quality

It should not attempt to score `should_cover` quality or semantic usefulness.

### Summary Judge Checks

Summary judge evaluation should happen in two parts:

1. one combined field-level judge call
2. one equivalence judge flow over follow-up queries

#### Field-Level Judge

One judge invocation should evaluate the main structured fields together while
returning per-field results for:

- `goal`
- `constraints`
- `established_facts`
- `recent_progress`
- `open_questions`

It should report per-field:

- fidelity
- usefulness
- missed items
- incorrect claims
- reason

#### Equivalence Judge

The equivalence judge compares:

- the follow-up query
- the full-context answer
- the summary-context answer
- the sample's equivalence expectations

It should report whether the summary-based answer drifts in a way that matters.

### Summary Scoring

The summary evaluator should produce both:

- `gate verdict`
- `diagnostic score`

#### Gate Verdict

A sample fails immediately when it triggers a critical failure.

Critical failures include:

- forbidden claims
- hallucinated established facts
- loss of critical constraints
- loss of critical entities
- stale superseded state retained as current truth
- critical goal distortion
- loss of critical unresolved questions
- dangerous downstream answer divergence on critical queries

#### Diagnostic Score

When not blocked by a hard gate, the evaluator should compute a weighted sample
score.

Initial weight recommendation:

- `structured_fidelity`: `45%`
- `structured_usefulness`: `25%`
- `downstream_equivalence`: `30%`

The diagnostic score is for comparison and debugging. It must not override a
critical failure.

### Summary Execution Flow

The summary evaluator should run in this order:

1. load sample
2. build compression input
3. run real summary generation
4. run rule checks
5. run field-level judge
6. run downstream equivalence checks
7. build sample result

If rule checks already trigger a critical failure, judge and equivalence steps
should still run when possible for diagnostic value. The final verdict remains
failed.

## Rewrite Evaluator

### Scope

The rewrite evaluator scores:

- rewrite quality
- retrieval impact after rewrite

It does not evaluate final answer quality in Phase 1.

### Execution Boundary

The rewrite evaluator should use the real rewrite execution path and compare
retrieval behavior using the same effective input shape the system would really
use.

If the system rewrites into sub-questions, evaluation should follow that real
execution pattern rather than collapsing everything into a single synthetic
query comparison.

### Sample Model

Each rewrite sample should contain four logical sections:

1. `input`
2. `rewrite_expectation`
3. `retrieval_expectation`
4. `metadata`

#### Input

`input` includes:

- optional `history`
- `query`

#### Rewrite Expectation

`rewrite_expectation` may include:

- `must_keep_terms`
- `must_keep_any_groups`
- `must_contain_any`
- `must_not_start_with`
- `need_retrieval`
- `sub_question_count`
- `critical_terms`
- `forbidden_rewrites`

#### Retrieval Expectation

`retrieval_expectation` may include:

- `target`
- `expected_ids`
- `top_k`
- `search_mode`
- `critical_expected_ids`
- `must_not_regress`

`must_not_regress` should be an explicit per-sample flag in Phase 1 rather than
an implicit default.

#### Metadata

`metadata` contains non-scoring notes and maintenance context.

### Rewrite Execution Flow

The rewrite evaluator should run in this order:

1. load sample
2. run baseline retrieval on the original query
3. run real rewrite generation
4. run rewrite rule checks
5. run retrieval using the rewritten execution shape
6. build sample result

### Rewrite Rule Checks

Deterministic rewrite checks should cover:

- critical term retention
- alias preservation or valid normalization
- `need_retrieval`
- sub-question count boundaries
- forbidden rewrites
- reuse of constraint-guard metadata where available

### Retrieval Comparison

The rewrite evaluator should compare before/after retrieval metrics.

Primary metrics for Phase 1:

- `Hit@K`
- `MRR`

Auxiliary metrics:

- `Recall@K`
- `NDCG@K`

The evaluator should keep both the baseline and rewritten retrieval metrics and
report uplift or regression explicitly.

### Rewrite Scoring

Like summary, rewrite should emit both:

- `gate verdict`
- `diagnostic score`

#### Rewrite Critical Failures

Phase 1 critical failures include:

- loss of `critical_terms`
- hit on `forbidden_rewrites`
- hard mismatch on required `need_retrieval`
- clearly invalid split behavior on critical samples
- loss of `critical_expected_ids`
- regression on samples marked `must_not_regress`

#### Rewrite Diagnostic Score

Initial weight recommendation:

- `rewrite_quality`: `45%`
- `retrieval_impact`: `55%`

This weighting reflects that rewrite is not a text-only task in this framework.
It is judged partly by its operational retrieval outcome.

## Sample Strategy

Phase 1 should use manually designed, failure-mode-oriented samples.

Target sample counts:

- `summary`: `20-30`
- `rewrite`: `40-60`

### Summary Failure Modes

Priority summary failure modes include:

- updated constraints overriding old ones
- long dialogs with important IDs or error strings
- task switching while preserving the active goal
- uncertainty that must remain unresolved
- recent progress plus open questions

### Rewrite Failure Modes

Priority rewrite failure modes include:

- alias normalization
- abbreviations
- cross-turn coreference
- multi-condition constraints
- split versus non-split boundaries
- retrieval-sensitive rewrites

## Implementation Order

Phase 1 should proceed in this order:

1. freeze shared runner, result, and judge contracts
2. implement `summary evaluator`
3. implement `rewrite evaluator`
4. wire `all` suite execution and shared reporting
5. add initial sample sets

This order lets the more demanding summary path shape the framework before
rewrite adds retrieval comparison behavior.

## Risks and Controls

### Risk: Summary judging becomes too subjective

Control:

- rules first
- structured judge outputs
- field-level evaluation before global impressions

### Risk: Summary equivalence is polluted by unrelated runtime changes

Control:

- fixed evaluator answer config
- no retrieval
- no tool usage
- no external compensation

### Risk: Rewrite text quality improves while retrieval gets worse

Control:

- bind rewrite evaluation to retrieval impact
- report before/after metrics explicitly

### Risk: Samples become overly exam-like

Control:

- author samples around concrete failure modes
- keep the first phase small and curated

### Risk: Average scores hide severe failures

Control:

- hard gate first
- diagnostic score second
- no score can override critical failure

## Acceptance Criteria

This design is satisfied only when:

1. the shared runner boundary is explicit
2. the shared judge layer boundary is explicit
3. the shared result contract uses a common outer schema with suite-specific
   inner extensions
4. the summary evaluator uses the three-layer quality model:
   - structured fidelity
   - structured usefulness
   - downstream answer equivalence
5. the summary evaluator uses an explicit `critical_contract`
6. the rewrite evaluator scores both rewrite quality and retrieval impact
7. rewrite retrieval comparison uses the system's real rewritten execution shape
8. Phase 1 scope remains limited to `summary + rewrite`
