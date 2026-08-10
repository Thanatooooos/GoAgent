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
	prefix := "test:stream:events:" + time.Now().String()
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
