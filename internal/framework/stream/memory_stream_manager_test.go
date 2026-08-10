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
