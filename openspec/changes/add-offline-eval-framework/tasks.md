# add-offline-eval-framework Tasks

## Phase 1 Scope

Phase 1 delivers only:

- shared offline evaluation runner and result contract
- `summary` evaluator
- `rewrite` evaluator
- initial manual sample sets

Phase 1 does not deliver:

- `tool` evaluator
- final answer evaluator
- online evaluation
- dashboard or annotation platform work

## Tasks

### 1. Build the shared offline evaluation skeleton

- [x] Define the shared suite runner contract for `summary`, `rewrite`, and
      `all`.
- [x] Define the shared sample-loading and suite-dispatch flow.
- [x] Define the shared result envelope:
      `suite / run_metadata / samples / aggregate / artifacts`.
- [x] Define the shared judge-layer contract, including structured judge output
      parsing and fixed evaluator judge config.

### 2. Implement the summary evaluator core

- [x] Add the summary sample schema with
      `input / expected_summary / critical_contract / next_turn_eval / metadata`.
- [x] Reuse the real summary generation path through an evaluator adapter rather
      than building a separate summary implementation.
- [x] Implement deterministic summary rule checks for schema validity, required
      fields, forbidden claims, critical entities, and state override handling.
- [x] Implement summary gate verdict plus diagnostic score with the agreed
      weighting model.

### 3. Implement summary semantic judging and downstream equivalence

- [x] Add one combined field-level judge flow for `goal`, `constraints`,
      `established_facts`, `recent_progress`, and `open_questions`.
- [x] Add downstream answer equivalence checks using:
      - full conversation context
      - summary-derived context only
- [x] Freeze evaluator answering config for this equivalence step and disable
      retrieval, tool usage, and other compensation paths.
- [x] Ensure critical failures still allow judge/equivalence execution when
      possible for diagnostic purposes, while preserving final failure status.

### 4. Implement the rewrite evaluator core

- [x] Add the rewrite sample schema with
      `input / rewrite_expectation / retrieval_expectation / metadata`.
- [x] Reuse the real rewrite execution path, including real rewritten execution
      shape when sub-questions are produced.
- [x] Implement rewrite rule checks for critical terms, alias/normalization
      behavior, `need_retrieval`, split boundaries, and forbidden rewrites.
- [x] Implement rewrite gate verdict plus diagnostic score with the agreed
      weighting model.

### 5. Implement retrieval before/after comparison for rewrite

- [x] Run baseline retrieval on the original query.
- [x] Run retrieval on the rewritten execution shape.
- [x] Report primary metrics:
      - `Hit@K`
      - `MRR`
- [x] Report auxiliary metrics:
      - `Recall@K`
      - `NDCG@K`
- [x] Surface uplift or regression per sample and in suite aggregate output,
      including critical regression handling for `must_not_regress` samples.

### 6. Add Phase-1 sample assets

- [ ] Author `20-30` manual, failure-mode-oriented summary samples.
- [x] Author `40-60` manual, failure-mode-oriented rewrite samples.
- [x] Add shared judge prompts and rubrics under `testdata/evals/shared/`.
- [x] Document suite-local sample conventions in the suite README files.

### 7. Add verification and regression entrypoints

- [x] Add focused tests for shared contracts, judge parsing, and suite sample
      parsing.
- [x] Add focused tests for summary rule checks and rewrite rule checks.
- [x] Add end-to-end suite validation for:
      - `summary`
      - `rewrite`
      - `all`
- [x] Ensure suite output supports tag-level regression review and preserves
      enough artifacts for debugging failed cases.

## Progress Notes

### 2026-06-19

- Re-ran the `summary` suite against real model routing instead of only relying on
  local package tests.
- Hardened judge execution in `PromptFileJudge`:
  - if the first judge response parses as empty, retry once without
    `response_format=json_object`
- Hardened provider diagnostics in the shared OpenAI-style chat client:
  - empty `message.content` is now treated as `INVALID_RESPONSE` so routing can
    fall back to the next candidate instead of silently accepting an empty
    completion
- Verified with focused tests that the new judge retry and empty-content
  detection work as intended.
- Confirmed the current blocker is external model availability rather than the
  offline eval framework contract itself:
  - `siliconflow / Qwen/Qwen3.6-35B-A3B` returns empty non-streaming content
  - `siliconflow / Qwen/Qwen3.5-27B` times out before headers
  - `bailian / qwen-plus` is blocked by `Arrearage`
  - local `ollama` fallback is not running
  - `siliconflow / glm-4.7` is disabled
- Checklist status did not change today: Phase 1 framework work remains largely
  implemented, while full `summary` suite execution is still blocked by runtime
  model availability and unfinished sample authoring under Task 6.

### 8. Add summary strategy evaluation mode

- [ ] Extend the `summary` evaluator with `standard` and `strategy` execution
      modes.
- [ ] Add runner flags for global threshold sweeps, including at least
      `-summary-mode` and `-summary-thresholds`.
- [ ] Extend the summary sample schema with an optional `strategy_eval` block
      containing checkpoint-based evaluation contracts.
- [ ] Implement repeated incremental summary compression simulation across full
      multi-turn dialogues.
- [ ] Add token accounting for baseline full-context cost versus
      summary-strategy cost.
- [ ] Reuse summary rule checks, field judge checks, and downstream equivalence
      checks at checkpoint and final-eval boundaries.
- [ ] Emit threshold-level sample artifacts and suite aggregates, including
      Pareto-candidate reporting.
- [ ] Author an initial strategy-focused summary sample set that is dense enough
      to expose repeated-compression drift.
- [ ] Add focused tests for turn replay, threshold sweeps, checkpoint
      evaluation, token accounting, and aggregate reporting.
