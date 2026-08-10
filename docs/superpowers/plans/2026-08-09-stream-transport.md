# SSE Stream Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decouple chat SSE delivery from chat execution via an append-only event stream (memory/Redis) plus a polling SSE consumer, adding disconnect resume (`continue`) and multi-instance stop.

**Architecture:** A new `internal/framework/stream` package provides `StreamManager` (memory + Redis list impls) storing `StreamEvent`s. The chat handler writes events into the stream via a new `streamChatSink`, runs the chat in a goroutine, and the handler's main goroutine polls the stream and writes SSE (single writer). `POST /stop` keeps the local task-registry fast path and additionally writes a `stop` control event; a per-task watcher on the executing node cancels on it (cross-node). A `continue` endpoint replays the stream from offset 0.

**Tech Stack:** Go 1.25, Gin, go-redis/v9 (already a dependency), SseEmitterSender (existing).

**Spec:** `docs/superpowers/specs/2026-08-09-stream-transport-design.md`

---

## File Structure

**Create:**
- `internal/framework/stream/stream.go` 鈥?`StreamEvent`, `StreamManager` interface, package doc.
- `internal/framework/stream/memory_stream_manager.go` 鈥?in-memory impl.
- `internal/framework/stream/memory_stream_manager_test.go`
- `internal/framework/stream/redis_stream_manager.go` 鈥?Redis list impl.
- `internal/framework/stream/redis_stream_manager_test.go` (uses miniredis or a fake; see Task 2 note)
- `internal/adapter/http/rag/stream_transport.go` 鈥?`pollLoop`, `stopWatcher`, transport constants, internal stop event name.
- `internal/adapter/http/rag/stream_transport_test.go`
- `internal/adapter/http/rag/stream_chat_sink.go` 鈥?`streamChatSink`.
- `internal/adapter/http/rag/stream_chat_sink_test.go` 鈥?wire-format parity.
- `internal/bootstrap/rag/runtime_build_stream.go` 鈥?`buildStreamManager`, sweep loop start/stop.

**Modify:**
- `internal/framework/config/config.go` 鈥?add `RagStreamConfig` + `RagConfig.Stream`.
- `configs/application.yaml` 鈥?add `rag.stream` block.
- `internal/app/rag/service/chat/types.go` 鈥?add `TaskID` to `RagChatInput` and `RagChatApprovalResumeInput`.
- `internal/app/rag/service/chat/stage_helpers.go` 鈥?add exported `NextTaskID()`.
- `internal/app/rag/service/chat/prepare_memory.go` 鈥?`runRuntimeStage` adopts `input.TaskID`.
- `internal/app/rag/service/chat/agent_stage.go` 鈥?`newAgentRuntimeState` adopts taskID.
- `internal/app/rag/service/aliases.go` 鈥?alias `NextTaskID`.
- `internal/adapter/http/rag/chat_handler.go` 鈥?rework `Chat`/`ResumeAfterApproval`/`StopChat`, add `ContinueChat`, delete `sseChatSink`.
- `internal/adapter/http/rag/handlers.go` 鈥?add `streamManager` to `Handler`/`NewHandler`.
- `internal/adapter/http/rag/routes.go` 鈥?add `streamManager` param + `continue` route.
- `internal/adapter/http/rag/test/chat_handler_test.go` 鈥?pass nil streamManager; add continue/stop tests.
- `internal/bootstrap/rag/runtime.go` 鈥?add `StreamManager` field, build + close it.
- `cmd/server/main.go:259` 鈥?pass `runtime.StreamManager` to `RegisterRoutes`.

---

### Task 1: Stream core 鈥?event + interface + memory manager

**Files:**
- Create: `internal/framework/stream/stream.go`
- Create: `internal/framework/stream/memory_stream_manager.go`
- Create: `internal/framework/stream/memory_stream_manager_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/framework/stream/memory_stream_manager_test.go`:

```go
package stream

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStreamManagerAppendAndGet(t *testing.T) {
	m := NewMemoryStreamManager()
	ctx := context.Background()
	ev := StreamEvent{Name: "meta", Data: []byte(`{"a":1}`), Done: false, Timestamp: time.Now()}
	if err := m.AppendEvent(ctx, "s1", ev); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, next, err := m.GetEvents(ctx, "s1", 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(events) != 1 || events[0].Name != "meta" || string(events[0].Data) != `{"a":1}` {
		t.Fatalf("events = %+v", events)
	}
	if next != 1 {
		t.Fatalf("next = %d, want 1", next)
	}
	// incremental read from offset 1 must be empty
	events, next, _ = m.GetEvents(ctx, "s1", 1)
	if len(events) != 0 || next != 1 {
		t.Fatalf("incremental events = %+v next=%d", events, next)
	}
	// offset beyond end stays put
	if _, next, _ = m.GetEvents(ctx, "s1", 99); next != 1 {
		t.Fatalf("offset beyond end next = %d, want 1", next)
	}
	// unknown stream
	if _, next, _ = m.GetEvents(ctx, "nope", 0); next != 0 {
		t.Fatalf("unknown stream next = %d, want 0", next)
	}
}

func TestMemoryStreamManagerPruneOlderThan(t *testing.T) {
	m := NewMemoryStreamManager()
	ctx := context.Background()
	_ = m.AppendEvent(ctx, "old", StreamEvent{Name: "done", Done: true})
	_ = m.AppendEvent(ctx, "fresh", StreamEvent{Name: "done", Done: true})
	// "old" looks stale; "fresh" is current
	if m.streams["old"] == nil || m.streams["fresh"] == nil {
		t.Fatal("streams not created")
	}
	m.streams["old"].lastUpdated = time.Now().Add(-2 * time.Hour)
	removed := m.PruneOlderThan(ctx, time.Now().Add(-time.Hour))
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, _, _ = m.GetEvents(ctx, "old", 0); m.streams["old"] != nil {
		t.Fatal("old stream should be pruned")
	}
	if m.streams["fresh"] == nil {
		t.Fatal("fresh stream should remain")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/framework/stream/...`
Expected: FAIL 鈥?package does not exist / types undefined.

- [ ] **Step 3: Implement `stream.go`**

`internal/framework/stream/stream.go`:

