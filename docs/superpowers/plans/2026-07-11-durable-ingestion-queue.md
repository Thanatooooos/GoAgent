# Durable Ingestion Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace process-local ingestion task dispatch with a Redis-backed durable queue so tasks survive service restarts, retry on transient errors, and reach an observable terminal failure state.

**Architecture:** `TaskService` continues to persist task metadata in PostgreSQL, but submits the task ID to an Asynq queue. A worker loads the persisted task and pipeline and invokes the existing executor synchronously. Asynq owns delivery, retry and crash recovery; the existing observer continues to own `task` and `task_node` state transitions.

**Tech Stack:** Go, Asynq, Redis, PostgreSQL, GORM, existing ingestion executor.

---

### Task 1: Add queue contracts and a synchronous executor entrypoint

**Files:**
- Create: `internal/app/ingestion/service/queue/task_queue.go`
- Modify: `internal/app/ingestion/port/executor.go`
- Modify: `internal/app/ingestion/service/executor/executor_workflow.go`
- Test: `internal/app/ingestion/service/queue/task_queue_test.go`

- [ ] Write failing tests showing that a queue receives only a task ID and that `RunTask` loads a persisted task/pipeline.
- [ ] Add `TaskQueue.Enqueue(ctx, taskID)` and `TaskExecutor.RunTask(ctx, task, pipeline)` contracts.
- [ ] Extract executor's existing workflow build/run path into `RunTask`; retain `Submit` only as a compatibility wrapper until callers migrate.
- [ ] Run focused queue/executor tests.

### Task 2: Implement Asynq enqueue and worker dispatch

**Files:**
- Create: `internal/adapter/queue/asynq/ingestion_queue.go`
- Create: `internal/adapter/queue/asynq/ingestion_worker.go`
- Modify: `go.mod`
- Modify: `internal/bootstrap/ingestion/runtime.go`
- Test: `internal/adapter/queue/asynq/ingestion_queue_test.go`

- [ ] Write failing tests for deterministic task payloads, retry configuration, and worker dispatch by task ID.
- [ ] Add the Asynq dependency and an enqueue adapter that uses task ID as the queue payload and configures retry/timeout options.
- [ ] Add a worker handler that loads the task and pipeline from PostgreSQL, ignores already-terminal tasks, and returns errors to Asynq for retry.
- [ ] Start/stop the worker as part of ingestion runtime ownership.
- [ ] Run focused adapter and bootstrap tests.

### Task 3: Make TaskService enqueue durably and surface terminal queue failure

**Files:**
- Modify: `internal/app/ingestion/service/task/service_task.go`
- Modify: `internal/app/ingestion/service/observer/observer_task_repository.go`
- Modify: `internal/bootstrap/ingestion/runtime.go`
- Test: `internal/app/ingestion/service/task/service_task_test.go`
- Test: `internal/bootstrap/ingestion/runtime_test.go`

- [ ] Write failing tests that creation enqueues the persisted task ID and that enqueue failure leaves a visible failed task rather than an invisible pending task.
- [ ] Replace direct in-process submission with queue enqueue after task persistence.
- [ ] Wire the worker's final retry callback to mark the task failed and preserve the terminal error.
- [ ] Run focused task/runtime tests.

### Task 4: Verify restart-safe semantics

**Files:**
- Create: `internal/adapter/queue/asynq/ingestion_integration_test.go`
- Modify: `docker-compose.yml` only if existing Redis configuration cannot serve Asynq.

- [ ] Write an integration test that enqueues a task before the worker starts, then starts a worker and observes one successful execution.
- [ ] Write an integration test that makes the executor return an error and verifies retry exhaustion marks the task failed.
- [ ] Run ingestion package tests and `go test ./...` where the local environment permits.

### Task 5: Document the delivery contract

**Files:**
- Modify: `docs/project_progress_context.md`

- [ ] Document that ingestion delivery is at-least-once, node effects must be idempotent, and a retry replays the pipeline from the start.
- [ ] Record the queue/retry/dead-letter operational behaviour and verification commands.
