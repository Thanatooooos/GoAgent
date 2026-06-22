---
name: spec-delivery-audit
description: Use when implementing work from a spec, proposal, design, or tasks checklist and preparing to mark tasks complete, claim delivery, or wire a main entrypoint. Especially useful when module tests pass but real runtime wiring, default assets, result emission, or suite independence may still lag behind.
---

# Spec Delivery Audit

## Overview

Prevent "implemented" from being mistaken for "delivered". Audit spec-driven work across module, integration, and delivery layers before checking boxes, reporting completion, or merging a phase milestone.

## Audit Flow

1. Freeze the promise surface.
   Read the proposal, design, spec delta, and tasks list. Record only what Phase scope explicitly promises. Keep non-goals separate.

2. Start from the formal path, not helper code.
   Open the shipped entrypoint first (`cmd`, API handler, UI action, shared runner registration). Confirm what the user can actually invoke today before inspecting internal helpers.

3. Check three delivery layers.
   - `module`: Do the functions, rules, focused tests, and local behaviors exist?
   - `integration`: Is the behavior reachable through real registry/runtime/dependency wiring?
   - `delivery`: Does the formal entrypoint (`cmd`, API, UI, shared runner) emit the output, artifacts, and behavior the spec promises?

4. Prove the promised defaults.
   Check the checked-in default sample paths, shared assets, fixed config plumbing, and whether each promised suite can run independently without unrelated dependencies.

5. Reconcile task status against the delivery surface.
   A task is not done just because code exists. Downgrade items that are:
   - implemented but not wired
   - wired but not emitted
   - emitted but not runnable from default inputs
   - emitted but not spec-complete

6. Report an audit verdict.
   Classify the current state as `aligned`, `partially aligned`, or `drifted`. List findings by severity with exact file references and the promise that is currently unmet.

## Quick Reference

| Layer | Core question | Typical false positive |
| --- | --- | --- |
| `module` | Does the capability exist locally? | "The evaluator has the logic." |
| `integration` | Can the real runtime reach it? | "We'll wire the registry later." |
| `delivery` | Can the user-visible entrypoint expose it now? | "The command returns `ok`, output can come later." |

## Delivery Checklist

- Formal entrypoint emits the promised result shape, not a placeholder status.
- Registered suites match phase scope and reject unsupported suites.
- Each promised suite is independently runnable from the real entrypoint.
- Default sample paths and shared assets exist in the checked-in tree.
- Fixed evaluator config is not only defined, but actually passed at callsites.
- Output preserves the artifacts needed for regression triage.

## Rationalization Table

| Excuse | Audit response |
| --- | --- |
| "The function exists, wiring is follow-up." | Not done until the real runtime can reach it. |
| "The CLI returns `ok`; result output can come later." | Delivery includes emitted contract, not just successful exit. |
| "Tests inject the missing dependency, so the feature exists." | Injected tests do not prove shipped entrypoint wiring. |
| "The suite works if you hand-pick files or services." | Phase delivery must work from default assets and promised entrypoints. |
| "The checkbox can stay checked because most code is there." | Tasks track delivered behavior, not implementation percentage. |

## Red Flags

- "Unit tests pass, so the task is done."
- "The capability already exists in the evaluator; entrypoint wiring is follow-up."
- "We can return `ok` for now and add the result envelope later."
- "The suite only works with injected deps or manual sample paths, but that is good enough."
- "A config or contract type exists, so it counts as delivered even if callsites never use it."
- "This is only a CLI or registration gap."
- "The task can stay checked because the function exists."

## Output Template

- `Promise surface`: What the spec actually promises in this phase.
- `Module status`: Present or missing.
- `Integration status`: Reachable or blocked by wiring.
- `Delivery status`: Exposed or not exposed through the formal entrypoint.
- `Default-path status`: Runnable or blocked by missing checked-in assets/config/deps.
- `Task corrections`: Which checkboxes should be unchecked or left open.
- `Blocking gaps`: What must change before claiming delivery.

## Common Mistakes

- Starting from evaluator internals instead of the shipped entrypoint.
- Auditing internal code paths instead of the formal delivery path.
- Treating injected test dependencies as proof of production wiring.
- Forgetting to verify checked-in default inputs and shared assets.
- Missing "independently runnable" promises because all suites were reasoned about together.
- Letting `tasks.md` track implementation progress instead of deliverable reality.
- Treating runtime wiring, shared runner registration, or output emission as "later" when the phase spec includes them.
- Counting optional diagnostic helpers as proof that the main contract is complete.