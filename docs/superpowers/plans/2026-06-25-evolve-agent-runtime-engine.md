# Evolve Agent Runtime Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Converge `internal/app/agent` onto one shared runtime engine that preserves `reactive` and `plan_execute` as patterns while normalizing runtime decisions, approval/resume, scheduler, budget/error ownership, event journal, and compatibility boundaries.

**Architecture:** Keep the existing `service -> pattern compile -> kernel.Runner` shape, but formalize a runtime facade in front of `kernel.Runner` and move shared lifecycle mechanics into `runtime`, `state`, and a scheduler contract. Patterns remain responsible for strategy and node flow only; session, checkpoint, approval, replay, event, runtime decisions, budget policy, and error policy become runtime-owned.

**Tech Stack:** Go, OpenSpec, existing `internal/app/agent/{runtime,state,kernel,pattern,capability}` packages, Go tests

---

## File Map

### Existing directories that already hold the right responsibilities

- `internal/app/agent/runtime`
  - Session container, stores, replay, projection, pending approval lookup.
- `internal/app/agent/state`
  - Snapshot, delta, reducer, runtime events.
- `internal/app/agent/kernel`
  - Graph runner, checkpointing, journal append, reducer application.
- `internal/app/agent/pattern`
  - Shared runtime config and pattern assembly entrypoints.
- `internal/app/agent/pattern/reactive`
  - Reactive strategy and node flow.
- `internal/app/agent/pattern/planexecute`
  - Plan/step strategy and node flow.
- `internal/app/agent/capability`
  - Capability spec, registry, invocation, resolver, selector.
- `internal/app/agent`
  - Service boundary, approval/resume API, response compatibility tests.

### Files expected to be introduced or expanded

- `internal/app/agent/runtime/engine.go`
- `internal/app/agent/runtime/engine_test.go`
- `internal/app/agent/runtime/decision.go`
- `internal/app/agent/runtime/outcome.go`
- `internal/app/agent/runtime/scheduler.go`
- `internal/app/agent/runtime/scheduler_test.go`
- `internal/app/agent/runtime/policy.go`
- `internal/app/agent/runtime/policy_test.go`
- `internal/app/agent/state/invariants_test.go`
- `internal/app/agent/state/version.go`

## Recommended Execution Order

The OpenSpec task list is correct, but the safest implementation order is:

1. Freeze contracts and names first.
2. Harden shared state model before extracting more lifecycle logic.
3. Add runtime facade before moving approval/scheduler logic.
4. Introduce scheduler contract.
5. Make runtime decision vocabulary, budget ownership, and error policy explicit.
6. Normalize approval/resume through runtime decisions.
7. Converge both patterns onto the same runtime-owned mechanics.
8. Stabilize event journal and projections.
9. Run compatibility and rollout verification last.

This sequence reduces the chance of duplicating logic in both patterns and avoids stabilizing events before state ownership is clear.

### Task 1: Freeze Runtime Contracts and Naming

**Covers OpenSpec:** `1. Freeze Runtime Contracts`

**Files:**
- Modify: `openspec/changes/evolve-agent-runtime-engine/tasks.md`
- Modify: `openspec/changes/evolve-agent-runtime-engine/design.md`
- Modify: `internal/app/agent/runtime/session.go`
- Modify: `internal/app/agent/service_assembly.go`
- Modify: `internal/app/agent/service.go`
- Modify: `internal/app/agent/request_response.go`
- Test: `internal/app/agent/service_contract_test.go`
- Test: `internal/app/agent/service_session_boundary_test.go`

- [ ] Confirm the contract vocabulary to keep:
  - `RuntimeSession` = runtime state container
  - `kernel.Runner` = compiled graph executor
  - `runtime.Engine` or equivalent = run/resume/outcome facade
  - `StateDelta` + `Reducer` = only snapshot write path
  - `RuntimeEvent` = shared journal event model

- [ ] Remove or rename code paths that imply parallel concepts such as service-owned runtime lifecycle or pattern-owned approval lifecycle.

- [ ] Add or update package comments where these contracts live so the names are discoverable from code, not only from OpenSpec.

- [ ] Verify current naming assumptions with focused tests.

