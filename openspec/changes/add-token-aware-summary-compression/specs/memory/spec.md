# Memory Specification Delta

## ADDED Requirements

### Requirement: Summary compression shall be triggered by token budget

The system SHALL decide whether to compress conversation summary state using an
effective token estimate rather than a fixed conversation-turn threshold.

#### Scenario: Trigger compression above history budget

- GIVEN a conversation has a latest summary and uncovered messages
- WHEN the effective tokens of the latest summary plus uncovered messages meet
  or exceed the configured history budget
- THEN the system SHALL schedule summary compression

#### Scenario: Skip compression below history budget

- WHEN the effective summary and uncovered-history tokens remain below the
  configured history budget
- THEN the system SHALL NOT generate a new summary

#### Scenario: Ignore legacy turn threshold

- WHEN the conversation has any number of turns
- THEN `summary-start-turns` SHALL NOT be a hard precondition for token-triggered
  compression

### Requirement: Summary compression shall use incremental coverage boundaries

The system SHALL use the latest valid summary coverage boundary to identify
uncovered conversation messages.

#### Scenario: Load uncovered messages after latest summary

- GIVEN the latest summary covers messages through `CoveredToMessageID`
- WHEN the system evaluates or executes summary compression
- THEN it SHALL only treat later user and assistant messages as uncovered input

#### Scenario: Load all messages without an existing summary

- GIVEN no valid summary exists
- WHEN the system evaluates summary compression
- THEN it SHALL treat all eligible user and assistant messages as uncovered

#### Scenario: Keep trigger and compression inputs aligned

- WHEN token-triggered compression runs
- THEN the messages used for token trigger calculation and the messages supplied
  as fresh summary input SHALL use the same coverage boundary

### Requirement: Summary compression shall run asynchronously after assistant persistence

The system SHALL schedule summary evaluation after the assistant response has
been successfully persisted.

#### Scenario: Schedule after successful response

- WHEN the assistant response is persisted successfully
- THEN the system SHALL enqueue a summary compression check
- AND the generated summary SHALL be available to a later request

#### Scenario: Do not block chat completion

- WHEN summary scheduling or execution is slow
- THEN the system SHALL complete the current chat response without waiting for
  summary generation

#### Scenario: Fail open on summary errors

- WHEN summary scheduling, generation, validation, or persistence fails
- THEN the system SHALL keep the completed chat response successful
- AND it SHALL record the failure through existing observability hooks

### Requirement: Token estimation shall use one shared contract

The system SHALL use one shared token estimation contract for summary trigger
decisions and final chat prompt budgeting.

#### Scenario: Estimate identical text consistently

- WHEN summary and chat budgeting estimate the same text with the same
  configuration
- THEN they SHALL produce the same base token estimate

#### Scenario: Apply message overhead

- WHEN chat messages are included in token accounting
- THEN the system SHALL include configured per-message structural overhead

#### Scenario: Apply safety factor once

- WHEN effective history tokens are calculated
- THEN the configured safety factor SHALL be applied once at the history budget
  decision boundary

### Requirement: History budget shall reserve capacity for later prompt stages

The system SHALL calculate automatic history budget by reserving configured
capacity for fixed prompt content, long-term memory, session recall, retrieval,
tools, and safety overhead.

#### Scenario: Derive automatic history budget

- GIVEN `summary-trigger-tokens` is not explicitly configured
- WHEN the runtime initializes summary compression
- THEN it SHALL derive history budget from `max-prompt-tokens` minus configured
  stage reserves

#### Scenario: Use explicit history threshold

- GIVEN `summary-trigger-tokens` is greater than zero
- WHEN the runtime initializes summary compression
- THEN the explicit value SHALL override the automatically derived history
  budget

#### Scenario: Reject invalid budget

- WHEN configured reserves leave no valid history budget
- THEN the system SHALL use a documented safe fallback or reject the invalid
  configuration
- AND it SHALL NOT silently use a negative or zero effective budget

### Requirement: Retrieve context shall respect a token budget

The system SHALL limit retrieval context by token budget before final prompt
injection.

#### Scenario: Retain highest-ranked chunks first

- WHEN retrieval produces more context than the retrieve token budget permits
- THEN the system SHALL retain higher-ranked chunks before lower-ranked chunks

#### Scenario: Truncate an oversized chunk

- WHEN one chunk exceeds the remaining retrieve token budget
- THEN the system SHALL truncate that chunk by token budget
- AND it SHALL preserve minimum source-identification information

#### Scenario: Report retrieve truncation

- WHEN retrieval context is truncated
- THEN trace output SHALL include token counts before and after truncation and
  the number of retained chunks

### Requirement: Tool context shall respect a token budget

The system SHALL limit rendered tool context by token budget while retaining a
character hard cap for abnormal output protection.

#### Scenario: Retain critical tool sections

- WHEN tool context exceeds its token budget
- THEN the system SHALL prioritize conclusions, critical evidence, and source
  references over lower-priority detail

#### Scenario: Apply abnormal-output hard cap

- WHEN tool output is unexpectedly large or token estimation fails
- THEN the system SHALL still enforce a character hard cap

#### Scenario: Report tool truncation

- WHEN tool context is truncated
- THEN trace output SHALL include token counts before and after truncation and
  retained or dropped section information

### Requirement: Final prompt budgeting shall use actual stage outputs

The system SHALL recalculate prompt tokens after memory, session recall,
retrieval, and tool stages have produced their actual outputs.

#### Scenario: Reuse unused stage capacity

- GIVEN one stage uses less than its reserved capacity
- WHEN the complete prompt remains within `max-prompt-tokens`
- THEN another stage MAY use the remaining capacity

#### Scenario: Degrade an oversized prompt

- WHEN the actual complete prompt exceeds `max-prompt-tokens`
- THEN the system SHALL apply deterministic degradation steps
- AND it SHALL preserve the current question, required system prompt, and latest
  valid summary

#### Scenario: Emit final token breakdown

- WHEN final prompt budgeting completes
- THEN trace output SHALL expose total prompt tokens and per-stage token usage

### Requirement: Summary scheduling shall be idempotent by coverage boundary

The system SHALL prevent duplicate or stale summary jobs from creating multiple
effective summaries for the same coverage boundary.

#### Scenario: Skip already covered target

- GIVEN a queued job targets a message boundary
- AND a newer summary already covers that boundary
- WHEN the job executes
- THEN the job SHALL skip summary generation or persistence

#### Scenario: Deduplicate concurrent checks

- WHEN multiple summary checks are scheduled concurrently for one conversation
- THEN the system SHALL ensure coverage does not regress
- AND at most one effective summary SHALL be accepted for an equivalent target
  boundary
