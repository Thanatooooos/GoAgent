# SSE Stream Transport Design

Date: `2026-08-09`

## Background

The chat response path in goagent currently couples the SSE write to the chat
execution itself:

- `internal/adapter/http/rag/chat_handler.go` builds a `sseChatSink` and passes
  it into `chatService.Chat`, which runs **synchronously in the HTTP handler
  goroutine**; every sink method writes directly to the SSE emitter.
- There is no persistence of the emitted events, no way to replay a stream, and
  no way for a client to reconnect after a dropped connection.
- All streaming state (task registry, cancellation) is in-process. A second
  replica cannot serve a stream started by the first, and `POST /stop` only
  works when it lands on the node that owns the task.

WeKnora solves exactly this with an **append-only event stream** (memory or
Redis list) plus a **polling SSE consumer**:

- A `StreamManager` interface with `AppendEvent` / `GetEvents(fromOffset)`.
- The business layer writes typed events into the stream; an SSE loop polls the
  stream from a local offset and is the *only* writer to the client.
- Stop is itself written as an event into the shared stream, so any node can
  stop a task owned by any other node.
- A `continue` endpoint replays the stream from offset 0, giving disconnect
  resume.

This design ports that skeleton into goagent while freezing the SSE wire format
so the existing frontend works unchanged.

## Goals

- Decouple chat execution from SSE delivery: single producer (chat writes to a
  stream), single consumer (SSE poller), no direct SSE writes from the sink.
- Make the stream the source of truth: reconnect replays it; Redis mode makes it
  visible across instances.
- Multi-instance stop: `POST /stop` reaches the executing node even when it lands
  on another replica.
- Keep the SSE wire format byte-for-byte identical to today (`event: <name>\ndata:
  <json>\n\n`), so the frontend needs no changes.
- Add a `continue` endpoint as an additive capability; frontend adoption is
  optional and can follow later.

## Non-Goals

- No changes to the chat service's internal pipeline (prepare / tool / prompt /
  streaming stages). The `RagChatEventSink` interface and its 18 methods are the
  contract and stay as-is.
- No rework of the frontend event protocol.
- No new rate limiting, auth, or multi-tenancy work.
- No change to the agent runtime, tool workflow, citation, or memory subsystems.
- Live-stream event loss is out of scope for this iteration: the stream is
  append-only and bounded by TTL; a live client is expected to consume in
  near-real-time.

## Architecture

```
                         ┌──────────────────────────────────────────────┐
                         │               StreamManager                   │
  chat goroutine  ─────▶ │   memory: map[streamID][]StreamEvent          │
  (streamChatSink)       │   redis:  LIST stream:{taskID} (RPush/LRange) │
                         └──────────────────────────────────────────────┘
                              ▲                            │
                              │ GetEvents(fromOffset)      │ GetEvents(fromOffset)
   SSE poller (live) ─────────┘                            └───────────────▶ SSE poller (continue)
   SseEmitterSender                                        SseEmitterSender (fresh connection)
```

- `streamID == taskID`. The taskID is pre-generated in the handler and passed
  into `RagChatInput`, so the stream key is known before the chat starts and the
  `meta` event the client receives carries the same value (used later to
  reconnect).
- The sink writes events; the poller is the only thing that writes SSE. There is
  no path where the sink touches the HTTP writer.

## 1. Stream package: `internal/framework/stream/`

### StreamEvent

```go
type StreamEvent struct {
    ID        string          `json:"id"`
    Name      string          `json:"name"`            // SSE event name, e.g. "meta", "message", "tool"
    Data      json.RawMessage `json:"data,omitempty"`  // raw JSON payload, replayed verbatim
    Done      bool            `json:"done"`            // terminal sentinel (done / cancel / error)
    Timestamp time.Time       `json:"timestamp"`
}
```

`Data` holds the exact JSON bytes that will be written after `event: <name>\n
data: `. Storing the final payload guarantees replay is byte-identical to live.

### StreamManager interface

```go
type StreamManager interface {
    AppendEvent(ctx context.Context, streamID string, e StreamEvent) error
    GetEvents(ctx context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error)
}
```

