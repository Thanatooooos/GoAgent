package rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

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

func TestPollLoopMaxDurationIdleTimeout(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	collector := newCollectorSink()
	// No events ever appended; idle timeout should close the stream.
	pollLoop(ctx, collector, m, "s1", 0, 20*time.Millisecond, 50*time.Millisecond)
	if !collector.completed {
		t.Fatal("poller did not complete via idle timeout")
	}
}

func TestPollLoopLongStreamNotCutByIdleReset(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	collector := newCollectorSink()
	done := make(chan struct{})
	go func() {
		pollLoop(ctx, collector, m, "s1", 0, 10*time.Millisecond, 50*time.Millisecond)
		close(done)
	}()
	// Keep appending events longer than the 50ms maxDuration; the reset must keep the stream alive.
	for i := 0; i < 6; i++ {
		time.Sleep(20 * time.Millisecond)
		_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "message", Data: []byte(`{"type":"response","delta":"x"}`)})
	}
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller was cut by idle timeout despite continuous events")
	}
	if !strings.Contains(collector.body, "event: done") {
		t.Fatalf("done not reached: %s", collector.body)
	}
}

func TestStopWatcherRetriesUntilCancelSucceeds(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	done := make(chan struct{})
	go func() {
		stopWatcher(ctx, m, "s1", func() bool { calls++; return calls >= 3 }, 10*time.Millisecond, time.Second)
		close(done)
	}()
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: internalStopEventName, Data: []byte(`{}`), Done: true})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not exit")
	}
	if calls < 3 {
		t.Fatalf("cancelTask called %d times, expected retries until success", calls)
	}
}

func TestPollLoopAbortsOnSendError(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = m.AppendEvent(context.Background(), "s1", stream.StreamEvent{Name: "meta", Data: []byte(`{}`)})
	failing := &failingCollectorSink{errOnEvent: true}
	pollLoop(ctx, failing, m, "s1", 0, 10*time.Millisecond, time.Second)
	if failing.sent == 0 {
		t.Fatal("expected at least one event attempted")
	}
}

// collectorSink 模拟 SseEmitterSender 的 SendEvent/Complete 契约。
type collectorSink struct {
	body      string
	completed bool
}

func newCollectorSink() *collectorSink { return &collectorSink{} }

func (c *collectorSink) SendEvent(name string, data interface{}) error {
	var raw []byte
	switch v := data.(type) {
	case []byte:
		raw = v
	case gin.H:
		raw, _ = json.Marshal(v)
	default:
		raw, _ = json.Marshal(data)
	}
	c.body += "event: " + name + "\ndata: " + string(raw) + "\n\n"
	return nil
}

func (c *collectorSink) Complete() { c.completed = true }

type failingCollectorSink struct {
	errOnEvent bool
	sent       int
	body       string
}

func (c *failingCollectorSink) SendEvent(name string, data interface{}) error {
	c.sent++
	if c.errOnEvent {
		return assertError("write failed")
	}
	c.body += "event: " + name + "\n"
	return nil
}

func (c *failingCollectorSink) Complete() {}
