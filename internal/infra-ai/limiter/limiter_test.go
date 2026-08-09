package limiter

import (
	"sync"
	"testing"
	"time"
)

func TestGovernorPassthroughWhenNoLimit(t *testing.T) {
	g := NewGovernor()
	called := 0
	if err := g.Gate("m", 0, func() error { called++; return nil }); err != nil {
		t.Fatalf("err = %v", err)
	}
	if err := g.Gate("", 2, func() error { called++; return nil }); err != nil {
		t.Fatalf("empty key err = %v", err)
	}
	if called != 2 {
		t.Fatalf("called = %d, want 2", called)
	}
}

func TestGovernorLimitsConcurrency(t *testing.T) {
	g := NewGovernor()
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	var wg sync.WaitGroup
	const n = 8
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = g.Gate("m", 3, func() error {
				mu.Lock()
				inFlight++
				if inFlight > maxInFlight {
					maxInFlight = inFlight
				}
				mu.Unlock()
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				inFlight--
				mu.Unlock()
				return nil
			})
		}()
	}
	close(start)
	wg.Wait()
	if maxInFlight > 3 {
		t.Fatalf("max in-flight = %d, want <= 3", maxInFlight)
	}
	if maxInFlight < 1 {
		t.Fatal("expected at least 1 in-flight")
	}
}

func TestGovernorIsolatesKeys(t *testing.T) {
	g := NewGovernor()
	release := make(chan struct{})
	started := make(chan struct{})
	go func() {
		_ = g.Gate("a", 1, func() error { close(started); <-release; return nil })
	}()
	<-started // key "a" slot held
	done := make(chan struct{})
	go func() {
		_ = g.Gate("b", 1, func() error { close(done); return nil })
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("key b should run while key a is saturated")
	}
	close(release)
}