```go
// Package stream 鎻愪緵 append-only 浜嬩欢娴佹娊璞★紝渚?SSE 浼犺緭灞備娇鐢ㄣ€?package stream

import (
	"context"
	"encoding/json"
	"time"
)

// StreamEvent 鏄祦涓殑涓€涓師瀛愪簨浠讹紝Name+Data 鐩存帴瀵瑰簲涓€鏉?SSE 鎶ユ枃銆?type StreamEvent struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data,omitempty"`
	Done      bool            `json:"done"`
	Timestamp time.Time       `json:"timestamp"`
}

// StreamManager 鍙拷鍔犱簨浠舵祦锛欰ppendEvent 鍐欍€丟etEvents 浠?fromOffset 澧為噺璇汇€?type StreamManager interface {
	AppendEvent(ctx context.Context, streamID string, event StreamEvent) error
	GetEvents(ctx context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error)
}
```

- [ ] **Step 4: Implement `memory_stream_manager.go`**

`internal/framework/stream/memory_stream_manager.go`:

```go
package stream

import (
	"context"
	"sync"
	"time"
)

type memoryStream struct {
	events      []StreamEvent
	lastUpdated time.Time
}

// MemoryStreamManager 杩涚▼鍐呰拷鍔犲紡浜嬩欢娴侊紝浠呭崟瀹炰緥鍙銆?type MemoryStreamManager struct {
	mu      sync.RWMutex
	streams map[string]*memoryStream
}

func NewMemoryStreamManager() *MemoryStreamManager {
	return &MemoryStreamManager{streams: map[string]*memoryStream{}}
}

func (m *MemoryStreamManager) AppendEvent(_ context.Context, streamID string, event StreamEvent) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.streams[streamID]
	if s == nil {
		s = &memoryStream{}
		m.streams[streamID] = s
	}
	s.events = append(s.events, event)
	s.lastUpdated = time.Now()
	return nil
}

func (m *MemoryStreamManager) GetEvents(_ context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error) {
	if m == nil {
		return nil, fromOffset, nil
	}
	if fromOffset < 0 {
		fromOffset = 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.streams[streamID]
	if s == nil || fromOffset >= len(s.events) {
		return nil, len(s.events), nil
	}
	out := make([]StreamEvent, len(s.events)-fromOffset)
	copy(out, s.events[fromOffset:])
	return out, len(s.events), nil
}

// PruneOlderThan 鍒犻櫎 lastUpdated 鏃╀簬 cutoff 鐨勬祦锛岃繑鍥炲垹闄ゆ暟閲忋€?func (m *MemoryStreamManager) PruneOlderThan(_ context.Context, cutoff time.Time) int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, s := range m.streams {
		if s.lastUpdated.Before(cutoff) {
			delete(m.streams, id)
			removed++
		}
	}
	return removed
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/framework/stream/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/framework/stream/
git commit -m "feat: add stream package with memory stream manager"
```

---

### Task 2: Redis stream manager

**Files:**
- Create: `internal/framework/stream/redis_stream_manager.go`
- Create: `internal/framework/stream/redis_stream_manager_test.go`

> Note: no miniredis dependency exists. Test with a live Redis by gating on env `TEST_REDIS_ADDR`; when absent, the test is skipped (keeps CI green without Redis).

- [ ] **Step 1: Write the failing tests**

`internal/framework/stream/redis_stream_manager_test.go`:

```go
package stream

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *RedisStreamManager {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping redis stream manager test")
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	prefix := "test:stream:events:" + os.Getenv("GORAND") + time.Now().String()
	return NewRedisStreamManager(client, prefix, time.Minute)
}

func TestRedisStreamManagerAppendAndGet(t *testing.T) {
	m := newTestRedis(t)
	ctx := context.Background()
	ev := StreamEvent{Name: "meta", Data: []byte(`{"a":1}`), Timestamp: time.Now()}
	if err := m.AppendEvent(ctx, "s1", ev); err != nil {
		t.Fatalf("append: %v", err)
	}
	events, next, err := m.GetEvents(ctx, "s1", 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(events) != 1 || events[0].Name != "meta" || string(events[0].Data) != `{"a":1}` {
		t.Fatalf("events = %+v", events)
	}
	if next != 1 {
		t.Fatalf("next = %d, want 1", next)
	}
	// incremental read
	if events, next, _ = m.GetEvents(ctx, "s1", 1); len(events) != 0 || next != 1 {
		t.Fatalf("incremental events = %+v next=%d", events, next)
	}
	// unknown stream -> empty, offset stays 0
	if events, next, _ = m.GetEvents(ctx, "nope", 0); len(events) != 0 || next != 0 {
		t.Fatalf("unknown stream events = %+v next=%d", events, next)
	}
}
```

- [ ] **Step 2: Run tests to verify they skip**

Run: `go test ./internal/framework/stream/...`
Expected: `redis` test SKIPPED (no TEST_REDIS_ADDR).

- [ ] **Step 3: Implement `redis_stream_manager.go`**

```go
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// RedisStreamManager 鍩轰簬 Redis List 鐨勮法瀹炰緥浜嬩欢娴併€?// key = prefix:streamID锛汚ppend=RPush锛孏et=LRange(fromOffset,-1)銆?type RedisStreamManager struct {
	client *goredis.Client
	prefix string
	ttl    time.Duration
}