Offset semantics: append-only; `GetEvents` returns `events[fromOffset:]` plus the
next offset (`fromOffset + len(events)`). There is no separate "is done" method;
termination is signalled by a `Done == true` event inside the stream.

### memory_stream_manager.go

- `map[string]*memoryStream` where `memoryStream` holds `events []StreamEvent`,
  `mu sync.RWMutex`, `lastUpdated time.Time`.
- Append is append-only; GetEvents copies the slice tail.
- A background sweeper removes streams older than the configured TTL (default
  `1h`), so single-instance streams do not grow unbounded.
- Same-process only; not visible across instances.

### redis_stream_manager.go

- Key: `{prefix}:{streamID}` with prefix default `stream:events` (reuses the
  existing `redis-key-prefix: goagent` config).
- `AppendEvent` = `RPush` of `json.Marshal(event)` + `Expire(key, ttl)` refresh.
- `GetEvents` = `LRange(key, fromOffset, -1)` + decode; next offset =
  `fromOffset + len(events)`.
- TTL configurable, default `1h`; list elements are never mutated, so index
  offsets stay valid across readers.
- Reuses the existing `github.com/redis/go-redis/v9` client and `redis` config
  section already used by the memory cache.

### Config

```yaml
rag:
  stream:
    type: memory   # memory | redis
    ttl: 1h
```

Default `memory` keeps single-instance behavior equivalent to today; `redis`
enables multi-instance stream visibility and cross-node stop.

## 2. Handler rework: `internal/adapter/http/rag/`

### TaskID pre-generation

- Add `TaskID string` to `RagChatInput` (`internal/app/rag/service/chat/types.go`).
- In `prepare` runtime stage (`prepare_orchestrator.go` `runRuntimeStage`), use
  `input.TaskID` when non-empty, else generate as today. The `meta` event and the
  trace already carry this ID.

### streamChatSink

- New sink in `chat_handler.go` (or a new `stream_chat_sink.go` in the same
  package) implementing the same `RagChatEventSink`.
- Each method builds the exact same payload JSON the current `sseChatSink`
  builds, but calls `manager.AppendEvent` with `{Name, Data, Done}` instead of
  writing SSE. Extract the per-event payload constructors into shared helpers so
  there is a single source of truth for wire bytes.
- `SendDone` / `SendCancel` / `SendError` set `Done: true`. `SendDone` remains
  the universal terminal event (the chat service already guarantees a deferred
  `SendDone`).
- The old `sseChatSink` is deleted; SSE now has exactly one writer (the poller).

### Chat handler flow

```go
func (h *Handler) Chat(c *gin.Context) {
    user := requireLoginUser(c)
    if user == nil { return }
    sender := fwweb.NewSseEmitterSender(c)
    taskID := newTaskID()                       // shared with chat service
    sink := &streamChatSink{manager: h.streamManager, streamID: taskID}
    go h.chatService.Chat(
        context.WithoutCancel(c.Request.Context()),   // finish persistence even if client disconnects
        ragservice.RagChatInput{..., TaskID: taskID}, sink)
    go stopWatcher(context.WithoutCancel(c.Request.Context()), h.streamManager, taskID, h.chatService)
    pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0)
}
```

`ResumeAfterApproval` uses the same transport (fresh taskID + sink + pollLoop).

### SSE poller

```go
func pollLoop(ctx, sender, manager, streamID, startOffset) {
    offset := 0
    ticker := time.NewTicker(100 * time.Millisecond)
    deadline := time.NewTimer(maxStreamDuration)      // default 5min backstop
    for {
        events, next, err := manager.GetEvents(ctx, streamID, offset)
        for _, e := range events {
            if e.Name == internalStopEventName { continue }   // control event, not forwarded
            if err := sender.SendEvent(e.Name, rawPayload(e.Data)); err != nil { return }
            if e.Done { sender.Complete(); return }
        }
        offset = next
        select {
        case <-ticker.C:
        case <-ctx.Done():
            return
        case <-deadline.C:
            sender.Complete(); return
        }
    }
}
```

- `stop` is a control event written into the stream but filtered from SSE
  forwarding (the frontend has no such event today).
- On client disconnect (`ctx.Done`) the poller returns; the chat goroutine keeps
  running via its `WithoutCancel` context and finishes persistence, appending the
  terminal events to the stream (later cleaned by TTL).