Run:

```bash
go test ./internal/app/agent -run "Contract|SessionBoundary" -v
```

Expected:

- Existing service/session contract tests still pass.
- No new code introduces a second session or engine abstraction.

### Task 2: Harden the Shared State Model

**Covers OpenSpec:** `2. Strengthen Runtime State Model`

**Files:**
- Modify: `internal/app/agent/state/snapshot.go`
- Modify: `internal/app/agent/state/delta.go`
- Modify: `internal/app/agent/state/reducer.go`
- Modify: `internal/app/agent/state/event.go`
- Modify: `internal/app/agent/runtime/projection.go`
- Modify: `internal/app/agent/runtime/replay.go`
- Test: `internal/app/agent/state/reducer_test.go`
- Test: `internal/app/agent/runtime/projection_test.go`
- Test: `internal/app/agent/runtime/replay_test.go`
- Create: `internal/app/agent/state/invariants_test.go`

- [ ] Keep the existing top-level snapshot domains and explicitly document ownership for:
  - `request`
  - `context`
  - `plan`
  - `evidence`
  - `approval`
  - `execution`
  - `answer`

- [ ] Add snapshot versioning or compatibility metadata in `state/snapshot.go` or `state/version.go`. Do not hide versioning inside service metadata; it belongs to replayable state.

- [ ] Define a stable extension area for pattern-private state instead of allowing ad hoc overloading of generic fields. Use the existing `Pattern` section inside `StateSnapshot` as the only documented pattern-private domain.

- [ ] Tighten reducer invariants:
  - approval states cannot be both pending and reviewed
  - execution cannot be both interrupted and completed
  - answer `draft`, `degrade`, and `final` must have deterministic overwrite rules
  - plan updates must not silently corrupt step sequencing

- [ ] Make replay and projection read only from shared state and journal, not from pattern-private assumptions.

- [ ] Add tests for:
  - invalid approval/execution/answer combinations
  - replay recovery of pending approval and final outcome
  - old snapshot compatibility behavior

Run:

```bash
go test ./internal/app/agent/state ./internal/app/agent/runtime -v
```

Expected:

- Reducer rejects invalid state combinations.
- Replay/projection can reconstruct pending approval and final outcome without pattern-private reads.

### Task 3: Extract the Runtime Engine Facade

**Covers OpenSpec:** `4. Add Runtime Engine Facade`

**Files:**
- Create: `internal/app/agent/runtime/engine.go`
- Create: `internal/app/agent/runtime/decision.go`
- Create: `internal/app/agent/runtime/outcome.go`
- Create: `internal/app/agent/runtime/engine_test.go`
- Modify: `internal/app/agent/service_run.go`
- Modify: `internal/app/agent/service_resume.go`
- Modify: `internal/app/agent/service_response.go`
- Modify: `internal/app/agent/service_assembly.go`
- Modify: `internal/app/agent/kernel/runner.go`
- Test: `internal/app/agent/service_resume_contract_test.go`
- Test: `internal/app/agent/service_flow_test.go`

- [ ] Move run/resume/outcome normalization into a runtime facade owned by `internal/app/agent/runtime`.

- [ ] Keep `kernel.Runner` focused on compiled graph execution, reducer application, checkpointing, and event append.

- [ ] Make the service construct or load `RuntimeSession`, invoke the runtime facade, and then map runtime outcome to the existing response shape.

- [ ] Normalize at least these lifecycle outcomes:
  - continue
  - wait approval
  - replan
  - resume completed
  - rejected
  - degraded
  - completed
  - failed

- [ ] Preserve current external API shape while moving mechanics inward.

Run:

```bash
go test ./internal/app/agent -run "Flow|ResumeContract" -v
```

Expected:

- Existing service behavior still passes through the same response contract.
- Runtime facade, not service glue, owns run/resume normalization.

### Task 4: Introduce the Capability Scheduler Contract

**Covers OpenSpec:** `5. Introduce Capability Scheduler Contract`