func NewRedisStreamManager(client *goredis.Client, prefix string, ttl time.Duration) *RedisStreamManager {
	if prefix == "" {
		prefix = "stream:events"
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &RedisStreamManager{client: client, prefix: prefix, ttl: ttl}
}

func (m *RedisStreamManager) key(streamID string) string {
	return fmt.Sprintf("%s:%s", m.prefix, streamID)
}

func (m *RedisStreamManager) AppendEvent(ctx context.Context, streamID string, event StreamEvent) error {
	if m == nil || m.client == nil {
		return nil
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := m.key(streamID)
	if err := m.client.RPush(ctx, key, raw).Err(); err != nil {
		return err
	}
	return m.client.Expire(ctx, key, m.ttl).Err()
}

func (m *RedisStreamManager) GetEvents(ctx context.Context, streamID string, fromOffset int) ([]StreamEvent, int, error) {
	if m == nil || m.client == nil {
		return nil, fromOffset, nil
	}
	if fromOffset < 0 {
		fromOffset = 0
	}
	values, err := m.client.LRange(ctx, m.key(streamID), int64(fromOffset), -1).Result()
	if err != nil {
		return nil, fromOffset, err
	}
	out := make([]StreamEvent, 0, len(values))
	for _, v := range values {
		var e StreamEvent
		if err := json.Unmarshal([]byte(v), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, fromOffset + len(values), nil
}

// Close 鍏抽棴搴曞眰 Redis 瀹㈡埛绔€?func (m *RedisStreamManager) Close() error {
	if m == nil || m.client == nil {
		return nil
	}
	return m.client.Close()
}
```

- [ ] **Step 4: Run tests (skip with no env; optional manual run against local docker redis)**

Run: `go test ./internal/framework/stream/...`
Expected: PASS (redis test SKIPPED).

Optional manual check (docker redis from docker-compose is `goagent-redis` on localhost:6379):
`$env:TEST_REDIS_ADDR="localhost:6379"; go test ./internal/framework/stream/...`
Expected: PASS, redis test runs.

- [ ] **Step 5: Commit**

```bash
git add internal/framework/stream/redis_stream_manager.go internal/framework/stream/redis_stream_manager_test.go
git commit -m "feat: add redis-backed stream manager"
```

---

### Task 3: Config 鈥?`rag.stream` block

**Files:**
- Modify: `internal/framework/config/config.go`
- Modify: `configs/application.yaml`

- [ ] **Step 1: Add config struct fields**

In `internal/framework/config/config.go`:
- Add `Stream RagStreamConfig \`mapstructure:"stream"\`` to `RagConfig` (after `CitationEnabled`).
- Add the type after `RagDefaultConfig`:

```go
type RagStreamConfig struct {
	Type        string `mapstructure:"type"`        // memory | redis
	TTLSeconds  int    `mapstructure:"ttl-seconds"` // 娴佷繚鐣欐椂闀匡紝榛樿 3600
	RedisPrefix string `mapstructure:"redis-prefix"` // redis 妯″紡 key 鍓嶇紑锛岄粯璁?stream:events
}
```

- [ ] **Step 2: Add yaml block**

In `configs/application.yaml`, immediately under `rag:` (line 38, after `citation-enabled`):

```yaml
  stream:
    type: memory
    ttl-seconds: 3600
```

- [ ] **Step 3: Build to verify**

Run: `go build ./internal/framework/config/...`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/framework/config/config.go configs/application.yaml
git commit -m "feat: add rag.stream config"
```

---

### Task 4: TaskID pre-generation in chat service

**Files:**
- Modify: `internal/app/rag/service/chat/types.go`
- Modify: `internal/app/rag/service/chat/stage_helpers.go`
- Modify: `internal/app/rag/service/chat/prepare_memory.go`
- Modify: `internal/app/rag/service/chat/agent_stage.go`
- Modify: `internal/app/rag/service/aliases.go`
- Test: `internal/app/rag/service/chat/prepare_memory_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Create `internal/app/rag/service/chat/prepare_memory_test.go`:

```go
package chat

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestNextTaskIDRoundTrips(t *testing.T) {
	id, err := NextTaskID()
	if err != nil {
		t.Fatalf("NextTaskID: %v", err)
	}
	if id == "" {
		t.Fatal("NextTaskID returned empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/rag/service/chat/ -run TestNextTaskIDRoundTrips`
Expected: FAIL 鈥?`NextTaskID` undefined.

- [ ] **Step 3: Implement**

In `internal/app/rag/service/chat/stage_helpers.go`, after `nextRagTaskID`:

```go
// NextTaskID 渚?HTTP 灞傞鐢熸垚 task id锛堟祦 key锛夛紝涓庡唴閮ㄧ敓鎴愬櫒鍏变韩瀹炵幇銆?func NextTaskID() (string, error) {
	return nextRagTaskID()
}
```

In `internal/app/rag/service/chat/types.go`, add `TaskID` to both input structs:

```go
type RagChatInput struct {
	ConversationID   string
	UserID           string
	Question         string
	KnowledgeBaseIDs []string
	DeepThinking     bool
	UseAgentRuntime  bool
	RequireApproval  bool
	TaskID           string // 鍙€夛細鐢?HTTP 灞傞鐢熸垚锛岃繍琛屾椂闃舵浼樺厛閲囩敤
}
```

Find `RagChatApprovalResumeInput` (in `agent_stage.go` or `types.go`) and add:

```go
	TaskID           string
```

In `internal/app/rag/service/chat/prepare_memory.go`, modify `runRuntimeStage` (lines 28-31):

```go
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		taskID, err = nextRagTaskID()
		if err != nil {
			return ragChatRuntimeStageResult{}, err
		}
	}
```

In `internal/app/rag/service/chat/agent_stage.go`:
- Change `newAgentRuntimeState` signature (line 220) to accept `taskID string` and adopt it:

```go
func (s *RagChatService) newAgentRuntimeState(ctx context.Context, conversationID string, userID string, taskID string) (ragChatRuntimeState, error) {
	traceID, err := nextRagTraceID()
	if err != nil {
		return ragChatRuntimeState{}, err
	}
	if strings.TrimSpace(taskID) == "" {
		taskID, err = nextRagTaskID()
		if err != nil {
			return ragChatRuntimeState{}, err
		}
	}
	state := ragChatRuntimeState{
		meta: RagChatMeta{
			ConversationID: strings.TrimSpace(conversationID),
			TaskID:         taskID,
		},
		traceID:   traceID,
		startTime: s.tracer.now(),
	}
	_ = s.tracer.startTraceRunAt(ctx, traceID, strings.TrimSpace(conversationID), taskID, strings.TrimSpace(userID), state.startTime)
	return state, nil
}
```

- Update the caller (agent_stage.go ~line 34):

```go
	state, err := s.newAgentRuntimeState(ctx, strings.TrimSpace(input.ConversationID), userID, strings.TrimSpace(input.TaskID))
```

Run `grep -rn "newAgentRuntimeState" internal/` and update ALL other callers the same way.

In `internal/app/rag/service/aliases.go`, add to the type block:

```go
	NextTaskID                      = ragchat.NextTaskID
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./internal/app/rag/service/... && go test ./internal/app/rag/service/chat/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/rag/service/
git commit -m "feat: pre-generate rag chat task id at http layer"
```

---

### Task 5: streamChatSink with wire-format parity

**Files:**
- Create: `internal/adapter/http/rag/stream_chat_sink.go`
- Create: `internal/adapter/http/rag/stream_chat_sink_test.go`

- [ ] **Step 1: Write the failing test (golden payloads)**

`internal/adapter/http/rag/stream_chat_sink_test.go`:

```go
package rag

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/framework/stream"
)

func streamSinkSSEBody(t *testing.T, fn func(s *streamChatSink)) string {
	t.Helper()
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	fn(s)
	events, _, err := m.GetEvents(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString("event: " + e.Name + "\n")
		sb.WriteString("data: " + string(e.Data) + "\n\n")
	}
	return sb.String()
}

func TestStreamChatSinkWireFormatParity(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		if err := s.SendMeta(ragServiceRagChatMetaForTest()); err != nil {
			t.Fatalf("meta: %v", err)
		}
		if err := s.SendThinking("ok"); err != nil {
			t.Fatalf("thinking: %v", err)
		}
		if err := s.SendMessage("hi"); err != nil {
			t.Fatalf("message: %v", err)
		}
		if err := s.SendTitle("t"); err != nil {
			t.Fatalf("title: %v", err)
		}
		if err := s.SendFinish(ragServiceFinishForTest()); err != nil {
			t.Fatalf("finish: %v", err)
		}
		if err := s.SendDone(); err != nil {
			t.Fatalf("done: %v", err)
		}
	})

	want := `event: meta
data: {"conversationId":"c1","taskId":"t1"}

event: message
data: {"type":"think","delta":"ok"}

event: message
data: {"type":"response","delta":"hi"}

event: title
data: {"title":"t"}

event: finish
data: {"messageId":"m1","title":"t"}

event: done
data: {}

`
	if body != want {
		t.Fatalf("wire mismatch:\n--- got ---\n%s--- want ---\n%s", body, want)
	}
}

func TestStreamChatSinkMarksTerminalDone(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	_ = s.SendDone()
	events, _, _ := m.GetEvents(context.Background(), "s1", 0)
	if len(events) != 1 || !events[0].Done || events[0].Name != "done" {
		t.Fatalf("events = %+v", events)
	}
	_ = s.SendError(nil)
	_ = s.SendError(assertError("boom"))
	events, _, _ = m.GetEvents(context.Background(), "s1", 1)
	if len(events) != 1 || !events[0].Done || events[0].Name != "error" || string(events[0].Data) != `{"error":"boom"}` {
		t.Fatalf("error events = %+v", events)
	}
}
```

The two helpers `ragServiceRagChatMetaForTest` and `ragServiceFinishForTest` reference the `ragservice` package types; define them in the test file:

```go
func ragServiceRagChatMetaForTest() ragservice.RagChatMeta {
	return ragservice.RagChatMeta{ConversationID: "c1", TaskID: "t1"}
}

func ragServiceFinishForTest() ragservice.RagChatFinishPayload {
	return ragservice.RagChatFinishPayload{MessageID: "m1", Title: "t"}
}
```

(`assertError` is a tiny helper: `type assertError string; func (e assertError) Error() string { return string(e) }`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/http/rag/ -run 'TestStreamChatSink'`
Expected: FAIL 鈥?`streamChatSink` undefined.

- [ ] **Step 3: Implement `stream_chat_sink.go`**

```go
package rag

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/stream"
)

// streamChatSink 鎶?RagChatEventSink 浜嬩欢鍐欏叆 StreamManager锛岀敱 SSE 杞娑堣垂銆?type streamChatSink struct {
	manager  stream.StreamManager
	streamID string
	seq      int64
}

func (s *streamChatSink) append(name string, payload interface{}, done bool) error {
	if s == nil || s.manager == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.seq++
	return s.manager.AppendEvent(context.Background(), s.streamID, stream.StreamEvent{
		ID:        strconv.FormatInt(s.seq, 10),
		Name:      name,
		Data:      data,
		Done:      done,
		Timestamp: time.Now(),
	})
}

func (s *streamChatSink) SendMeta(meta ragservice.RagChatMeta) error {
	return s.append("meta", meta, false)
}

func (s *streamChatSink) SendFallback(reason string) error {
	return s.append("fallback", gin.H{"reason": reason}, false)
}

func (s *streamChatSink) SendAgentThink(message string) error {
	return s.append("agent_think", gin.H{"message": message}, false)
}

func (s *streamChatSink) SendAgentOutcome(payload ragservice.RagChatAgentOutcomePayload) error {
	if err := s.append("agent_outcome", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentOutcomeStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendApprovalPending(payload ragservice.RagChatApprovalPendingPayload) error {
	if err := s.append("approval_pending", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentApprovalStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendAgentServiceError(payload ragservice.RagChatAgentServiceErrorPayload) error {
	if err := s.append("agent_service_error", payload, false); err != nil {
		return err
	}
	return s.append("agent_status", newAgentServiceErrorStatusEventPayload(payload), false)
}

func (s *streamChatSink) SendMemoryStored(payload ragservice.RagChatMemoryStoredPayload) error {
	return s.append("memory_stored", payload, false)
}

func (s *streamChatSink) SendSessionRecall(payload ragservice.RagChatSessionRecallPayload) error {
	return s.append("session_recall", payload, false)
}

func (s *streamChatSink) SendThinking(delta string) error {
	return s.append("message", gin.H{"type": "think", "delta": delta}, false)
}

func (s *streamChatSink) SendMessage(delta string) error {
	return s.append("message", gin.H{"type": "response", "delta": delta}, false)
}

func (s *streamChatSink) SendToolStart(payload ragtool.ToolCallEvent) error {
	return s.append("tool_start", payload, false)
}

func (s *streamChatSink) SendToolResult(payload ragtool.ToolCallEvent) error {
	return s.append("tool_result", payload, false)
}

func (s *streamChatSink) SendTool(name string, status string, summary string) error {
	return s.append("tool", gin.H{"name": name, "status": status, "summary": summary}, false)
}

func (s *streamChatSink) SendTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return nil
	}
	return s.append("title", gin.H{"title": title}, false)
}

func (s *streamChatSink) SendFinish(payload ragservice.RagChatFinishPayload) error {
	return s.append("finish", gin.H{"messageId": payload.MessageID, "title": payload.Title}, false)
}

func (s *streamChatSink) SendCancel(payload ragservice.RagChatFinishPayload) error {
	return s.append("cancel", gin.H{"messageId": payload.MessageID, "title": payload.Title}, true)
}

func (s *streamChatSink) SendError(err error) error {
	if err == nil {
		return nil
	}
	return s.append("error", gin.H{"error": err.Error()}, true)
}

func (s *streamChatSink) SendDone() error {
	return s.append("done", gin.H{}, true)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/http/rag/ -run 'TestStreamChatSink'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/http/rag/stream_chat_sink.go internal/adapter/http/rag/stream_chat_sink_test.go
git commit -m "feat: add stream chat sink with wire-format parity"
```

---

### Task 6: SSE poller and stop watcher

**Files:**
- Create: `internal/adapter/http/rag/stream_transport.go`
- Create: `internal/adapter/http/rag/stream_transport_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/adapter/http/rag/stream_transport_test.go`:

```go
package rag

import (
	"context"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/framework/stream"
)

func TestPollLoopForwardsEventsAndStopsOnDone(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "meta", Data: []byte(`{"a":1}`)})
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "message", Data: []byte(`{"type":"response","delta":"hi"}`)})
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector := newCollectorSink()
	pollLoop(ctx, collector, m, "s1", 0, 10*time.Millisecond, time.Second)

	if !collector.completed {
		t.Fatal("poller did not complete on done event")
	}
	want := "event: meta\ndata: {\"a\":1}\n\nevent: message\ndata: {\"type\":\"response\",\"delta\":\"hi\"}\n\nevent: done\ndata: {}\n\n"
	if collector.body != want {
		t.Fatalf("body mismatch:\n--- got ---\n%s--- want ---\n%s", collector.body, want)
	}
}

func TestPollLoopFiltersStopControlEvent(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: internalStopEventName, Data: []byte(`{}`), Done: true})
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "cancel", Data: []byte(`{}`), Done: true})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	collector := newCollectorSink()
	pollLoop(ctx, collector, m, "s1", 0, 10*time.Millisecond, time.Second)

	if strings.Contains(collector.body, "event: stop") {
		t.Fatalf("stop control event leaked to SSE: %s", collector.body)
	}
	if !strings.Contains(collector.body, "event: cancel") {
		t.Fatalf("cancel event missing: %s", collector.body)
	}
}

