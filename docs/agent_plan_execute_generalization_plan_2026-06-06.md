# Agent Plan-Execute Generalization Plan

Date: 2026-06-06

## 1. Goal

The short-term goal is not to make `plan_execute` a longer external-search
workflow.

The goal is to upgrade `internal/app/agent/pattern/planexecute` from a
search/fetch-specialized executor into a **general capability plan executor**
that can:

- plan across multiple capability families
- pass structured intermediate outputs between steps
- evaluate step completion through reusable policies
- replan locally when a step fails or produces insufficient artifacts

In other words, `plan_execute` should become a strong execution mode for
multi-step capability orchestration, not only a web-evidence variant.

## 2. Current Limitation

The current implementation already has a useful skeleton:

- `build_plan`
- `select_step`
- `execute_step`
- `assess_step`
- `finalize`

However, its planning semantics are still too narrow.

Main limitations:

1. The default plan shape is still essentially `search -> fetch`.
2. `assess_step` still contains capability-specific logic, especially around
   search and fetch.
3. Step-to-step data transfer is weak. Most intermediate state is implicitly
   inferred from snapshot context rather than explicitly modeled as plan
   artifacts.
4. Plan generation and plan execution are still too tightly coupled.

This means the runtime can execute a plan graph, but it cannot yet serve as a
truly general multi-capability planner-executor.

## 3. Design Direction

The design direction is:

1. keep the current runtime/service outward contract stable
2. generalize the plan model first
3. make step completion policy-driven
4. make intermediate artifacts explicit
5. separate plan synthesis from plan execution

This keeps the existing service/runtime boundaries intact while making
`plan_execute` more expressive internally.

## 4. Refactor Phases

### Phase 1: Generalize the Plan Model

First expand the plan model so a step can represent more than a thin wrapper
around one search or fetch call.

Primary target:

- [snapshot.go](/d:/goagent/internal/app/agent/state/snapshot.go)

Planned `PlanStep` additions:

- `Goal`
- `Consumes`
- `Produces`
- `CompletionPolicy`
- `FailurePolicy`
- `Optional`
- `MaxAttempts`

Planned `PlanStepResult` additions:

- structured `Artifacts`
- richer execution result metadata that later steps can consume

Acceptance criteria:

- a step can describe what it is trying to achieve
- a step can describe what it consumes and produces
- step semantics are no longer implicitly bound to search/fetch only

### Phase 2: Introduce Explicit Step Artifacts

The next step is to make intermediate outputs explicit rather than relying on
ad hoc reads from runtime context.

Suggested new files:

- `internal/app/agent/pattern/planexecute/artifacts.go`
- `internal/app/agent/pattern/planexecute/artifacts_test.go`

First artifact types should be small and practical:

- `urls`
- `search_results`
- `fetch_results`
- `evidence_refs`
- `structured_output`
- `diagnosis_summary`

Important boundary:

- start inside `planexecute`
- do not immediately expand the entire global runtime state model

Acceptance criteria:

- later steps can explicitly consume upstream artifacts
- multi-capability flows do not need to guess state from unrelated context

### Phase 3: Replace Capability-Specific Assessment with Policies

This is the key generalization step.

Today `nodes_assess_step.go` still contains capability-specific branching such
as:

- search expects search results
- fetch expects fetch evidence
- others fall back to `ProducedEvidence`

That approach does not scale.

Primary target:

- [nodes_assess_step.go](/d:/goagent/internal/app/agent/pattern/planexecute/nodes_assess_step.go)

Suggested new files:

- `internal/app/agent/pattern/planexecute/assessment_policy.go`
- `internal/app/agent/pattern/planexecute/assessment_policy_test.go`

First policy set:

- `expect_search_results`
- `expect_fetch_results`
- `expect_evidence`
- `expect_structured_output`
- `expect_non_empty_observation`

Acceptance criteria:

- new capabilities can usually integrate by choosing a policy
- `assess_step` no longer needs to keep growing a large capability switch

### Phase 4: Split Plan Synthesis from Plan Execution

`plan_execute` should distinguish:

- how a plan is created
- how a plan is executed

Primary target:

- [nodes_build_plan.go](/d:/goagent/internal/app/agent/pattern/planexecute/nodes_build_plan.go)

Suggested new files:

- `internal/app/agent/pattern/planexecute/synthesizer.go`
- `internal/app/agent/pattern/planexecute/synthesizer_default.go`
- `internal/app/agent/pattern/planexecute/synthesizer_test.go`

Proposed seam:

- `PlanSynthesizer` interface

First synthesis modes:

1. deterministic template plan
2. selector-driven single-capability plan
3. mixed-capability template plan

Acceptance criteria:

- the runtime can evolve planning logic without rewriting execution nodes
- `build_plan` stops being a hardcoded helper-only template

### Phase 5: Validate with More Than One Capability Family

Once the model is generalized, validate that `plan_execute` is not only a web
evidence executor.

Recommended first validation set:

- `external_evidence`
- `document_investigation`

Target scenarios:

1. single-capability single-step plan
2. mixed-capability multi-step plan
3. one capability family produces artifacts that drive a later step
4. one capability fails and another capability family is used to continue or
   replan

Acceptance criteria:

- `plan_execute` proves it can orchestrate different capability families

### Phase 6: Re-lock Service-Level Contract Safety

After the internal pattern changes are stable, extend service-level coverage.

Primary target:

- [service_pattern_test.go](/d:/goagent/internal/app/agent/service_pattern_test.go)

Recommended coverage:

1. mixed-capability answer path
2. mixed-capability handoff path
3. approval and resume for non-search/fetch steps
4. duplicate resume after mixed-capability terminal state

Acceptance criteria:

- `plan_execute` becomes more powerful without reopening service outward
  contract drift

## 5. Recommended Implementation Order

The recommended order is:

1. generalize `PlanStep` and `PlanStepResult`
2. add explicit plan artifacts
3. refactor assessment into policy-driven logic
4. extract plan synthesis seam
5. add mixed-capability pattern tests
6. add service-level regression tests

This order keeps the runtime executable at each stage while reducing the risk
of introducing broad regressions.

## 6. What Not To Do First

To avoid accidental overreach, the first refactor should **not** start with:

1. free-form LLM-generated arbitrary plans
2. broad outward service contract changes
3. large global state expansion across every runtime domain
4. sub-agent planning or multi-agent orchestration
5. continuously growing capability-specific branching in `assess_step`

Those can come later, but they should not be the first move.

## 7. Near-Term Definition of Success

`plan_execute` should be considered meaningfully upgraded when:

1. a plan step can express reusable execution semantics
2. steps can pass explicit artifacts to downstream steps
3. completion is judged by policies rather than capability-name branching
4. plan generation is replaceable through a synthesis seam
5. at least two capability families can be orchestrated under the same pattern

At that point, `plan_execute` will no longer be just a search/fetch-specialized
executor. It will be a true general capability plan executor within the current
agent runtime.

## 8. Implementation Update: 2026-06-07

### Status Update

As of `2026-06-07`, the generalization work described in this document is now
materially implemented across all six phases of the near-term plan.

This means the document should now be read as both:

- the original refactor intent
- a record of what the current implementation has already validated

The implementation now includes:

- generalized step semantics and step results
- explicit step artifacts plus artifact-first downstream input preparation
- policy-driven completion assessment
- a replaceable `PlanSynthesizer` seam
- mixed-capability orchestration across document and external-evidence families
- service-level regression coverage for mixed flows and non-search/fetch approval

### What Has Landed

#### Phase 1: Generalize the Plan Model - materially landed

`PlanStep` has now been expanded beyond thin search/fetch wrappers.

The current model now includes generalized fields such as:

- `Goal`
- `Consumes`
- `Produces`
- `CompletionPolicy`
- `FailurePolicy`
- `Optional`
- `MaxAttempts`
- `AttemptCount`

`PlanStepResult` has also been expanded to include:

- `Artifacts`
- `Observation`
- attempt metadata
- execution timing metadata

This means step semantics are now better modeled directly in state rather than
being inferred only from legacy `query` / `urls`-style fields.

#### Phase 2: Introduce Explicit Step Artifacts - first closure landed

The suggested artifact seam has now been created in:

- `internal/app/agent/pattern/planexecute/artifacts.go`
- `internal/app/agent/pattern/planexecute/step_inputs.go`
- `internal/app/agent/pattern/planexecute/step_inputs_test.go`

The currently validated artifact kinds are:

- `url_list`
- `search_results`
- `fetch_results`
- `evidence_refs`
- `structured_output`

