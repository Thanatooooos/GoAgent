# Summary Strategy Token Threshold Design

## Goal

Upgrade summary strategy evaluation so candidate compression thresholds are
measured in tokens rather than conversation turns. The evaluation shall model
the production trigger rule closely enough to compare candidate history budgets
such as 800, 1200, and 1600 tokens while preserving the existing turn-based mode
for compatibility.

## Scope

This change includes:

- a new `eval-runner` token-threshold CLI option
- token-aware trigger simulation after each completed user/assistant turn
- accounting for the latest committed summary plus uncovered messages
- shared token estimator usage and per-message structural overhead
- explicit threshold units in sample artifacts and aggregate output
- compatibility with the existing turn-threshold option

This change does not include:

- applying a safety factor inside the offline threshold sweep
- changing production summary generation or persistence
- replacing `after_turn` checkpoints in evaluation samples
- collecting production trace percentiles
- choosing the final production history budget

## CLI Contract

Add:

```text
-summary-token-thresholds 800,1200,1600
```

Retain:

```text
-summary-thresholds 4,6,8
```

The existing option remains a legacy turn-threshold mode. Resolution rules are:

1. If `-summary-token-thresholds` is non-empty, use token mode.
2. If both token and turn thresholds are provided, token mode wins.
3. If only `-summary-thresholds` is provided, use legacy turn mode.
4. If strategy mode is requested without either option, return a clear
   validation error.
5. Standard summary evaluation ignores both threshold options.

Both threshold lists accept positive, comma-separated integers. Empty entries
are ignored; invalid or non-positive values are rejected.

## Runtime Contract

Replace the ambiguous runtime `Thresholds` field with an explicit strategy
configuration:

```go
type SummaryStrategyThresholdUnit string

const (
    SummaryStrategyThresholdTokens SummaryStrategyThresholdUnit = "tokens"
    SummaryStrategyThresholdTurns  SummaryStrategyThresholdUnit = "turns"
)

type SummaryEvaluatorRuntimeOptions struct {
    Mode                  SummaryEvalMode
    ThresholdUnit         SummaryStrategyThresholdUnit
    Thresholds            []int
    MessageOverheadTokens int
}
```

Token mode is the recommended path. Turn mode exists only to keep existing
scripts and tests working during migration.

The message overhead value should come from
`rag.memory.summary-token.message-overhead-tokens` when the runtime config is
available. The fallback is 4 tokens per message, matching the checked-in
development configuration.

## Token Trigger Semantics

Evaluation processes source messages in completed user/assistant turns.

After each completed turn, compute:

```text
effective_strategy_tokens =
    tokens(rendered committed summary)
  + sum(tokens(uncovered message content) + message overhead)
```

If there is no committed summary, the summary component is zero and all
messages through the current turn are uncovered.

Compression triggers when:

```text
effective_strategy_tokens >= token_threshold
```

On trigger:

1. Generate a summary from the previous committed structured summary and all
   messages after the committed coverage boundary through the current turn.
2. Commit that generated summary in the simulation.
3. Advance the simulated coverage boundary to the current turn.
4. Increment `summary_call_count`.

Only one compression is attempted for a given completed turn. The simulation
does not repeatedly compress the newly generated summary within the same turn.

No safety factor is applied. Candidate values therefore represent direct
estimated token budgets and remain easy to interpret. Safety-factor calibration
belongs to production rollout analysis.

## Checkpoint Semantics

Evaluation sample checkpoints remain turn-based:

```json
{
  "after_turn": 6
}
```

`after_turn` identifies a stable point at which quality and downstream behavior
are compared; it does not control compression timing.

At a checkpoint:

- the baseline context is all source messages through `after_turn`
- the strategy context is the latest committed rendered summary plus uncovered
  messages through `after_turn`
- quality rules continue to evaluate the generated summary snapshot expected by
  the current evaluation contract
- downstream equivalence compares baseline context against strategy context

Keeping checkpoints turn-based avoids changing existing sample files and
separates “when to inspect quality” from “when compression triggers.”

## Legacy Turn Mode

Legacy mode preserves the current behavior:

- a threshold value means turns between simulated summary commits
- existing strategy samples require no changes
- result output identifies the unit as `turns`

Token and turn execution should share checkpoint evaluation, scoring, artifact,
aggregation, and Pareto logic. Only trigger scheduling differs.

## Output Contract

Add `threshold_unit` to threshold-level results:

```json
{
  "threshold": 1200,
  "threshold_unit": "tokens",
  "summary_call_count": 2
}
```

Add the same field to aggregate rows:

```json
{
  "threshold": 1200,
  "threshold_unit": "tokens",
  "sample_count": 8
}
```

The shared sample `rule_checks` shall also include:

```json
{
  "strategy_mode": true,
  "threshold_unit": "tokens"
}
```

Pareto candidates remain integer threshold values because one strategy run has
exactly one threshold unit.

## Estimation Contract

Token mode uses the shared estimator contract already used by production
summary and chat budgeting.

Message accounting is:

```text
message_tokens = estimator(content) + message_overhead_tokens
```

Summary accounting uses the rendered committed summary text without an
additional message overhead because it is represented as one strategy state
artifact rather than a source chat message in the current evaluator.

The same estimator must be used for:

- trigger decisions
- baseline token totals
- strategy token totals
- token saved ratios

This avoids a sweep where triggering and reported savings use different token
semantics.

## Error Handling

- Missing thresholds in strategy mode: return a validation error before sample
  execution.
- Invalid threshold unit: return an explicit unsupported-unit error.
- Empty or non-positive normalized threshold list: return an error.
- Missing estimator: use the shared default estimator.
- Negative message overhead: normalize to zero.
- Summary generation failure: retain the existing threshold-and-turn context in
  the returned error.

## Testing

### CLI Tests

- parses `-summary-token-thresholds`
- token thresholds win when both options are supplied
- legacy turn thresholds still work
- strategy mode without thresholds fails clearly
- invalid token thresholds fail before runtime construction

### Trigger Tests

- below token threshold does not commit a summary
- exactly equal to the threshold commits a summary
- committed summary plus uncovered tail can trigger a later compression
- message overhead contributes to trigger timing
- only one compression occurs per completed turn
- turn mode retains existing trigger behavior

### Output Tests

- threshold result contains `threshold_unit`
- aggregate result contains `threshold_unit`
- shared sample rule checks contain `threshold_unit`
- token totals use the same overhead-aware estimator as trigger decisions

### Regression Tests

- existing standard summary evaluation remains unchanged
- existing strategy checkpoint, final-eval, equivalence, artifact, and Pareto
  tests continue to pass
- `cmd/eval-runner` and `internal/app/rag/evaluation` focused suites pass

## Success Criteria

The change is complete when:

- `eval-runner` can execute a strategy sweep using token thresholds
- compression timing is determined by committed summary plus uncovered message
  tokens after each completed turn
- token and legacy turn modes are unambiguous in output
- existing turn-based scripts remain usable
- focused CLI and evaluation tests pass
- the OpenSpec rollout task for token-budget strategy comparison can be checked
  only after a real candidate sweep has been executed