func TestPollLoopExitsOnContextDone(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithCancel(context.Background())
	collector := newCollectorSink()
	done := make(chan struct{})
	go func() {
		pollLoop(ctx, collector, m, "s1", 0, 10*time.Millisecond, time.Minute)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("poller did not exit on ctx done")
	}
}

func TestStopWatcherCancelsOnStopEvent(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cancelled := make(chan struct{})
	go stopWatcher(ctx, m, "s1", func() bool { close(cancelled); return true }, 10*time.Millisecond, time.Second)
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: internalStopEventName, Data: []byte(`{}`), Done: true})
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("watcher did not cancel on stop event")
	}
}

func TestStopWatcherExitsOnTaskDone(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cancelled := make(chan struct{})
	done := make(chan struct{})
	go func() {
		stopWatcher(ctx, m, "s1", func() bool { close(cancelled); return true }, 10*time.Millisecond, time.Second)
		close(done)
	}()
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not exit on task done")
	}
	select {
	case <-cancelled:
		t.Fatal("watcher must not cancel on normal done")
	default:
	}
}

// collectorSink 妯℃嫙 SseEmitterSender 鐨?SendEvent/Complete 濂戠害銆?type collectorSink struct {
	body      string
	completed bool
}

func newCollectorSink() *collectorSink { return &collectorSink{} }