This is still intentionally local to `planexecute`, which matches the original
boundary recommendation in this document.

#### Phase 3: Replace Capability-Specific Assessment with Policies - materially landed

`assess_step` has now been refactored so that its main completion logic is no
longer organized around a large capability-name switch.

The new policy seam now lives in:

- `internal/app/agent/pattern/planexecute/assessment_policy.go`
- `internal/app/agent/pattern/planexecute/assessment_policy_test.go`

The current first policy set is:

- `expect_search_results`
- `expect_fetch_results`
- `expect_evidence`
- `expect_structured_output`
- `expect_non_empty_observation`

This means the key architectural objective of Phase 3 has already materially
landed.

#### Phase 4: Split Plan Synthesis from Plan Execution - landed

The plan synthesis seam now exists and is used by the runtime.

The current implementation now includes:

- `internal/app/agent/pattern/planexecute/synthesizer.go`
- `internal/app/agent/pattern/planexecute/synthesizer_default.go`
- `internal/app/agent/pattern/planexecute/synthesizer_mixed.go`
- `internal/app/agent/pattern/planexecute/synthesizer_test.go`

`build_plan` now delegates plan creation to a `PlanSynthesizer`, which keeps
plan creation and plan execution from evolving in the same node.

The currently validated synthesis modes are:

1. deterministic template plan
2. selector-driven single-capability plan
3. mixed-capability template plan

#### Phase 5: Validate with More Than One Capability Family - first target landed

`plan_execute` is now validated against more than the original external-search
shape.

The current regression coverage now includes:

1. single-step selector-driven capability execution
2. mixed `document_investigation -> external_evidence` planning
3. multi-step artifact-first chains such as
   `document_investigation -> web_search -> web_fetch`
4. retry and optional-step semantics inside mixed-capability plans

This means the runtime has now demonstrated that it can orchestrate at least
two different capability families under the same pattern.

#### Phase 6: Re-lock Service-Level Contract Safety - landed

Service-level regression coverage has now been extended to match the new
pattern behavior.

The current service coverage now includes:

1. mixed-capability answer path
2. mixed-capability handoff path
3. approval and resume for non-search/fetch document-investigation steps
4. duplicate resume after terminal plan-execute approval outcomes

The primary service regression file is now:

- `internal/app/agent/service_pattern_planexecute_test.go`

### Important Transitional Detail

The current implementation still does **not** assume a broad runtime-level
artifact store.

The current transitional strategy is:

1. emit explicit artifacts into `PlanStepResult`
2. consume artifacts first inside `planexecute`
3. fall back to snapshot context where needed for compatibility

This remains a good incremental shape because it improves explicit step
contracts without forcing a larger runtime-state redesign too early.

### What Still Remains

The original six-phase near-term plan is now effectively closed, but a few
follow-on items remain outside the narrow scope of this document:

- widen mixed-capability templates beyond the first document/external-evidence
  validation set
- continue moving more downstream input preparation onto explicit artifacts
- extend the same generalized behavior into higher-level chat/service entry
  paths that sit above the agent service layer
- keep tightening naming and semantics across `state`, `pattern/planexecute`,
  and `service` as more capability families are onboarded

### Validation

Validated on `2026-06-07`:

```powershell
go test ./internal/app/agent/state ./internal/app/agent/pattern/planexecute -count=1
go test ./internal/app/agent/pattern/planexecute -count=1
go test ./internal/app/agent -run "TestPlanExecuteService_|TestPatternValidation_PlanExecute|TestNewService_PlanExecutePatternRunDetailed|TestServiceRunDetailed_PlanExecuteApprovalIncludesStepContext" -count=1
go test ./internal/app/agent/... -count=1
```

Current result:

- `internal/app/agent/state` PASS
- `internal/app/agent/pattern/planexecute` PASS
- `internal/app/agent` focused `plan_execute` service regressions PASS
- `internal/app/agent/...` PASS

### Updated Near-Term Recommendation

After this `2026-06-07` progress point, the best next move is no longer inside
the original near-term `plan_execute` generalization scope because that scope
is now substantially complete.

The best next move is now:

1. add broader mixed-capability templates beyond the first validated families
2. keep widening artifact-first consumption where step-to-step dataflow matters
3. expand this generalized pattern into higher-level product integration paths
