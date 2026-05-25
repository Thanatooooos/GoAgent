# Skill: Architecture & File Boundary Discipline

## Objective

Maintain clear architectural boundaries and prevent code from accumulating into large mixed-responsibility files or folders.

Code structure is part of the implementation, not a future cleanup task.

Never solve a feature request by dumping everything into a single package or file and proposing a refactor afterward.

---

## Mandatory Planning

Before making changes, always determine:

1. What responsibility is being added?
2. Which layer owns that responsibility?
3. Does an existing package already own it?
4. Should code be added to an existing package or a new package?

Provide a short structure plan before implementation.

Example:

Structure plan:

- handler:
  - internal/api/knowledge_handler.go

- service:
  - internal/service/retrieval_service.go

- repository:
  - internal/repository/chunk_repository.go

- tests:
  - internal/service/retrieval_service_test.go

Reason:
Retrieval orchestration belongs to service layer.
Persistence belongs to repository layer.
HTTP concerns remain in handler layer.

---

## Layer Ownership

Always keep responsibilities separated.

### Handler / Controller

Owns:

- HTTP
- REST
- SSE
- request parsing
- response formatting
- validation

Must NOT own:

- business logic
- database logic
- workflow orchestration

---

### Service / UseCase

Owns:

- business workflows
- orchestration
- domain decisions
- application logic

Must NOT own:

- SQL
- HTTP details
- infrastructure implementation

---

### Repository

Owns:

- database access
- query logic
- persistence

Must NOT own:

- business decisions
- HTTP logic

---

### Domain

Owns:

- entities
- value objects
- domain rules
- interfaces

Must remain infrastructure-independent.

---

### Adapter

Owns:

- LLM integration
- embedding integration
- vector store integration
- external APIs
- MQ
- storage providers

Must not contain application workflow logic.

---

## File Size Constraints

Target:

- <300 lines = preferred
- 300-500 lines = acceptable
- >500 lines = requires review
- >800 lines = split immediately

Never intentionally grow an already overloaded file.

---

## Single Responsibility Rule

A file should have one primary reason to change.

Bad:

- handler + service
- service + repository
- API + SQL
- DTO + workflow + persistence

Good:

- one responsibility per file
- one package owns one concern

---

## Package Rules

Prefer domain-oriented package names.

Good:

- knowledge
- retrieval
- ingestion
- memory
- workflow
- scheduler
- diagnosis
- document

Bad:

- common
- util
- helper
- misc
- manager
- logic
- tools

Do not create generic packages unless absolutely necessary.

---

## Utility Rule

Never create a helper package simply to avoid thinking about ownership.

Before creating utility code ask:

Which domain actually owns this behavior?

If ownership exists:

Place it there.

Only create shared utilities when:

- logic is pure
- reused by multiple bounded contexts
- has no domain ownership

---

## Refactoring Policy

Refactor before adding more code.

Do NOT:

1. add feature
2. create large mixed file
3. claim "should be refactored later"

Instead:

1. establish proper boundaries
2. extract responsibilities
3. implement feature
4. leave structure cleaner than before

---

## New File Policy

Creating a new file is preferred over growing a large unrelated file.

Do not avoid new files merely to reduce file count.

Optimize for maintainability, not minimum file number.

---

## Existing Architecture Rule

Follow existing architecture when it is reasonable.

Do NOT:

- flatten package hierarchy
- move everything into one folder
- bypass established boundaries

When architecture already exists:

conform to it.

---

## Go Project Rules

Prefer:

internal/
├── api/
├── service/
├── repository/
├── domain/
├── adapter/
├── workflow/
├── scheduler/

Keep package dependencies directional:

handler
→ service
→ domain/repository

adapter
→ external systems

Avoid circular dependencies.

---

## Required Self-Review

Before completing any task verify:

- responsibilities are separated
- no god file created
- no mixed-layer implementation
- no unnecessary utility package introduced
- package ownership is clear
- tests are located near affected code
- architecture is cleaner than before

If any answer is "No", fix it before considering the task complete.

---

## Hard Constraint

Never say:

"This should be refactored later."

"This can be cleaned up in a future task."

"We can split this afterwards."

If the code requires refactoring to be maintainable, perform the refactor as part of the current implementation.

Structural debt must not be intentionally introduced.