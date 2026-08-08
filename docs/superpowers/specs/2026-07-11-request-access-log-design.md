# Request Access Log Design

**Goal:** Add request-level structured access logs that can be correlated with existing context-aware business logs, without changing authentication, routing behavior, or introducing an external observability platform.

## Scope

This change adds a Gin middleware that records one structured completion event for every HTTP request. It also ensures the request logger contains authenticated-user fields when authentication succeeds.

The event fields are:

```text
request_id, method, path, status_code, latency_ms,
response_size, client_ip, user_id, username, role
```

The middleware must not log `Authorization`, Cookie values, request bodies, response bodies, or URL query strings.

## Request flow

```text
RequestIDMiddleware
  -> LogContextMiddleware
  -> AccessLogMiddleware
  -> ErrorHandlerMiddleware
  -> UserContextMiddleware
  -> handler
  -> AccessLogMiddleware emits completion event
```

`RequestIDMiddleware` remains responsible for accepting or generating `X-Request-Id`. `LogContextMiddleware` binds that ID to the request context logger. `AccessLogMiddleware` wraps the error and user middlewares so it observes final 4xx/5xx responses, including recovered panics. `UserContextMiddleware` resolves the optional login session before the route runs and enriches the request context that the access logger reads after `c.Next()` returns.

## Logging behavior

- Use `c.FullPath()` when non-empty; otherwise use `c.Request.URL.Path`. Never log `RequestURI`, which includes the query string.
- Use `Info` for status codes below 400, `Warn` for 4xx, and `Error` for 5xx.
- Treat a missing authenticated user as normal and omit user fields rather than logging empty placeholders.
- The log event is emitted once, including for requests aborted by authorization middleware. Middleware order makes this possible because `AccessLogMiddleware` wraps downstream handlers.
- Long-lived SSE requests produce one completion event when the connection closes. No per-event log entries are introduced.

## Context correlation

The existing framework logger already stores a child `*zap.SugaredLogger` in `context.Context`. The access-log middleware will enrich the request context with `user_id`, `username`, and `role` after downstream authentication has run. New or touched request-scoped business code should use `log.FromContext(ctx)` instead of global `log.Infof` helpers.

The initial rollout will add correlation fields only at the RAG chat and ingestion entry points:

- chat execution adds `conversation_id` when known;
- ingestion execution adds `task_id` and `pipeline_id`.

This is additive: existing global log calls continue to work, but do not automatically inherit request fields.

## Validation

Unit tests cover request-ID propagation, anonymous and authenticated access-log fields, route-template selection, status-level selection, duration/response-size fields, and absence of sensitive header/query data. Existing middleware and server tests remain green.

## Explicitly out of scope

- Langfuse, OpenTelemetry, or another external trace backend.
- File rotation, log shipping, dashboards, and metrics aggregation.
- Security audit-event storage.
- Bulk conversion of every existing business log call.
- Changes to authentication/session semantics.
