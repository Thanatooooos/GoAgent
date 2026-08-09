package embedding

import (
	"errors"
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/limiter"
	"local/rag-project/internal/infra-ai/model"
)

type recordingEmbeddingClient struct {
	mu         sync.Mutex
	embedCalls int
	batchCalls int
	batchFn    func(texts []string, target model.ModelTarget) ([][]float32, error)
}

func (c *recordingEmbeddingClient) Provider() string { return "fake" }

func (c *recordingEmbeddingClient) Embed(text string, target model.ModelTarget) ([]float32, error) {
	c.mu.Lock()
	c.embedCalls++
	c.mu.Unlock()
	return []float32{1}, nil
}

func (c *recordingEmbeddingClient) EmbedBatch(texts []string, target model.ModelTarget) ([][]float32, error) {
	c.mu.Lock()
	c.batchCalls++
	c.mu.Unlock()
	if c.batchFn != nil {
		return c.batchFn(texts, target)
	}
	return [][]float32{{1}}, nil
}

func TestConcurrencyEmbeddingClientEmbedPassthrough(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 1}
	if _, err := client.Embed("q", model.ModelTarget{Id: "m"}); err != nil {
		t.Fatalf("err = %v", err)
	}
	if inner.embedCalls != 1 {
		t.Fatalf("embedCalls = %d", inner.embedCalls)
	}
}

func TestConcurrencyEmbeddingClientEmbedBatchGates(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 2}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	inner.batchFn = func([]string, model.ModelTarget) ([][]float32, error) {
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
		return [][]float32{{1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.EmbedBatch([]string{"x"}, model.ModelTarget{Id: "m"}); err != nil {
				t.Errorf("EmbedBatch err = %v", err)
			}
		}()
	}
	wg.Wait()
	if maxInFlight > 2 {
		t.Fatalf("max in-flight = %d, want <= 2", maxInFlight)
	}
	if inner.batchCalls != 6 {
		t.Fatalf("batchCalls = %d, want 6", inner.batchCalls)
	}
}

func TestConcurrencyEmbeddingClientCandidateLimitOverridesDefault(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 5}
	target := model.ModelTarget{Id: "m", Candidate: config.ModelCandidate{MaxConcurrency: 1}}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	inner.batchFn = func([]string, model.ModelTarget) ([][]float32, error) {
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
		return [][]float32{{1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.EmbedBatch([]string{"x"}, target)
		}()
	}
	wg.Wait()
	if maxInFlight > 1 {
		t.Fatalf("candidate limit 1 should cap in-flight, got %d", maxInFlight)
	}
}

func TestWithConcurrencyLimitPassthroughWhenDisabled(t *testing.T) {
	inner := &recordingEmbeddingClient{}
	clients := WithConcurrencyLimit([]EmbeddingClient{inner}, 0)
	if len(clients) != 1 || clients[0] != inner {
		t.Fatalf("zero default limit should pass through unchanged: %#v", clients)
	}
}

func TestConcurrencyEmbeddingClientEmbedBatchPropagatesError(t *testing.T) {
	inner := &recordingEmbeddingClient{
		batchFn: func([]string, model.ModelTarget) ([][]float32, error) {
			return nil, errors.New("upstream failure")
		},
	}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 2}
	if _, err := client.EmbedBatch([]string{"x"}, model.ModelTarget{Id: "m"}); err == nil {
		t.Fatal("expected upstream error to propagate through the gate")
	}
}

func TestConcurrencyEmbeddingClientFallbackKeyWhenTargetIDEmpty(t *testing.T) {
	// target.Id empty → falls back to "embedding"; concurrent calls share one slot
	inner := &recordingEmbeddingClient{}
	client := &concurrencyEmbeddingClient{inner: inner, gov: limiter.NewGovernor(), defaultLimit: 1}
	var mu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	inner.batchFn = func([]string, model.ModelTarget) ([][]float32, error) {
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
		return [][]float32{{1}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.EmbedBatch([]string{"x"}, model.ModelTarget{}); err != nil {
				t.Errorf("EmbedBatch err = %v", err)
			}
		}()
	}
	wg.Wait()
	if maxInFlight > 1 {
		t.Fatalf("fallback key should serialize on one slot, got %d in-flight", maxInFlight)
	}
}
