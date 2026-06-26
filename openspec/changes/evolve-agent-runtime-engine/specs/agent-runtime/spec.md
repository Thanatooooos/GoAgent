# agent-runtime Specification Delta

## ADDED Requirements

### Requirement: Shared Agent Runtime Engine

The system SHALL execute agent patterns through a shared Agent Runtime Engine rather than through separate pattern-specific engines.

#### Scenario: Running plan_execute through the shared runtime

- **GIVEN** an agent request is routed to `plan_execute`
- **WHEN** the runtime starts the request
- **THEN** the request SHALL be represented as a `RuntimeSession`
- **AND** execution SHALL go through the shared runtime/kernel lifecycle
- **AND** state changes SHALL be applied through runtime state deltas and reducer
- **AND** runtime events SHALL be appended to the session journal

#### Scenario: Running reactive through the shared runtime

- **GIVEN** an agent request is routed to `reactive`
- **WHEN** the runtime starts the request
- **THEN** the request SHALL use the same session, checkpoint, approval, event, and capability scheduling infrastructure as `plan_execute`

### Requirement: Runtime Component Boundaries

The system SHALL define clear ownership boundaries between Agent Service, Runtime Engine Facade, Pattern, Kernel Runner, Capability Scheduler, State Reducer, and Event Journal.

#### Scenario: Agent service delegates execution

- **GIVEN** an external request enters the agent service
- **WHEN** the service routes the request to agent runtime
- **THEN** the service SHALL create or load a runtime session
- **AND** it SHALL delegate execution to the runtime engine facade
- **AND** it SHALL NOT depend on pattern-internal graph details to produce the external response

#### Scenario: Runtime engine facade owns lifecycle mechanics

- **GIVEN** a runtime session is being executed or resumed
- **WHEN** the runtime engine facade handles the session
- **THEN** it SHALL own run, resume, interrupt, checkpoint, approval outcome, and final runtime outcome normalization
- **AND** it SHALL use the selected pattern runner for strategy-specific execution

#### Scenario: Kernel runner remains graph execution layer

- **GIVEN** a compiled runtime graph exists for a pattern
- **WHEN** the runtime engine invokes the graph
- **THEN** the kernel runner SHALL execute graph nodes and checkpoint mechanics
- **AND** it SHALL NOT own pattern selection, capability policy, external response mapping, or frontend event projection

#### Scenario: State and event layers remain shared

- **GIVEN** any runtime component changes state or records progress
- **WHEN** that component emits a delta or event
- **THEN** the shared reducer and event journal SHALL be used
- **AND** the component SHALL NOT create a private state/event protocol for externally visible runtime behavior

### Requirement: Multiple Agent Patterns

The system SHALL support multiple agent execution patterns, including `reactive` and `plan_execute`.

#### Scenario: Reactive remains exploration pattern

- **GIVEN** a request is exploratory, diagnostic, search-heavy, or short-chain
- **WHEN** it is routed to `reactive`
- **THEN** the pattern MAY decide actions incrementally from observations
- **AND** it SHALL still use shared runtime session, scheduler, approval, checkpoint, and event infrastructure

#### Scenario: Plan-execute remains multi-step pattern

- **GIVEN** a request requires explicit steps, progress, approval, resume, or mixed capability execution
- **WHEN** it is routed to `plan_execute`
- **THEN** the pattern MAY maintain and update a plan
- **AND** it SHALL still use shared runtime session, scheduler, approval, checkpoint, and event infrastructure

#### Scenario: Pattern decides strategy

- **GIVEN** a runtime session is active
- **WHEN** the selected pattern evaluates the next step
- **THEN** the pattern MAY decide whether to act, observe, replan, continue, degrade, or stop
- **BUT** the pattern SHALL NOT bypass runtime-level approval, checkpoint, event, or capability scheduling

#### Scenario: Adding a future pattern

- **GIVEN** a new agent pattern is introduced
- **WHEN** it is registered with the agent runtime
- **THEN** it SHALL reuse the shared runtime session, checkpoint, approval, event, and scheduler infrastructure
- **AND** it SHALL NOT introduce an independent session or approval lifecycle

### Requirement: Runtime State Model Boundaries

The system SHALL maintain a shared runtime state model with explicit domain ownership, reducer invariants, and replay-compatible schema evolution.

#### Scenario: Applying state changes

- **GIVEN** a runtime node needs to update agent state
- **WHEN** it completes its work
- **THEN** it SHALL express changes as a state delta
- **AND** the runtime reducer SHALL apply the delta to the current snapshot
- **AND** the node SHALL NOT mutate the runtime snapshot directly

#### Scenario: State domain ownership

- **GIVEN** a state field belongs to request, context, plan, evidence, approval, execution, or answer state
- **WHEN** a runtime component updates that field
- **THEN** the field SHALL have a documented owner
- **AND** updates SHALL preserve the invariants of that state domain

#### Scenario: Pattern-specific state

- **GIVEN** a pattern needs state that is not shared by all patterns
- **WHEN** the pattern records that state
- **THEN** it SHALL use an explicit pattern-specific extension area or documented domain
- **AND** external projections SHALL NOT depend on pattern-private fields

#### Scenario: Replay projection from state and events

