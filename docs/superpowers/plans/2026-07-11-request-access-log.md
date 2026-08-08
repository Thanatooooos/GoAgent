# Request Access Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Emit one safe structured completion log per HTTP request and correlate it with authenticated-user, chat, and ingestion workflow logs.

**Architecture:** Retain Zap and the existing context logger. A new global Gin middleware will run after request-context binding but before error and user middleware, so it wraps route execution and observes final HTTP outcomes through the request-context logger. Existing chat context enrichment remains canonical; ingestion workflow logs become context-aware.

**Tech Stack:** Go, Gin, Zap SugaredLogger, `zaptest/observer`.

---

### Task 1: Add and test the request-completion middleware

**Files:**
- Create: `internal/middleware/access_log.go`
- Modify: `internal/middleware/middleware_test.go`

- [ ] Write failing tests using `zaptest/observer`. Bind `zap.New(core).Sugar()` to an `httptest` request with `fwlog.BindLogger`, route `GET /items/:id`, then assert exactly one access entry contains `request_id`, `method`, route-template `path`, `status_code`, non-negative `latency_ms`, `response_size`, and `client_ip`. Send `/items/42?token=secret` and assert the message/fields do not contain `secret`, `authorization`, `cookie`, or a query string.

- [ ] Add two failing cases: a handler returning 401 must emit Warn; a handler returning 500 must emit Error. Add an unmatched path case that expects only `c.Request.URL.Path`.

- [ ] Run `go test ./internal/middleware -run TestAccessLogMiddleware -count=1` and verify compilation fails because `AccessLogMiddleware` does not exist.

- [ ] Create `AccessLogMiddleware`:

```go
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		path := strings.TrimSpace(c.FullPath())
		if path == "" { path = c.Request.URL.Path }
		fields := []any{
			"request_id", RequestID(c), "method", c.Request.Method,
			"path", path, "status_code", c.Writer.Status(),
			"latency_ms", time.Since(startedAt).Milliseconds(),
			"response_size", c.Writer.Size(), "client_ip", c.ClientIP(),
		}
		emitAccessLog(c.Request.Context(), c.Writer.Status(), fields...)
	}
}
```

`emitAccessLog` must use `log.FromContext(ctx).Infow` for `<400`, `Warnw` for `400..499`, and `Errorw` for `>=500`. It must not inspect headers, cookies, request/response bodies, or `RequestURI`.

- [ ] Run `go test ./internal/middleware -count=1`; expect PASS.

### Task 2: Add authenticated-user fields to request logs

**Files:**
- Modify: `internal/middleware/user_context_middleware.go`
- Modify: `internal/middleware/middleware_test.go`

- [ ] Write `TestAccessLogMiddlewareIncludesAuthenticatedUser`: use a `UserContextMiddleware` loader returning `LoginUser{UserID:"u-1", Username:"alice", Role:"admin"}` and assert the observed entry contains `user_id`, `username`, and `role`. Add an anonymous case asserting those keys are absent.

- [ ] Run `go test ./internal/middleware -run TestAccessLogMiddlewareIncludesAuthenticatedUser -count=1`; expect FAIL because context logger lacks user fields.

- [ ] Immediately after `contextx.Set(c, user)` in `UserContextMiddleware`, add:

```go
ctx := fwlog.NewContext(c.Request.Context(),
	"user_id", user.UserID,
	"username", user.Username,
	"role", user.Role,
)
c.Request = c.Request.WithContext(ctx)
```

- [ ] Run `go test ./internal/middleware -count=1`; expect PASS.

### Task 3: Wire the middleware into the production HTTP chain

**Files:**
- Modify: `cmd/server/main.go:134-143`
- Test: `cmd/server/main_test.go` or a focused router-wiring test in `cmd/server`

- [ ] Write a failing router-wiring test that sends an anonymous request to a route with `RequireLogin()` and asserts one Warn access entry with `status_code=401`.

- [ ] Run `go test ./cmd/server -run TestServerAccessLogRecordsAuthorizationRejection -count=1`; expect FAIL because the production chain has no access middleware.

- [ ] Register it before error handling so it observes the final response:

```go
r.Use(umw.RequestIDMiddleware())
r.Use(umw.LogContextMiddleware())
r.Use(umw.AccessLogMiddleware())
r.Use(umw.ErrorHandlerMiddleware())
r.Use(umw.UserContextMiddleware(userRuntime.LoadLoginUser, loginIDExtractor))
```

- [ ] Run `go test ./cmd/server ./internal/middleware -count=1`; expect PASS.

### Task 4: Make chat and ingestion logs use correlation context

**Files:**
- Modify: `internal/app/rag/service/chat/observability_logging.go`
- Modify: `internal/app/rag/service/chat/agent_stage.go`
- Modify: `internal/app/ingestion/service/executor/executor_workflow.go`
- Test: `internal/app/rag/service/chat/observability_logging_test.go`
- Test: `internal/app/ingestion/service/executor/executor_workflow_test.go`

- [ ] Write a chat test that binds an observed Zap logger, calls `enrichRagChatLogContext(ctx, "trace-1", "conv-1", "u-1", "task-1")`, invokes `logRagChatStart`, and asserts all four fields on its entry.

- [ ] Write an ingestion test that binds an observed logger, invokes `ExecutorService.Execute` for a successful one-node pipeline, and asserts task start/completion logs contain `task_id=task-1` and `pipeline_id=p-1`.

- [ ] Run the focused tests. The ingestion test must fail before implementation because executor workflow logging uses global `log.Infow`/`Errorw`.

- [ ] At the top of `ExecutorService.runWorkflow`, add:

```go
ctx = log.NewContext(ctx, "task_id", task.ID, "pipeline_id", task.PipelineID)
```

Replace task/node log calls in `runWorkflow` and `executeWorkflowNode` with `log.FromContext(ctx).Infow`, `Warnw`, or `Errorw`, preserving existing messages and fields. Keep the existing chat helper and ensure request-scoped chat entrypoints call it before `logRagChatStart`.

- [ ] Run `go test ./internal/app/rag/service/chat ./internal/app/ingestion/service/executor -count=1`; expect PASS.

### Task 5: Format, document, and verify

**Files:**
- Modify: `docs/project_progress_context.md`

- [ ] Document the request-log contract: one completion event per request; fields emitted; credentials, bodies, and query strings excluded; chat/ingestion correlation fields when present.

- [ ] Run:

```powershell
gofmt -w internal/middleware/access_log.go internal/middleware/user_context_middleware.go internal/app/ingestion/service/executor/executor_workflow.go
go test ./internal/middleware ./cmd/server ./internal/app/rag/service/chat ./internal/app/ingestion/service/executor -count=1
git diff --check
```

Expected: scoped tests pass and whitespace validation has no errors.

- [ ] Run `go test ./... -count=1`; if existing standalone root scripts or user-owned temporary Go files prevent a full pass, report those failures separately without altering them.

- [ ] Commit each task only if its named files can be staged without unrelated user work. The current worktree is dirty, so do not stage broad directory globs or create a mixed commit.