**Files:**
- Create: `internal/app/agent/runtime/scheduler.go`
- Create: `internal/app/agent/runtime/scheduler_test.go`
- Create: `internal/app/agent/runtime/policy.go`
- Create: `internal/app/agent/runtime/policy_test.go`
- Modify: `internal/app/agent/capability/spec.go`
- Modify: `internal/app/agent/capability/invocation.go`
- Modify: `internal/app/agent/pattern/config.go`
- Modify: `internal/app/agent/pattern/planexecute/nodes_execute_step.go`
- Modify: `internal/app/agent/pattern/reactive/capability_policy.go`
- Modify: `internal/app/agent/pattern/reactive/nodes_search.go`
- Modify: `internal/app/agent/pattern/reactive/nodes_fetch.go`
- Test: `internal/app/agent/capability/contract_test.go`

- [ ] Define scheduler input around:
  - capability spec
  - runtime options
  - current snapshot
  - requested pattern action
  - approval status
  - retry/resume context

- [ ] Define normalized scheduler output around:
  - execute
  - wait approval
  - skip
  - retry
  - degrade
  - fail

- [ ] Start consuming existing spec policy fields centrally:
  - `RequiresApproval`
  - `SupportsParallel`
  - `SupportsResume`
  - `RiskLevel`
  - `Idempotency`
  - `Preconditions`

- [ ] Make `SupportsParallel` and `SupportsResume` visible as shared runtime policy, even if first-stage execution remains mostly sequential.

- [ ] Introduce a small runtime policy layer for:
  - canonical error classes
  - error-to-decision mapping
  - runtime-owned budget policy hooks

- [ ] Change pattern nodes so they submit capability intent and receive scheduler decisions, rather than each pattern interpreting capability policy on its own.

Run:

```bash
go test ./internal/app/agent/runtime ./internal/app/agent/capability ./internal/app/agent/pattern/... -v
```

Expected:

- Approval-gated capability execution cannot bypass scheduler.
- Parallel-safe work can still be grouped.
- Resume behavior is explicit for non-resumable capabilities.

### Task 5: Make Runtime Decision, Budget, and Error Ownership Explicit

**Covers OpenSpec:** `4. Add Runtime Engine Facade`, `5. Introduce Capability Scheduler Contract`

**Files:**
- Create: `internal/app/agent/runtime/decision.go`
- Create: `internal/app/agent/runtime/outcome.go`
- Create: `internal/app/agent/runtime/policy.go`
- Create: `internal/app/agent/runtime/policy_test.go`
- Modify: `internal/app/agent/runtime/engine.go`
- Modify: `internal/app/agent/runtime/engine_test.go`
- Modify: `internal/app/agent/request_response.go`
- Modify: `internal/app/agent/runtime/replay.go`
- Test: `internal/app/agent/service_flow_test.go`
- Test: `internal/app/agent/runtime/replay_test.go`

- [ ] Freeze the canonical runtime decisions:
  - `continue`
  - `wait_approval`
  - `resume`
  - `reject`
  - `retry`
  - `replan`
  - `degrade`
  - `complete`
  - `fail`

- [ ] Ensure `runtime.Engine` is the only layer that turns graph execution state into canonical runtime decisions and outcomes.

- [ ] Keep budget policy runtime-owned:
  - session/run level budget policy belongs to runtime
  - scheduler may consult budget context
  - patterns must not invent their own global budget semantics

- [ ] Keep error policy runtime-owned:
  - runtime/scheduler define canonical error classes
  - patterns may branch on those shared classes
  - patterns must not expose their own runtime-visible error taxonomy

Run:

```bash
go test ./internal/app/agent/runtime ./internal/app/agent -run "Engine|Flow" -v
```

Expected:

- Runtime decisions are normalized in one place.
- Replay and outward outcomes can rely on canonical decisions.
- Budget/error ownership no longer depends on per-pattern private logic.

### Task 6: Normalize Approval and Resume Through Runtime Decisions

**Covers OpenSpec:** `6. Normalize Approval and Resume`

