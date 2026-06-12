package retrieve

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"local/rag-project/internal/framework/convention"
)

type stubSearchChannel struct {
	name     string
	priority int
	enabled  bool
	delay    time.Duration
	chunks   []convention.RetrievedChunk
	err      error
	calls    *int32
}

func (c *stubSearchChannel) Name() string  { return c.name }
func (c *stubSearchChannel) Priority() int { return c.priority }
func (c *stubSearchChannel) Enabled(SearchContext) bool {
	return c.enabled
}

func (c *stubSearchChannel) Search(ctx context.Context, _ SearchContext) (SearchChannelResult, error) {
	if c.calls != nil {
		atomic.AddInt32(c.calls, 1)
	}
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return SearchChannelResult{}, ctx.Err()
		}
	}
	if c.err != nil {
		return SearchChannelResult{}, c.err
	}
	return SearchChannelResult{
		ChannelName: c.name,
		Chunks:      c.chunks,
	}, nil
}

func TestExecuteChannelsPreservesRegistrationOrder(t *testing.T) {
	engine := &Engine{
		channels: []SearchChannel{
			&stubSearchChannel{name: ChannelVectorGlobal, enabled: true, chunks: []convention.RetrievedChunk{{ID: "v1"}}},
			&stubSearchChannel{name: ChannelKeyword, enabled: true, chunks: []convention.RetrievedChunk{{ID: "k1"}}},
			&stubSearchChannel{name: ChannelMetadataTitle, enabled: true, chunks: []convention.RetrievedChunk{{ID: "m1"}}},
		},
	}

	results, err := engine.executeChannels(context.Background(), SearchContext{Query: "test", SearchMode: SearchModeHybrid})
	if err != nil {
		t.Fatalf("executeChannels() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 channel results, got %d", len(results))
	}
	expected := []string{ChannelVectorGlobal, ChannelKeyword, ChannelMetadataTitle}
	for i, name := range expected {
		if results[i].ChannelName != name {
			t.Fatalf("results[%d].ChannelName = %q, want %q", i, results[i].ChannelName, name)
		}
	}
}

func TestExecuteChannelsPartialFailureContinues(t *testing.T) {
	engine := &Engine{
		channels: []SearchChannel{
			&stubSearchChannel{name: ChannelVectorGlobal, enabled: true, err: errors.New("vector down")},
			&stubSearchChannel{name: ChannelKeyword, enabled: true, chunks: []convention.RetrievedChunk{{ID: "k1"}}},
		},
	}

	results, err := engine.executeChannels(context.Background(), SearchContext{Query: "test", SearchMode: SearchModeHybrid})
	if err != nil {
		t.Fatalf("expected partial success, got error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 channel results, got %d", len(results))
	}
	if results[0].Error == "" || results[1].Error != "" {
		t.Fatalf("expected first channel failed and second succeeded, got %+v", results)
	}
}

func TestExecuteChannelsAllFailedReturnsError(t *testing.T) {
	engine := &Engine{
		channels: []SearchChannel{
			&stubSearchChannel{name: ChannelVectorGlobal, enabled: true, err: errors.New("vector down")},
			&stubSearchChannel{name: ChannelKeyword, enabled: true, err: errors.New("keyword down")},
		},
	}

	_, err := engine.executeChannels(context.Background(), SearchContext{Query: "test", SearchMode: SearchModeHybrid})
	if err == nil {
		t.Fatal("expected error when all channels fail")
	}
}

func TestExecuteChannelsNoEnabledReturnsError(t *testing.T) {
	engine := &Engine{
		channels: []SearchChannel{
			&stubSearchChannel{name: ChannelVectorGlobal, enabled: false},
		},
	}
	_, err := engine.executeChannels(context.Background(), SearchContext{Query: "test"})
	if err == nil {
		t.Fatal("expected error when no channels enabled")
	}
}

func TestExecuteChannelsRunsInParallel(t *testing.T) {
	var calls int32
	delay := 50 * time.Millisecond
	engine := &Engine{
		channels: []SearchChannel{
			&stubSearchChannel{name: ChannelVectorGlobal, enabled: true, delay: delay, calls: &calls},
			&stubSearchChannel{name: ChannelKeyword, enabled: true, delay: delay, calls: &calls},
			&stubSearchChannel{name: ChannelMetadataTitle, enabled: true, delay: delay, calls: &calls},
		},
	}

	startedAt := time.Now()
	_, err := engine.executeChannels(context.Background(), SearchContext{Query: "test", SearchMode: SearchModeHybrid})
	if err != nil {
		t.Fatalf("executeChannels() error = %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("expected 3 channel calls, got %d", calls)
	}
	elapsed := time.Since(startedAt)
	if elapsed >= 3*delay {
		t.Fatalf("expected parallel execution under %v, took %v", 3*delay, elapsed)
	}
}
