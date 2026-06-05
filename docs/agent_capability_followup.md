# Agent Capability Follow-up

This note captures the remaining capability-platform work after the current
`fetch` / resolver precondition fix.

## Completed in this pass

- `fetch` can now preserve its skip semantics through the
  `resolver -> invoke` path.
- Resolver precondition handling now allows an explicit capability opt-in for
  invoke-time handling of precondition failures.

## Remaining Work

### 1. Generalize workflow output decoding

Current workflow-style capabilities such as `external_evidence` still need
hand-written output type assertions for downstream capability calls.

Suggested follow-up:

- add shared typed output decode helpers in `internal/app/agent/capability`
- migrate workflow capabilities away from custom `decodeXOutput(...)`

### 2. Reduce root assembly friction for new families

Adding a capability inside an existing family is smoother now, but adding a
new family or new dependency type still requires changes in:

- capability constants
- service options
- service assembly groups

Suggested follow-up:

- formalize capability family registration groups
- centralize assembly dependency wiring rules

### 3. Add explicit resolver tests

The current regression coverage now proves the `fetch` skip path, but the
resolver package still has no direct package-level tests.

Suggested follow-up:

- add `capability/resolve` tests for:
  - precondition fallback pass-through
  - invalid input rejection
  - ambiguous selector rejection

### 4. Revisit fetch precondition policy

`fetch` currently treats empty URL input as a valid skip outcome rather than an
invalid invocation.

That behavior is now consistent across direct invoke and resolver paths, but it
is still a product/runtime policy decision worth documenting explicitly.