**Files:**
- Modify: `internal/app/agent/service_approval.go`
- Modify: `internal/app/agent/service_approval_resume.go`
- Modify: `internal/app/agent/service_pending_approval.go`
- Modify: `internal/app/agent/service_resume.go`
- Modify: `internal/app/agent/runtime/pending_approval_store.go`
- Modify: `internal/app/agent/runtime/session.go`
- Modify: `internal/app/agent/runtime/engine.go`
- Modify: `internal/app/agent/pattern/planexecute/nodes_approval.go`
- Modify: `internal/app/agent/pattern/reactive/nodes_approval.go`
- Test: `internal/app/agent/service_approval_lifecycle_test.go`
- Test: `internal/app/agent/service_pending_approval_test.go`

- [ ] Make `approval pending` a runtime decision, not a pattern-private side effect.

- [ ] Make `approval resolved`, `approval rejected`, and `resume completed` runtime events emitted through the same journal contract.

- [ ] Store checkpoint id, rerun node, approval note, and decision metadata on runtime-owned session/state fields.

- [ ] Reduce pattern-specific approval logic to choosing when approval is needed; the runtime decides how interruption, pending lookup, resume, and rejection are persisted.

Run:

```bash
go test ./internal/app/agent -run "Approval|PendingApproval|Resume" -v
```

Expected:

- Pending approval can be restored to UI.
- Approved runs resume from checkpoint.
- Rejected runs produce stable degraded/final outcome.

### Task 7: Converge Both Patterns Under One Engine

**Covers OpenSpec:** `3. Preserve Patterns Under One Engine`

**Files:**
- Modify: `internal/app/agent/pattern/config.go`
- Modify: `internal/app/agent/service_assembly.go`
- Modify: `internal/app/agent/pattern/reactive/builder.go`
- Modify: `internal/app/agent/pattern/reactive/pattern_test.go`
- Modify: `internal/app/agent/pattern/planexecute/builder.go`
- Modify: `internal/app/agent/pattern/planexecute/pattern_test.go`
- Modify: `internal/app/agent/pattern/planexecute/checkpoint_regression_test.go`
- Test: `internal/app/agent/service_pattern_test.go`
- Test: `internal/app/agent/service_pattern_planexecute_test.go`

- [ ] Keep `reactive` and `plan_execute` as registered patterns compiled through the same assembly path.

- [ ] Restrict patterns to strategy concerns only:
  - next action
  - observe/evaluate
  - replan
  - continue/degrade/stop
  - pattern-private state only in the explicit extension area

- [ ] Remove any direct pattern bypass around:
  - approval persistence
  - pending approval lookup
  - scheduler policy
  - externally visible runtime event protocol

- [ ] Keep `plan_execute` as the default agent service pattern.

Run:

```bash
go test ./internal/app/agent/pattern/... ./internal/app/agent -run "Pattern" -v
```

Expected:

- Both patterns compile and run through one runtime contract.
- Default pattern remains `plan_execute`.

### Task 8: Stabilize Runtime Events and External Projections

**Covers OpenSpec:** `7. Stabilize Runtime Events`

**Files:**
- Modify: `internal/app/agent/state/event.go`
- Modify: `internal/app/agent/kernel/journal.go`
- Modify: `internal/app/agent/kernel/builder.go`
- Modify: `internal/app/agent/runtime/replay.go`
- Modify: `internal/app/agent/runtime/projection.go`
- Modify: `internal/app/agent/handoff/build.go`
- Modify: `internal/app/agent/request_response.go`
- Test: `internal/app/agent/runtime/replay_test.go`
- Test: `internal/app/agent/runtime/projection_test.go`
- Test: `internal/app/agent/kernel/kernel_test.go`

- [ ] Normalize event families for:
  - session lifecycle
  - node lifecycle
  - decision
  - branch
  - capability start/result/skipped
  - checkpoint recorded
  - approval pending/resolved/rejected
  - checkpoint/interruption
  - resume completed
  - state applied
  - answer finalized
  - degraded/failed

- [ ] Ensure event sequence remains append-only and monotonically increasing from the session journal.

- [ ] Make replay, handoff, pending approval restore, and SSE/response projections consume the shared event journal instead of pattern-private event assumptions.

Run:

```bash
go test ./internal/app/agent/kernel ./internal/app/agent/runtime ./internal/app/agent/handoff -v
```

Expected:

- Replay and projection recover key runtime state from the journal.
- Event consumers no longer depend on pattern-private event shapes.

### Task 9: Preserve Compatibility Boundaries