func (c *collectorSink) SendEvent(name string, data interface{}) error {
	raw := []byte{}
	switch v := data.(type) {
	case []byte:
		raw = v
	default:
		raw = []byte(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(formatAny(v), "data: "), "event: ")))
	}
	c.body += "event: " + name + "\n" + "data: " + string(raw) + "\n\n"
	return nil
}

func (c *collectorSink) Complete() { c.completed = true }
```

(Add `"encoding/json"` and `"github.com/gin-gonic/gin"` to the test file imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/http/rag/ -run 'TestPollLoop|TestStopWatcher'`
Expected: FAIL 鈥?`pollLoop`/`stopWatcher` undefined.

- [ ] **Step 3: Implement `stream_transport.go`**

```go
package rag

import (
	"context"
	"time"

	"local/rag-project/internal/framework/stream"
)

const (
	internalStopEventName = "stop"

	defaultStreamPollInterval  = 100 * time.Millisecond
	defaultStopWatcherInterval = 200 * time.Millisecond
)

// streamEventSender 鏄?pollLoop 鍐?SSE 鐨勬渶灏忎緷璧栵紙SseEmitterSender 婊¤冻锛夈€?type streamEventSender interface {
	SendEvent(eventName string, data interface{}) error
	Complete()
}

// pollLoop 浠?startOffset 澧為噺璇绘祦骞跺啓 SSE锛岃鍒?Done=true 浜嬩欢鎴?ctx 缁撴潫銆?// stop 鏄唴閮ㄦ帶鍒朵簨浠讹紝涓嶈浆鍙戠粰瀹㈡埛绔€?func pollLoop(ctx context.Context, sender streamEventSender, manager stream.StreamManager, streamID string, startOffset int, interval time.Duration, maxDuration time.Duration) {
	if interval <= 0 {
		interval = defaultStreamPollInterval
	}
	if maxDuration <= 0 {
		maxDuration = resolveDefaultMaxStreamDuration()
	}
	offset := startOffset
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	maxTimer := time.NewTimer(maxDuration)
	defer maxTimer.Stop()
	for {
		events, next, err := manager.GetEvents(ctx, streamID, offset)
		if err == nil {
			for _, e := range events {
				if e.Name == internalStopEventName {
					continue
				}
				if sendErr := sender.SendEvent(e.Name, e.Data); sendErr != nil {
					return
				}
				if e.Done {
					sender.Complete()
					return
				}
			}
			offset = next
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-maxTimer.C:
			sender.Complete()
			return
		}
	}
}

// stopWatcher 鍦ㄦ墽琛岃妭鐐圭洃鍚?stop 鎺у埗浜嬩欢锛岃Е鍙?cancelTask銆?// 鍙嶅璋冪敤 cancelTask 鐩村埌鎴愬姛鎴栦换鍔＄粨鏉燂紝瑙勯伩浠诲姟灏氭湭娉ㄥ唽鐨勭珵鎬併€?func stopWatcher(ctx context.Context, manager stream.StreamManager, streamID string, cancelTask func() bool, interval time.Duration, maxDuration time.Duration) {
	if interval <= 0 {
		interval = defaultStopWatcherInterval
	}
	if maxDuration <= 0 {
		maxDuration = resolveDefaultMaxStreamDuration()
	}
	offset := 0
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	maxTimer := time.NewTimer(maxDuration)
	defer maxTimer.Stop()
	stopSeen := false
	for {
		events, next, err := manager.GetEvents(ctx, streamID, offset)
		if err == nil {
			for _, e := range events {
				if e.Name == internalStopEventName {
					stopSeen = true
				} else if e.Done {
					return
				}
			}
			offset = next
		}
		if stopSeen && cancelTask() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-maxTimer.C:
			return
		}
	}
}

// resolveDefaultMaxStreamDuration 澶嶇敤 rag.default.sse-timeout-ms锛屽厹搴?5 鍒嗛挓銆?func resolveDefaultMaxStreamDuration() time.Duration {
	d := defaultMaxStreamDuration
	if cfg := frameworkConfig.Get(); cfg != nil && cfg.Rag.Default.SseTimeoutMs > 0 {
		d = time.Duration(cfg.Rag.Default.SseTimeoutMs) * time.Millisecond
	}
	return d
}
```

