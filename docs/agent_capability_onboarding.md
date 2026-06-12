# Agent Capability Onboarding

This guide describes the required steps for adding a new non-diagnostic
capability under `internal/app/agent`.

## Goal

New capabilities should plug into the existing runtime through the same
governed path:

`registry -> catalog -> selector -> resolver -> execution`

When adding a capability, prefer following the existing non-diagnostic samples:

- `search`
- `fetch`
- `external_evidence`
- `think`
- `knowledge_discovery`
- `memory_recall`
- `content_summarize`

Do not treat `document_investigation` as the default template for new
capabilities unless the new work is explicitly diagnosis-oriented.

## Required Implementation Checklist

1. Define typed input and output contracts.
2. Define a complete `capability.Spec`.
3. Implement `NormalizeInput(...)`.
4. Implement `Invoke(...)`.
5. Project any required `StateDelta` and `EvidenceRefs`.
6. Register the capability in the correct assembly group.
7. Add shared contract coverage.
8. Add selector / resolver coverage.
9. Add at least one service-level or pattern-level integration check.

## Capability Shape

Each capability should provide:

```go
type CapabilityInput struct {
    // Typed runtime input.
}

type CapabilityOutput struct {
    // Typed runtime output.
}
```

`Spec` must declare:

- `Name`
- `Kind`
- `Family`
- `Roles`
- `Description`
- `InputSchema`
- `OutputSchema`
- `RiskLevel`
- `SupportsParallel`
- `SupportsResume`
- `ProducesEvidence`
- `Idempotency`
- `Preconditions`

The registry rejects unsupported:

- family values
- role values
- risk levels
- idempotency values
- precondition requirements

## Input Contract

Use the shared structured-input entrypoint:

```go
input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](
    c.spec,
    req.Input,
    "capability input is required",
    "capability input",
)
```

This provides:

- structured JSON-like input decoding
- typed input normalization
- declared precondition validation

For special control-flow paths such as a fetch-style skip on empty input, use
`agentcapability.IsPreconditionError(err)` to branch explicitly.

## Failure Results

Use the shared failure builders when the capability should degrade:

```go
agentcapability.ValidationFailureResult(spec, "capability rejected", err)
agentcapability.DependencyFailureResult(spec, "dependency lookup failed", err)
agentcapability.ExternalFailureResult(spec, "external call failed", err)
```

Capabilities remain responsible for:

- action summaries
- any skip behavior
- domain-specific deltas
- domain-specific evidence projection

## Invoke Error Semantics

New capabilities should follow this `(result, err)` contract:

- `StatusSucceeded` or `StatusSkipped` -> `err == nil`
- `StatusDegraded` with usable output -> `err == nil`
- validation / dependency / external hard failures -> use the shared failure builders and return `err != nil`

Use `ValidationFailureResult` for invalid input, `DependencyFailureResult` for upstream service failures, and `ExternalFailureResult` for third-party or LLM failures.

## Registration

Add the capability to the matching registration group inside agent assembly.

Current groups:

- `external evidence`
- `optional workflow`
- `meta` (`think`, `content_summarize` when LLM is available)
- `discovery` (`knowledge_discovery`, optional dependency)
- `memory` (`memory_recall`, optional dependency)

Family to workflow projection is centralized in `capability.WorkflowCapabilityForFamily`.

Rules:

- construct capability dependencies first
- register through the group helper
- keep role bindings explicit
- do not hide binding policy inside the registry

## Testing

Every new capability should cover:

1. package-local behavior tests for domain logic
2. shared capability contract coverage for:
   - registry registration
   - catalog projection
   - resolver match / resolve
   - structured input normalization
   - invalid input rejection
   - successful invocation result shape
3. one service-level or pattern-level integration test

## Example Pattern

Use `search`, `fetch`, and `external_evidence` as the preferred references for:

- small typed input contracts
- shared input decoding
- shared degraded result construction
- capability registration in assembly

## Architecture Improvement Backlog

The following items are intentionally deferred and should not block new capability delivery:

1. `TypedHandle[I, O]` at the invoke boundary to remove node-level type assertions.
2. Runtime taxonomy registration for families/roles instead of static maps in `spec.go`.
3. Unified evidence ownership: capabilities with `ProducesEvidence: true` should populate `EvidenceDelta` directly.
4. Journal-driven handoff profiles for plan-execute and non-reactive patterns.
5. Generic capability node/slot config to reduce reactive hardcoded node names.
6. LLM selector nil-service behavior and expanded contract coverage for skipped/degraded paths.
