# add-offline-eval-framework Proposal

## Summary

This change establishes a unified offline evaluation framework for model-facing
stages in `goagent`, while keeping Phase 1 intentionally narrow.

Phase 1 delivers:

- one shared offline evaluation runner and result contract
- one `summary` evaluator for future structured conversation summaries
- one `rewrite` evaluator that scores both rewrite quality and retrieval uplift
- small, high-quality, manually designed sample sets
- `rules + LLM judge` scoring with deterministic checks as the first guardrail

The goal is not to build a full evaluation platform in one step. The goal is to
create a repeatable regression loop that can answer:

- did a change improve quality
- which sample classes improved or regressed
- whether a local optimization harmed later stages

## Why

The current repository already contains partial evaluation building blocks:

- retrieval evaluation tooling and sample plans
- rewrite evaluation tooling and checks
- structured summary design work

However, the current state is still fragmented:

- `rewrite` and `retrieve` evaluation are not yet part of one shared framework
- `summary` quality is discussed in design, but lacks a formal offline suite
- `tool` and final answer evaluation are future concerns, but current work
  should not block their later integration

Without a shared offline framework, future changes to prompts, validation rules,
fallback behavior, and policies are difficult to compare consistently.

## Goals

This change aims to:

1. establish one unified offline evaluation framework with extensible evaluator
   boundaries
2. deliver Phase-1 offline evaluation for `summary` and `rewrite`
3. evaluate `summary` directly against the future structured summary contract,
   not the compatibility text rendering
4. evaluate `rewrite` both as a standalone transformation and as a retrieval
   improvement mechanism
5. use manually designed, small-scale, high-quality samples focused on failure
   modes
6. make all Phase-1 outputs suitable for repeated regression runs and
   before/after comparisons

## Non-Goals

Phase 1 does not include:

- `tool` evaluator implementation
- final answer evaluator implementation
- online metrics, online experiments, or online A/B testing
- a full evaluation product surface such as dashboards, annotation backends, or
  automated sample mining
- standard-answer-based summary scoring
- full historical conversation replay as the primary sample source

## Recommended Scope

Phase 1 intentionally chooses option `2` from the design discussion:

- go deep on `summary + rewrite`
- do not yet implement `tool + answer`

This is narrower than a four-stage first release, but still preserves a shared
framework so later evaluators can be added without redoing sample organization,
runner interfaces, or result schemas.

## Key Decisions

### 1. Unified framework first, limited evaluators first

The system should build one common offline evaluation framework now, but only
register two evaluators in Phase 1:

- `summary`
- `rewrite`

### 2. Summary is evaluated as structured output

Phase 1 should evaluate the future structured summary object directly instead of
the current compatibility text summary.

Summary samples do not use a single standard-answer summary. They use:

- structured expected constraints
- forbidden hallucinations
- follow-up questions that test future usefulness

### 3. Summary uses dual quality dimensions

Structured summary quality is evaluated on two equally important dimensions:

- fidelity
- usefulness

Both dimensions must pass. A summary that is correct but not useful fails. A
summary that is useful but hallucinates also fails.

### 4. Rewrite is evaluated together with retrieval impact

Rewrite samples are not evaluated as text-only transformations. Each rewrite
sample also carries retrieval expectations so that Phase 1 can measure:

- rewrite correctness
- retrieval uplift after rewrite

### 5. Rules first, LLM judge second

Phase 1 adopts `rules + LLM judge`, but deterministic checks remain the first
gate:

- schema checks
- field checks
- entity retention checks
- forbidden-content checks
- retrieval metrics

LLM judging is reserved for semantic quality decisions that are hard to encode
reliably with rules alone.

### 6. Sample source is manual and failure-mode-driven

Phase 1 samples are primarily manually designed. They should be small in count
and high in quality:

- `summary`: `20-30` samples
- `rewrite`: `40-60` samples

The objective is not to mirror production traffic volume. The objective is to
cover representative failure modes with high control and repeatability.

## Acceptance Criteria

This change is complete only when all of the following are true:

1. There is one shared offline evaluation runner contract for multiple suites.
2. Phase 1 can run `summary` evaluation independently.
3. Phase 1 can run `rewrite` evaluation independently.
4. Phase 1 can run an aggregated suite command across all registered
   evaluators.
5. `summary` evaluation consumes manually designed structured-summary samples.
6. `summary` evaluation reports:
   - rule-check outcomes
   - field-level judge outcomes
   - overall fidelity/usefulness pass-fail
7. `rewrite` evaluation consumes rewrite samples that embed retrieval
   expectations.
8. `rewrite` evaluation reports:
   - rewrite quality checks
   - retrieval metrics before rewrite
   - retrieval metrics after rewrite
   - uplift or regression summary
9. Results support aggregate reporting by tag.
10. The framework leaves a clear extension point for future `tool` and `answer`
    evaluators without redesigning the shared runner or result contract.