Add imports `fwconfig "local/rag-project/internal/framework/config"` (aliased to avoid collision) and the constant:

```go
const defaultMaxStreamDuration = 5 * time.Minute
```

Note: `resolveDefaultMaxStreamDuration` uses the config package; alias the import as `frameworkconfig` in the file.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapter/http/rag/ -run 'TestPollLoop|TestStopWatcher'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/http/rag/stream_transport.go internal/adapter/http/rag/stream_transport_test.go
git commit -m "feat: add SSE poll loop and stop watcher"
```

---

### Task 7: Handler rework 鈥?Chat / ResumeAfterApproval / StopChat / ContinueChat

**Files:**
- Modify: `internal/adapter/http/rag/chat_handler.go`
- Modify: `internal/adapter/http/rag/handlers.go`
- Modify: `internal/adapter/http/rag/routes.go`
- Modify: `internal/adapter/http/rag/test/chat_handler_test.go`

- [ ] **Step 1: Write the failing tests (append to existing test file)**

In `internal/adapter/http/rag/test/chat_handler_test.go`, update `newChatRouter` to pass a nil stream manager (signature change) and add:

```go
// taskIDFromMeta 浠?SSE body 鎻愬彇 meta 浜嬩欢鐨?taskId銆?func taskIDFromMeta(t *testing.T, body string) string {
	t.Helper()
	idx := strings.Index(body, `"taskId":`)
	if idx < 0 {
		t.Fatalf("no taskId in body: %s", body)
	}
	rest := body[idx:]
	start := strings.IndexByte(rest, '"') + 1
	rest = rest[start:]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		t.Fatalf("cannot parse taskId from body: %s", body)
	}
	return rest[:end]
}

func TestChatHandlerStreamsAndContinueReplays(t *testing.T) {
	router := newChatRouter(chatServiceStub{
		chatFn: func(_ context.Context, input ragservice.RagChatInput, sink ragservice.RagChatEventSink) error {
			if err := sink.SendMeta(ragservice.RagChatMeta{ConversationID: input.ConversationID, TaskID: input.TaskID}); err != nil {
				return err
			}
			if err := sink.SendMessage("hello"); err != nil {
				return err
			}
			return sink.SendDone()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/chat?question=hi", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "event: message") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("live chat stream incomplete: %s", rec.Body.String())
	}
	taskID := taskIDFromMeta(t, rec.Body.String())

	// continue must replay the same events from the stream.
	req2 := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/chat/continue?taskId="+taskID, nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "event: message") || !strings.Contains(rec2.Body.String(), `"delta":"hello"`) {
		t.Fatalf("continue replay incomplete: %s", rec2.Body.String())
	}
}

func TestChatHandlerStopIdempotentSuccess(t *testing.T) {
	router := newChatRouter(chatServiceStub{})
	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/stop?taskId=does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop should be idempotent success, got %d", rec.Code)
	}
}

func TestChatHandlerContinueWithUnknownTaskCompletesGracefully(t *testing.T) {
	router := newChatRouter(chatServiceStub{})
	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/chat/continue?taskId=missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("continue status = %d", rec.Code)
	}
}
```

> Determinism note: the mid-flight `/stop` cross-node cancellation is covered by the `stopWatcher` unit tests in Task 6; the handler-level stop assertion here only checks idempotent success. Do NOT add a concurrency test that calls `router.ServeHTTP` from two goroutines against a `ResponseRecorder` 鈥?it is not safe.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapter/http/rag/test/`
Expected: FAIL 鈥?handler signature/route changes not yet made.

- [ ] **Step 3: Rework `handlers.go`**

Add the stream manager to the handler:

```go
import (
	...
	"local/rag-project/internal/framework/stream"
)

type Handler struct {
	conversationService *ragservice.ConversationService
	messageService      *ragservice.ConversationMessageService
	memoryService       *longtermmemory.MemoryService
	feedbackService     *ragservice.MessageFeedbackService
	chatService         chatService
	preferenceCandidateService longtermmemory.PreferenceCandidateService
	streamManager       stream.StreamManager
}

func NewHandler(
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService chatService,
	preferenceCandidateService longtermmemory.PreferenceCandidateService,
	streamManager stream.StreamManager,
) *Handler {
	if streamManager == nil {
		streamManager = stream.NewMemoryStreamManager()
	}
	return &Handler{..., streamManager: streamManager}
}
```