- The max-duration backstop prevents a leaked poller if the chat goroutine dies
  without a terminal event.

## 3. Stop: dual channel

### StopChat handler

```go
func (h *Handler) StopChat(c *gin.Context) {
    taskID := c.Query("taskId")
    // fast path: local task registry (single instance, zero latency)
    h.chatService.CancelTask(taskID)
    // cross-node path: write stop control event into the shared stream
    _ = h.streamManager.AppendEvent(ctx, taskID, StreamEvent{Name: internalStopEventName, Done: true})
    writeSuccess[any](c, nil)     // idempotent success (was: 404 when task not found locally)
}
```

Behavior change: stop becomes idempotent and always succeeds; the previous 404
"chat task not found" is dropped because a task may be owned by another node.

### Stop watcher (on the executing node)

- When the chat goroutine starts, also spawn a watcher goroutine that polls the
  stream for `taskID` every 200ms looking for the `stop` control event.
- On seeing it, call `chatService.CancelTask(taskID)` locally — the existing
  task-registry cancellation path takes over (LLM handle cancel → `cancel`
  event → stream → SSE poller forwards to the client).
- Watcher exits when it observes the task's terminal (`Done`) event or when its
  own context is cancelled.
- `cancelOnce` in the task registry makes the fast path and the watcher
  idempotent against each other.

Single instance: the fast path handles stop with zero latency; the watcher is
redundant but harmless. Redis mode: a stop landed on any node is written to the
shared stream and picked up by the executing node's watcher.

## 4. Reconnect: `GET /rag/v3/chat/continue`

```go
// route: GET /rag/v3/chat/continue?taskId=...
func (h *Handler) ContinueChat(c *gin.Context) {
    taskID := c.Query("taskId")              // required
    sender := fwweb.NewSseEmitterSender(c)
    // replay from offset 0, then keep polling until done/ctx-done/backstop
    pollLoop(c.Request.Context(), sender, h.streamManager, taskID, startOffset = 0)
}
```

- The client obtains `taskId` from the `meta` event of the original stream.
- Same `pollLoop` as the live path, so replay and live delivery are one code path.
- If the stream is already gone (TTL expired) or unknown, `GetEvents` returns
  empty and the poller completes with no events.
- Single instance + memory manager: reconnect must reach the same replica.
  Redis mode: reconnect can land on any replica.

## Wire format compatibility

The frontend consumes `event: <name>\ndata: <json>\n\n` today. Because
`streamChatSink` builds identical payload JSON and the poller replays `Data`
verbatim, every event name and payload shape is unchanged. No frontend change is
required for this iteration; the `continue` endpoint is an additive capability
for later adoption.

## Error handling

- `AppendEvent` failures (Redis down): the sink logs and continues (fail-open),
  matching the current sink's tolerance; the poller will simply not see events it
  was not able to store. In memory mode this cannot happen.
- `GetEvents` failures: the poller retries on the next tick instead of tearing
  down the SSE connection.
- Chat goroutine panic without terminal event: the poller's max-duration backstop
  closes the connection.

## Testing

- `stream` package unit tests:
  - memory manager append/get/offset semantics, TTL sweep.
  - redis manager round-trip (append → get), offset math, key/prefix/TTL behavior.
- Sink equivalence test: feed the same sink methods to the new `streamChatSink`
  and assert the produced `(Name, Data)` pairs match the old `sseChatSink` output
  byte-for-byte (golden payloads).
- Poller tests with a fake manager: forwards events in order, filters `stop`,
  terminates on `Done`, exits on `ctx.Done`, terminates on backstop timeout.
- Reconnect: replay-from-0 returns the full event sequence then continues.
- Stop: writing a stop event triggers the watcher to cancel the local task;
  `cancelOnce` guards double-cancel.
- Handler-level test (httptest + fake manager): `POST /chat` streams to the
  client via the poller; `POST /stop` writes the stop event.

## Rollout

- Ship behind `rag.stream.type` (default `memory`). Single instance behaves as
  today; no frontend change; existing chat tests keep passing.
- Flip a dev instance to `redis` to validate multi-instance visibility and
  cross-node stop before any real horizontal deployment.
