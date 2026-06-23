# Summary Long-Dialogue Generation Design

## Goal

Generate realistic long conversations for summary strategy evaluation by
combining a controlled sequence of user questions with answers produced by the
configured external chat model.

The generated conversation must be long enough to exercise the 800, 1200, and
1600 token compression thresholds, while preserving deterministic evaluation
targets such as active goals, constraints, state overrides, and unresolved
questions.

## Scope

This change includes:

- one 24-turn Chinese software-project conversation scenario
- a checked-in question script controlled by the repository
- sequential external-model calls carrying the conversation so far
- immediate checkpointed persistence after every generated answer
- token measurements using the shared production estimator
- conversion of an approved raw conversation into a strategy evaluation sample

This change does not include:

- asking the external model to invent evaluation rules or gold annotations
- automatically accepting generated answers as ground truth
- production conversation capture
- tool or retrieval message generation
- selecting the final production compression threshold

## Scenario Design

The first scenario models a software project whose requirements and decisions
change over time. It contains 24 complete user/assistant turns:

1. Turns 1-4 establish the project goal, initial scope, and delivery criteria.
2. Turns 5-8 add database, compatibility, schedule, and implementation
   constraints.
3. Turns 9-12 investigate evidence and preserve explicitly unresolved
   questions.
4. Turns 13-16 replace an earlier decision and clearly invalidate the stale
   state.
5. Turns 17-20 shift the active goal while carrying forward selected earlier
   constraints.
6. Turns 21-24 reconcile progress, remaining questions, and the next action.

The repository owns every user question. Questions should sound natural, but
each one also has a declared purpose such as introducing a critical entity,
overriding a stale fact, or testing long-range recall.

The external model owns only assistant answers. Each request includes the
scenario system instruction and the complete generated conversation so far, so
answers can naturally reference prior context.

## Controlled Question Script

The script is stored separately from generated output. Each turn contains:

```json
{
  "turn": 12,
  "phase": "investigation",
  "purpose": "leave the automatic scoring threshold unresolved",
  "user": "自动评分阈值现在能定下来吗？如果证据还不够，请明确保留为待确认项。"
}
```

The script must deliberately include:

- exact entities such as component versions, error codes, and configuration
  keys
- hard constraints that remain active across later topic changes
- hypotheses that must remain unresolved
- at least one explicit decision replacement
- an explicit statement that the superseded decision is invalid
- distractor details that may safely disappear during compression
- late questions whose correct answers depend on early conversation state

Questions must not contain the expected answer to every test. Some turns should
ask the model to analyze or explain information already established in the
conversation, producing realistic variation in answer length.

## Generation Flow

The generator performs the following sequence:

1. Load and validate the 24-turn question script.
2. Create or resume a raw generation record.
3. For the next unanswered turn, send the system instruction, all completed
   messages, and the current user question to the configured external model.
4. Validate that the answer is non-empty.
5. Append the user question, assistant answer, request metadata, and cumulative
   token count.
6. Atomically persist the raw record before starting the next turn.
7. Stop after turn 24 or return the external-call error while retaining all
   completed turns.

Generation is sequential because each answer depends on prior answers.
Resuming must not regenerate completed turns unless an explicit overwrite mode
is requested.

## Raw Artifact

The raw artifact records provenance and supports interrupted runs:

```json
{
  "schema_version": 1,
  "scenario_id": "software_project_state_transitions_v1",
  "status": "in_progress",
  "provider": "openai-compatible",
  "model": "resolved-model-name",
  "estimator": {
    "name": "tokenestimate",
    "version": "v0.1.0",
    "message_overhead_tokens": 4
  },
  "turns": [
    {
      "turn": 1,
      "phase": "initial_scope",
      "purpose": "establish the initial goal",
      "user": "我们先确认这个项目当前要解决的核心问题。",
      "assistant": "当前核心问题是建立可重复执行的 summary 策略评估。",
      "cumulative_tokens": 214,
      "generated_at": "2026-06-23T10:30:00Z"
    }
  ]
}
```

Secrets, API keys, raw request headers, and hidden provider responses must not
be persisted.

`provider` and `model` must contain the concrete values resolved for that run;
the strings above illustrate the field shape rather than required constants.

The raw artifact belongs under `tmp/` by default. A reviewed fixture may be
copied into `testdata/evals/summary/generated/` when reproducibility requires
it.

## Token Measurement and Stop Conditions

Token measurement uses `tokenbudget.DefaultEstimator` and the configured
per-message overhead, matching strategy evaluation.

The primary run always attempts all 24 turns. It is considered suitable for the
800/1200/1600 sweep only when:

- cumulative tokens cross 800 before the end of the conversation
- cumulative tokens cross 1200 at a later completed turn
- cumulative tokens cross 1600 at another later completed turn
- final cumulative tokens are at least 2400

If the final conversation remains below 2400 tokens, revise selected questions
to request concrete analysis, alternatives, or implementation implications and
generate a new scenario version. Do not pad answers with repeated filler.

## Evaluation Sample Authoring

External answers are source material, not gold annotations.

After generation:

1. Review the dialogue for contradictions, accidental unsupported claims, and
   whether all planned state transitions actually occurred.
2. Copy only `role` and `content` into `input.source_messages`.
3. Add checkpoints after turns 6, 12, 18, and 24.
4. Add `final_eval` after turn 24.
5. Author `expected_summary`, `critical_contract`, and `next_turn_eval` from
   evidence explicitly present in the final dialogue.
6. Validate the sample with the existing evaluator before running an external
   threshold sweep.

No scoring-relevant annotation may rely solely on the question script if the
generated dialogue did not actually establish that fact.

## Failure Handling

- Invalid script: fail before any external request.
- Existing artifact with a different scenario or model contract: fail rather
  than mixing runs.
- Empty answer: do not advance the turn.
- External timeout or provider failure: retain completed turns and report the
  next resumable turn.
- Malformed raw artifact: fail without overwriting it.
- Insufficient final token count: mark the artifact complete but unsuitable for
  the target sweep.

## Testing

Tests use a fake chat model and cover:

- questions are sent in order with accumulated context
- each successful answer is persisted
- interrupted generation resumes at the next unanswered turn
- completed turns are not regenerated
- empty answers do not advance progress
- cumulative tokens include message overhead
- scenario suitability reflects all token crossing requirements
- raw artifacts omit credentials and provider headers
- conversion preserves only role/content in evaluation source messages

The real external-model run is a separate acceptance step and requires explicit
approval before transmitting the controlled question script and accumulated
conversation to the configured provider.

## Success Criteria

The work is complete when:

- one resumable 24-turn conversation has been generated with the configured
  external model
- its final measured size is at least 2400 tokens and it crosses all three
  target thresholds at distinct completed turns
- the generated dialogue contains the planned constraint, open-question, state
  override, goal-shift, and long-range recall behaviors
- a reviewed strategy sample with checkpoints at turns 6, 12, 18, and 24
  validates successfully
- the 800/1200/1600 strategy sweep triggers compression and produces
  distinguishable strategy results