- [ ] **Step 4: Rework `chat_handler.go`**

Replace the `Chat`, `ResumeAfterApproval`, `StopChat` bodies and delete the `sseChatSink` type. New file contents:

```go
package rag

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/exception"
	fwweb "local/rag-project/internal/framework/web"
	"local/rag-project/internal/framework/stream"
)

type resumeApprovalRequest struct {
	ConversationID string `json:"conversationId"`
	Question       string `json:"question"`
	CheckpointID   string `json:"checkpointId"`
	Decision       string `json:"decision"`
	DecisionNote   string `json:"decisionNote"`
}

// Chat 閫氳繃銆岃亰澶╁崗绋嬪啓娴?+ SSE 杞銆嶈緭鍑烘祦寮忕粨鏋溿€?func (h *Handler) Chat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID, err := ragservice.NextTaskID()
	if err != nil {
		_ = c.Error(err)
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	sink := &streamChatSink{manager: h.streamManager, streamID: taskID}
	baseCtx := context.WithoutCancel(c.Request.Context())
	go func() {
		_ = h.chatService.Chat(baseCtx, ragservice.RagChatInput{
			ConversationID:   strings.TrimSpace(c.Query("conversationId")),
			UserID:           user.UserID,
			Question:         strings.TrimSpace(c.Query("question")),
			KnowledgeBaseIDs: splitCommaValues(c.Query("knowledgeBaseId")),
			DeepThinking:     parseBool(c.Query("deepThinking")),
			RequireApproval:  parseBool(c.Query("requireApproval")),
			TaskID:           taskID,
		}, sink)
	}()
	go stopWatcher(baseCtx, h.streamManager, taskID, func() bool { return h.chatService.CancelTask(taskID) }, defaultStopWatcherInterval, 0)
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// ResumeAfterApproval 鎭㈠瀹℃壒鍚庣殑 agent 娴侊紝鍚屾牱璧版祦浼犺緭銆?func (h *Handler) ResumeAfterApproval(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	var req resumeApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(err)
		return
	}
	taskID, err := ragservice.NextTaskID()
	if err != nil {
		_ = c.Error(err)
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	sink := &streamChatSink{manager: h.streamManager, streamID: taskID}
	baseCtx := context.WithoutCancel(c.Request.Context())
	go func() {
		_ = h.chatService.ResumeAfterApproval(baseCtx, ragservice.RagChatApprovalResumeInput{
			ConversationID: strings.TrimSpace(req.ConversationID),
			UserID:         user.UserID,
			Question:       strings.TrimSpace(req.Question),
			CheckpointID:   strings.TrimSpace(req.CheckpointID),
			Decision:       strings.TrimSpace(req.Decision),
			DecisionNote:   strings.TrimSpace(req.DecisionNote),
			TaskID:         taskID,
		}, sink)
	}()
	go stopWatcher(baseCtx, h.streamManager, taskID, func() bool { return h.chatService.CancelTask(taskID) }, defaultStopWatcherInterval, 0)
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// ContinueChat 鏂嚎閲嶈繛锛氬洖鏀惧凡鏈夋祦骞剁户缁疆璇€?func (h *Handler) ContinueChat(c *gin.Context) {
	user := requireLoginUser(c)
	if user == nil {
		return
	}
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	sender := fwweb.NewSseEmitterSender(c)
	events, _, err := h.streamManager.GetEvents(c.Request.Context(), taskID, 0)
	if err != nil || len(events) == 0 {
		sender.Complete()
		return
	}
	pollLoop(c.Request.Context(), sender, h.streamManager, taskID, 0, defaultStreamPollInterval, 0)
}

// StopChat 鍙岄€氶亾鍙栨秷锛氭湰鍦?taskRegistry 蹇€熻矾寰?+ 鍐欏叆 stop 鎺у埗浜嬩欢锛堣法鑺傜偣锛夈€?func (h *Handler) StopChat(c *gin.Context) {
	taskID := strings.TrimSpace(c.Query("taskId"))
	if taskID == "" {
		_ = c.Error(exception.NewClientException("task id is required", nil))
		return
	}
	_ = h.chatService.CancelTask(taskID)
	stopData, _ := json.Marshal(gin.H{})
	_ = h.streamManager.AppendEvent(c.Request.Context(), taskID, stream.StreamEvent{
		Name:      internalStopEventName,
		Data:      stopData,
		Done:      true,
		Timestamp: time.Now(),
	})
	writeSuccess[any](c, nil)
}
```

- [ ] **Step 5: Update routes**

`internal/adapter/http/rag/routes.go`:

```go
func RegisterRoutes(
	r gin.IRouter,
	conversationService *ragservice.ConversationService,
	messageService *ragservice.ConversationMessageService,
	memoryService *longtermmemory.MemoryService,
	feedbackService *ragservice.MessageFeedbackService,
	chatService chatService,
	preferenceCandidateService longtermmemory.PreferenceCandidateService,
	traceService *ragservice.TraceService,
	cacheMetrics *ragcachemetrics.Service,
	streamManager stream.StreamManager,
) {
	handler := NewHandler(conversationService, messageService, memoryService, feedbackService, chatService, preferenceCandidateService, streamManager)
	...
	r.GET("/rag/v3/chat/continue", handler.ContinueChat)
	...
}
```

Add `"local/rag-project/internal/framework/stream"` to imports.

- [ ] **Step 6: Update the existing test call site**

In `internal/adapter/http/rag/test/chat_handler_test.go`, `newChatRouter` (line ~189):

```go
raghttp.RegisterRoutes(group, nil, nil, nil, nil, chatService, nil, nil, nil, nil)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go build ./internal/adapter/http/rag/... && go test ./internal/adapter/http/rag/...`
Expected: PASS (new + existing).

- [ ] **Step 8: Commit**

```bash
git add internal/adapter/http/rag/
git commit -m "feat: stream transport for chat handler with continue endpoint"
```

---

### Task 8: Bootstrap wiring 鈥?stream manager into Runtime and main

**Files:**
- Create: `internal/bootstrap/rag/runtime_build_stream.go`
- Modify: `internal/bootstrap/rag/runtime.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Implement `runtime_build_stream.go`**

```go
package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	frameworkconfig "local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/stream"
)

