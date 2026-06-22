\# Evaluation Specification Delta

## ADDED Requirements

### Requirement: The system shall provide a shared offline evaluation framework

The system SHALL provide a shared offline evaluation framework for model-facing
stages, with a common runner contract and a common result contract.

#### Scenario: Run one evaluation suite

- WHEN a caller requests a supported evaluation suite
- THEN the system SHALL execute that suite through the shared runner

#### Scenario: Run all registered suites

- WHEN a caller requests all suites
- THEN the system SHALL execute all registered suites through the shared runner

#### Scenario: Emit shared result shape

- WHEN any evaluation suite completes
- THEN the system SHALL emit results using a shared top-level contract
- AND the contract SHALL include `samples`, `aggregate`, and `artifacts`

### Requirement: Phase-1 offline evaluation shall be limited to summary and rewrite suites

Phase 1 SHALL register only the following suites in the shared offline
evaluation framework:

- `summary`
- `rewrite`

#### Scenario: Execute supported Phase-1 suite

- WHEN the caller requests `summary` or `rewrite`
- THEN the system SHALL execute the requested suite

#### Scenario: Exclude unsupported future suite

- WHEN the caller requests a suite not implemented in Phase 1
- THEN the system SHALL reject the request as unsupported

### Requirement: Summary evaluation shall score structured summary output directly

The system SHALL evaluate structured summary output directly in Phase 1 rather
than scoring only the rendered compatibility text summary.

#### Scenario: Evaluate structured summary fields

- WHEN a summary sample is executed
- THEN the system SHALL judge the structured summary object against the
  structured sample expectation

#### Scenario: Do not require gold prose summary

- WHEN a summary sample is authored
- THEN the sample SHALL NOT require a single gold prose summary as the scoring
  source of truth

### Requirement: Summary evaluation shall enforce both fidelity and usefulness

The system SHALL evaluate summary quality using two parallel dimensions:

- fidelity
- usefulness

Both dimensions SHALL be required for the sample to pass.

#### Scenario: Fail unfaithful summary

- WHEN a structured summary omits critical retained state, preserves stale
  superseded state, or introduces forbidden hallucinated state
- THEN the system SHALL fail the sample on fidelity

#### Scenario: Fail unusable summary

- WHEN a structured summary is not sufficient to support the intended follow-up
  work or follow-up questions
- THEN the system SHALL fail the sample on usefulness

#### Scenario: Pass only when both dimensions pass

- WHEN a structured summary passes fidelity checks and usefulness checks
- THEN the system SHALL mark the sample as passed

### Requirement: Summary evaluation shall combine rule checks with field-level judge checks

The system SHALL evaluate summary samples with deterministic rule checks first,
then field-level and overall judge checks.

#### Scenario: Fail invalid structured output before judging

- WHEN the structured summary output fails schema validation, required-field
  validation, entity-retention validation, or forbidden-content validation
- THEN the system SHALL fail the sample without requiring success from judge
  scoring

#### Scenario: Judge summary fields individually

- WHEN the structured summary passes deterministic preconditions
- THEN the system SHALL evaluate at least the following fields individually:
  - `goal`
  - `constraints`
  - `established_facts`
  - `recent_progress`
  - `open_questions`

#### Scenario: Judge overall follow-up usefulness

- WHEN the structured summary passes field-level parsing
- THEN the system SHALL evaluate whether the summary is sufficient to support
  the sample's follow-up questions

### Requirement: Rewrite evaluation shall score both rewrite quality and retrieval impact

The system SHALL evaluate rewrite samples on both rewrite correctness and
retrieval impact.

#### Scenario: Validate rewrite output directly

- WHEN a rewrite sample is executed
- THEN the system SHALL validate rewrite expectations such as retained terms,
  split boundaries, and retrieval-decision behavior

#### Scenario: Compare retrieval before and after rewrite

- WHEN a rewrite sample includes retrieval expectations
- THEN the system SHALL compute retrieval metrics before rewrite and after
  rewrite
- AND the system SHALL report the delta

#### Scenario: Fail harmful rewrite

- WHEN a rewrite appears textually acceptable but causes a critical retrieval
  regression against sample expectations
- THEN the system SHALL fail the sample

### Requirement: Rewrite sample authoring shall embed retrieval expectations

The system SHALL support rewrite samples that embed retrieval expectations in
the same sample record.

#### Scenario: One sample drives both rewrite and retrieval scoring

- WHEN a rewrite sample is loaded
- THEN the system SHALL read both rewrite expectations and retrieval
  expectations from the same sample

#### Scenario: Support retrieval metrics from embedded expectation

- WHEN the rewrite evaluator executes a sample
- THEN the evaluator SHALL use the embedded retrieval expectation to score
  before/after retrieval quality

### Requirement: Phase-1 samples shall be manually designed and failure-mode-oriented

The system SHALL use manually designed, failure-mode-oriented sample sets in
Phase 1.

#### Scenario: Summary suite sample count target

- WHEN the Phase-1 summary suite is authored
- THEN it SHALL contain a small high-quality set of approximately `20-30`
  samples

#### Scenario: Rewrite suite sample count target

- WHEN the Phase-1 rewrite suite is authored
- THEN it SHALL contain a small high-quality set of approximately `40-60`
  samples

#### Scenario: Prefer failure-mode coverage over volume

- WHEN Phase-1 samples are selected
- THEN priority SHALL be given to representative failure modes rather than raw
  sample-count growth

### Requirement: Evaluation output shall support tag-level regression analysis

The system SHALL support aggregate evaluation reporting by tag.

#### Scenario: Emit tag aggregates

- WHEN a suite completes with tagged samples
- THEN the system SHALL report aggregate results grouped by tag

#### Scenario: Surface regression location

- WHEN a change causes quality regression in a subset of tagged samples
- THEN the system SHALL make that regression visible in suite output