- **GIVEN** a runtime session has a snapshot and event journal
- **WHEN** replay or pending approval projection is requested
- **THEN** the projection SHALL reconstruct key state from shared runtime state and events
- **AND** it SHALL NOT require reading pattern-private implementation details

#### Scenario: Snapshot schema evolves

- **GIVEN** persisted checkpoints or sessions contain an older snapshot shape
- **WHEN** the runtime reads them after a schema change
- **THEN** the runtime SHALL either migrate them or handle them through an explicit compatibility path
- **AND** it SHALL avoid silently treating incompatible state as valid

### Requirement: Runtime-Level Approval Decision

The system SHALL model approval as a runtime-level decision and state transition.

#### Scenario: Capability requires approval

- **GIVEN** a pattern requests a capability whose spec requires approval
- **WHEN** the scheduler evaluates the request
- **THEN** the runtime SHALL emit a wait-approval decision
- **AND** the runtime SHALL persist approval state
- **AND** the runtime SHALL create or update the checkpoint reference needed for resume
- **AND** the pending approval lookup SHALL be able to restore the pending approval by conversation/user context

#### Scenario: Approval is resumed

- **GIVEN** a pending approval has been approved
- **WHEN** the runtime resumes execution
- **THEN** execution SHALL continue from the recorded checkpoint
- **AND** the runtime journal SHALL include a resume-completed event
- **AND** the runtime state SHALL record the approval decision metadata

#### Scenario: Approval is rejected

- **GIVEN** a pending approval has been rejected
- **WHEN** the runtime processes the rejection
- **THEN** the runtime SHALL produce a stable rejected/degraded/final outcome
- **AND** the pattern SHALL NOT execute the rejected capability call

### Requirement: Capability Scheduling

The system SHALL schedule capability execution through a shared scheduler contract.

#### Scenario: Scheduling by capability policy

- **GIVEN** a pattern requests one or more capability calls
- **WHEN** the scheduler evaluates the calls
- **THEN** it SHALL consider `RequiresApproval`, `SupportsParallel`, `SupportsResume`, `RiskLevel`, `Idempotency`, and `Preconditions`
- **AND** it SHALL produce a normalized execution decision

#### Scenario: Parallel-safe capability calls

- **GIVEN** multiple requested capability calls are marked parallel-safe
- **WHEN** the scheduler executes them
- **THEN** the calls MAY execute concurrently
- **AND** their results SHALL still be represented in a deterministic runtime event order

#### Scenario: Capability cannot resume

- **GIVEN** a capability does not support resume
- **WHEN** a runtime is resumed after interruption
- **THEN** the scheduler SHALL prevent unsafe re-execution or require an explicit retry/degrade decision

### Requirement: Stable Runtime Event Journal

The system SHALL maintain a stable append-only runtime event journal for agent execution.

#### Scenario: Runtime emits standard event families

- **GIVEN** an agent run progresses through nodes, decisions, capability calls, approval, resume, answer generation, or degradation
- **WHEN** runtime-visible progress occurs
- **THEN** the runtime SHALL emit standardized events for node lifecycle, decision, branch, capability, approval, checkpoint/interruption, resume, state-applied, answer, degradation, and failure
- **AND** event consumers SHALL NOT need pattern-private event types for these standard behaviors

#### Scenario: Event sequence is replayable

- **GIVEN** a runtime session has emitted events
- **WHEN** replay/projection reads the session journal
- **THEN** it SHALL be able to reconstruct key runtime state, including node progress, decisions, approval status, checkpoint metadata, capability results, and final outcome

#### Scenario: Runtime events feed external views

- **GIVEN** the frontend, trace view, or pending approval restore needs runtime progress
- **WHEN** runtime events are projected
- **THEN** they SHALL be consumable without reading pattern-private state

### Requirement: Pattern Routing and Gradual Adoption

The system SHALL support gradual adoption of agent runtime without forcing all chat traffic through it during the first convergence phase.

#### Scenario: Default agent pattern remains plan_execute

- **GIVEN** an agent request does not explicitly choose a pattern
- **WHEN** the agent service selects a pattern
- **THEN** it SHALL use `plan_execute` as the default pattern unless configuration states otherwise

#### Scenario: Routing to agent runtime for complex tasks

- **GIVEN** a request requires multi-step execution, approval, resume, or mixed capabilities
- **WHEN** the system routes the request
- **THEN** it MAY route the request to agent runtime
- **AND** the runtime SHALL select or receive an appropriate pattern

#### Scenario: Simple RAG traffic stays compatible

- **GIVEN** a request can be answered through the existing stable RAG chat path
- **WHEN** this change is introduced
- **THEN** the request SHALL NOT be forced into agent runtime solely because the runtime engine exists

### Requirement: Compatibility With Existing Chat and Tool Paths

The system SHALL preserve existing RAG chat and `rag/tool` behavior during the first runtime-engine convergence phase.

#### Scenario: Ordinary RAG chat

- **GIVEN** a request is handled by the existing RAG chat path
- **WHEN** this change is introduced
- **THEN** the request SHALL NOT be forced through agent runtime unless explicitly routed there

#### Scenario: Existing rag/tool production path

- **GIVEN** existing production behavior depends on `internal/app/rag/tool`
- **WHEN** agent runtime contracts are introduced
- **THEN** `rag/tool` SHALL remain available until a separate migration change explicitly replaces it