// buildStreamManager 鎸?rag.stream.type 鏋勯€犳祦瀛樺偍锛歮emory锛堥粯璁わ級鎴?redis銆?func buildStreamManager(cfg *frameworkconfig.Config) stream.StreamManager {
	if cfg == nil || strings.EqualFold(strings.TrimSpace(cfg.Rag.Stream.Type), "memory") {
		return stream.NewMemoryStreamManager()
	}
	host := strings.TrimSpace(cfg.Spring.Data.Redis.Host)
	port := cfg.Spring.Data.Redis.Port
	if host == "" || port <= 0 {
		return stream.NewMemoryStreamManager()
	}
	client := goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: cfg.Spring.Data.Redis.Password,
		DB:       cfg.Spring.Data.Redis.DB,
	})
	ttl := time.Duration(cfg.Rag.Stream.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	return stream.NewRedisStreamManager(client, cfg.Rag.Stream.RedisPrefix, ttl)
}

// startStreamSweep 涓哄唴瀛樻祦瀛樺偍鍚姩 TTL 娓呮壂鍗忕▼锛坮edis 鐗堢敱 Redis Expire 鍏滃簳锛夈€?func startStreamSweep(r *Runtime, cfg *frameworkconfig.Config) {
	if cfg == nil || r == nil {
		return
	}
	mm, ok := r.StreamManager.(*stream.MemoryStreamManager)
	if !ok {
		return
	}
	ttl := time.Duration(cfg.Rag.Stream.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.streamSweepCancel = cancel
	r.streamSweepWG.Add(1)
	go func() {
		defer r.streamSweepWG.Done()
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				mm.PruneOlderThan(ctx, time.Now().Add(-ttl))
			case <-ctx.Done():
				return
			}
		}
	}()
}

func stopStreamSweep(r *Runtime) {
	if r == nil || r.streamSweepCancel == nil {
		return
	}
	r.streamSweepCancel()
	r.streamSweepCancel = nil
	r.streamSweepWG.Wait()
}
```

- [ ] **Step 2: Wire into `runtime.go`**

Add fields to `Runtime`:

```go
	StreamManager       stream.StreamManager
	streamSweepCancel   context.CancelFunc
	streamSweepWG       sync.WaitGroup
```

In `NewRuntime`, after the runtime struct is built (before `runtime.startMemoryMaintenanceLoop(...)`):

```go
	runtime.StreamManager = buildStreamManager(buildCtx.cfg)
	startStreamSweep(runtime, buildCtx.cfg)
```

In `Close`, before `r.stopMemoryMaintenanceLoop()`:

```go
	stopStreamSweep(r)
	if sm, ok := r.StreamManager.(interface{ Close() error }); ok {
		_ = sm.Close()
	}
```

Add the `stream` import to `runtime.go`.

- [ ] **Step 3: Pass into `RegisterRoutes` in main**

`cmd/server/main.go:259`:

```go
	raghttp.RegisterRoutes(protected, runtime.Conversation, runtime.Message, runtime.Memory, runtime.Feedback, runtime.Chat, runtime.PreferenceCandidates, runtime.Trace, runtime.CacheMetrics, runtime.StreamManager)
```

- [ ] **Step 4: Build and test**

Run: `go build ./cmd/... ./internal/...`
Expected: success (ignore the pre-existing `scripts/` package which has multiple `main` funcs).

Run: `go test ./internal/bootstrap/rag/... ./internal/adapter/http/rag/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bootstrap/rag/ cmd/server/main.go
git commit -m "feat: wire stream manager into runtime and server main"
```

---

### Task 9: End-to-end verification (runtime)

**Files:** none (verification only)

- [ ] **Step 1: Restart the backend**

Kill the current server on 9090, rebuild, start, and confirm the port is listening (see the session's established restart procedure; server log at `C:\Users\1\AppData\Local\Temp\opencode\goagent-server.log`).

- [ ] **Step 2: Verify live chat still streams**

Login as `citetest` (credentials in the test user set). `GET /api/ragent/rag/v3/chat?question=...&conversationId=...` with the Bearer token. Expect the same SSE event sequence as before (`meta` 鈫?`message` 鈫?... 鈫?`finish` 鈫?`done`), including a `taskId` in the `meta` event.

- [ ] **Step 3: Verify continue replays**

Take the `taskId` from the `meta` event of a completed chat, then `GET /api/ragent/rag/v3/chat/continue?taskId=<id>`. Expect the full event sequence replayed then `done`.

- [ ] **Step 4: Verify stop still works**

`POST /api/ragent/rag/v3/stop?taskId=<taskId-of-running-chat>` returns 200, and the running chat's SSE receives `cancel`/`done`.

- [ ] **Step 5: Verify Redis mode (optional)**

Set `rag.stream.type: redis` in `configs/application.yaml`, restart, and repeat Steps 2-4 (the docker `goagent-redis` at localhost:6379 is available). Confirm `stream:events:*` keys appear in Redis.

- [ ] **Step 6: Final test sweep**

Run: `go build ./cmd/... ./internal/...`
Run: `go test ./internal/framework/stream/... ./internal/adapter/http/rag/... ./internal/app/rag/service/chat/... ./internal/bootstrap/rag/...`
Expected: all PASS.

---

## Self-Review

- **Spec coverage:** stream package (Task 1-2), config (Task 3), taskID pre-gen (Task 4), sink (Task 5), poller+watcher (Task 6), handler+continue+stop dual-channel (Task 7), bootstrap+main wiring (Task 8), runtime verify (Task 9). Spec section 4 (frontend zero-change) is a non-code guarantee enforced by the Task 5 parity test.
- **Type consistency:** `stream.StreamManager`/`stream.StreamEvent` defined in Task 1 and used identically in Tasks 5-8. `streamChatSink{manager, streamID}` consistent. `pollLoop`/`stopWatcher` signatures consistent between Task 6 definition and Task 7 call sites. `internalStopEventName` defined in Task 6, used in Task 7. `NextTaskID` defined in Task 4, used in Task 7. `RagChatInput.TaskID` / `RagChatApprovalResumeInput.TaskID` defined in Task 4, used in Task 7.
- **Placeholder scan:** every step carries exact paths, full code, and expected test output.