**Covers OpenSpec:** `8. Keep Compatibility Boundaries`

**Files:**
- Modify: `internal/app/agent/service.go`
- Modify: `internal/app/agent/service_response.go`
- Modify: `internal/app/agent/request_response.go`
- Test: `internal/app/agent/service_contract_test.go`
- Test: `internal/app/agent/service_pattern_test.go`
- Test: `internal/bootstrap/rag/runtime_build_chat_test.go`

- [ ] Keep ordinary RAG chat off the new runtime path unless the existing routing explicitly selects agent runtime.

- [ ] Do not delete or silently reroute `internal/app/rag/tool`.

- [ ] Preserve frontend-facing approval event fields and pending approval restore behavior.

- [ ] Keep current agent service request/response compatibility.

Run:

```bash
go test ./internal/app/agent -run "Contract|Pattern|PendingApproval" -v
```

Expected:

- Agent service remains backward compatible.
- Approval payload shape remains consumable by existing frontend code.

### Task 10: Verification and OpenSpec Rollout Gate

**Covers OpenSpec:** `9. Verification and Rollout`

**Files:**
- Modify: `openspec/changes/evolve-agent-runtime-engine/tasks.md`
- Modify: `openspec/changes/evolve-agent-runtime-engine/design.md`
- Modify: `openspec/changes/evolve-agent-runtime-engine/specs/agent-runtime/spec.md`
- Test: all updated runtime/pattern/service suites

- [ ] Run the targeted runtime, state, pattern, approval, and compatibility suites after each phase instead of saving all verification for the end.

- [ ] Run the final focused verification set:

```bash
go test ./internal/app/agent/... -v
go test ./internal/app/rag/... -v
openspec validate evolve-agent-runtime-engine --strict
```

- [ ] Only mark the OpenSpec tasks done when all four are true:
  - multiple patterns provably use one runtime engine
  - approval/checkpoint/event/scheduler are runtime-owned
  - budget/error policy are runtime-owned
  - default production path has not been forcibly switched

## Suggested Grouping by Code Directory

Use this map when assigning work:

- `internal/app/agent/state`
  - Best place for Task 2 and part of Task 7.
- `internal/app/agent/runtime`
  - Best place for Tasks 3, 4, 5, 6, and part of Task 8.
- `internal/app/agent/kernel`
  - Best place for Task 3 and the event/checkpoint mechanics in Task 8.
- `internal/app/agent/pattern/reactive`
  - Best place for Task 7 and scheduler adoption changes in Task 4.
- `internal/app/agent/pattern/planexecute`
  - Best place for Task 7 and scheduler adoption changes in Task 4.
- `internal/app/agent/capability`
  - Best place for Task 4 policy contract cleanup.
- `internal/app/agent`
  - Best place for service boundary compatibility, approval/resume API, and facade wiring in Tasks 1, 3, 5, 6, and 9.
- `openspec/changes/evolve-agent-runtime-engine`
  - Best place for keeping spec, design, and task checkboxes aligned with the shipped implementation.

## Sequencing Notes

- Do not start with scheduler extraction. It depends on clear runtime ownership and state invariants.
- Do not start with budget/error policy extraction before runtime decisions are explicit.
- Do not stabilize event projections before approval/resume semantics are runtime-owned.
- Do not rewrite both patterns at once. First land the shared facade and scheduler contract, then migrate one pattern at a time.
- Migrate `plan_execute` first because it is the default pattern and already carries the richer approval/step semantics. Then align `reactive` to the same runtime-owned mechanics.

## Spec Coverage Check

- OpenSpec Task 1 maps to plan Task 1.
- OpenSpec Task 2 maps to plan Task 2.
- OpenSpec Task 3 maps to plan Task 7.
- OpenSpec Task 4 maps to plan Task 3.
- OpenSpec Task 5 maps to plan Tasks 4 and 5.
- OpenSpec Task 6 maps to plan Task 6.
- OpenSpec Task 7 maps to plan Task 8.
- OpenSpec Task 8 maps to plan Task 9.
- OpenSpec Task 9 maps to plan Task 10.

No spec section is intentionally dropped; the main difference is sequencing.
