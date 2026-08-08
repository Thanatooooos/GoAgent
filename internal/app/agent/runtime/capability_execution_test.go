package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

type stubCapabilityHandle struct {
	spec   agentcapability.Spec
	result agentcapability.InvocationResult
	calls  int
}

func (h *stubCapabilityHandle) Spec() agentcapability.Spec {
	return h.spec
}

func (h *stubCapabilityHandle) Invoke(_ context.Context, _ agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	h.calls++
	return h.result, nil
}

func TestExecuteScheduledCapability_EmitsSharedCapabilityEvents(t *testing.T) {
	handle := &stubCapabilityHandle{
		spec: agentcapability.Spec{
			Name:             agentcapability.NameWebFetch,
			SupportsParallel: true,
			SupportsResume:   true,
			Idempotency:      agentcapability.IdempotencyBestEffort,
		},
		result: agentcapability.InvocationResult{
			Action: agentcapability.ActionRecord{
				Summary: "fetch https://example.com/doc",
			},
			Observation: agentcapability.ObservationRecord{
				Summary: "fetched 1 page",
			},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					Notes: []string{"fetched 1 page"},
				},
			},
			Status: agentcapability.StatusSucceeded,
		},
	}

	result, err := ExecuteScheduledCapability(context.Background(), CapabilityExecutionRequest{
		Session: &RuntimeSession{
			SessionID: "sess-execute-capability",
			Snapshot: agentstate.StateSnapshot{
				Request: agentstate.RequestState{
					RuntimeOptions: agentstate.RuntimeOptions{},
				},
			},
		},
		Node:          "fetch",
		PatternAction: "reactive_fetch",
		Handle:        handle,
		Input:         map[string]any{"urls": []string{"https://example.com/doc"}},
		StartSummary:  "fallback start",
		ResultSummary: "fallback result",
	})
	if err != nil {
		t.Fatalf("ExecuteScheduledCapability() error = %v", err)
	}

	if handle.calls != 1 {
		t.Fatalf("expected one capability invocation, got %d", handle.calls)
	}
	if result.Schedule.Decision != ScheduleDecisionExecute || result.Schedule.PatternAction != "reactive_fetch" {
		t.Fatalf("expected execute schedule with pattern action, got %+v", result.Schedule)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected start/result events, got %+v", result.Events)
	}
	if result.Events[0].EventType != agentstate.EventTypeCapabilityStart || result.Events[0].Node != "fetch" {
		t.Fatalf("expected capability_start event, got %+v", result.Events[0])
	}
	if result.Events[1].EventType != agentstate.EventTypeCapabilityResult || result.Events[1].PayloadText != "fetched 1 page" {
		t.Fatalf("expected capability_result event, got %+v", result.Events[1])
	}
}

func TestExecuteScheduledCapability_PreservesSchedulerDecisionMetadataWhenApprovalWouldBeRequired(t *testing.T) {
	handle := &stubCapabilityHandle{
		spec: agentcapability.Spec{
			Name:             agentcapability.NameWebFetch,
			RequiresApproval: true,
		},
		result: agentcapability.InvocationResult{
			Status: agentcapability.StatusSucceeded,
		},
	}

	result, err := ExecuteScheduledCapability(context.Background(), CapabilityExecutionRequest{
		Session: &RuntimeSession{
			SessionID: "sess-bypass-approval",
		},
		Node:          "fetch",
		PatternAction: "reactive_fetch",
		Handle:        handle,
		Input:         map[string]any{"urls": []string{"https://example.com/doc"}},
	})
	if err != nil {
		t.Fatalf("ExecuteScheduledCapability() error = %v", err)
	}
	if handle.calls != 0 {
		t.Fatalf("expected approval-gated capability not to execute, got %d calls", handle.calls)
	}
	if result.Schedule.Decision != ScheduleDecisionWaitApproval || result.Schedule.Reason != "approval_required" {
		t.Fatalf("expected scheduler metadata to preserve approval decision, got %+v", result.Schedule)
	}
	if result.Invocation.Status != agentcapability.StatusSkipped {
		t.Fatalf("expected approval-gated invocation to be skipped, got %+v", result.Invocation)
	}
}

func TestExecuteScheduledCapability_DegradesWithoutInvokingUnsupportedResumeCapability(t *testing.T) {
	handle := &stubCapabilityHandle{
		spec: agentcapability.Spec{
			Name:           agentcapability.NameWebFetch,
			SupportsResume: false,
		},
		result: agentcapability.InvocationResult{
			Status: agentcapability.StatusSucceeded,
		},
	}

	result, err := ExecuteScheduledCapability(context.Background(), CapabilityExecutionRequest{
		Session: &RuntimeSession{
			SessionID: "sess-resume-unsupported",
			Metadata: SessionMetadata{
				ResumeCount: 1,
			},
		},
		Node:          "fetch",
		PatternAction: "reactive_fetch",
		Handle:        handle,
		Input:         map[string]any{"urls": []string{"https://example.com/doc"}},
	})
	if err != nil {
		t.Fatalf("ExecuteScheduledCapability() error = %v", err)
	}
	if handle.calls != 0 {
		t.Fatalf("expected resume-unsupported capability not to execute, got %d calls", handle.calls)
	}
	if result.Schedule.Decision != ScheduleDecisionDegrade || result.Schedule.Reason != "resume_not_supported" {
		t.Fatalf("expected degrade schedule metadata, got %+v", result.Schedule)
	}
	if result.Invocation.Status != agentcapability.StatusDegraded {
		t.Fatalf("expected resume-unsupported invocation to degrade, got %+v", result.Invocation)
	}
}

func TestExecuteScheduledCapability_DoesNotInvokeCapabilityWhenPreconditionsFail(t *testing.T) {
	handle := &stubCapabilityHandle{
		spec: agentcapability.Spec{
			Name: agentcapability.NameWebFetch,
			Preconditions: []agentcapability.Precondition{
				{Field: "urls", Requirement: agentcapability.PreconditionRequirementNonEmpty},
			},
		},
		result: agentcapability.InvocationResult{
			Status: agentcapability.StatusSucceeded,
		},
	}

	result, err := ExecuteScheduledCapability(context.Background(), CapabilityExecutionRequest{
		Session: &RuntimeSession{
			SessionID: "sess-precondition-fail",
		},
		Node:          "fetch",
		PatternAction: "reactive_fetch",
		Handle:        handle,
		Input:         map[string]any{"urls": []string{}},
	})
	if err != nil {
		t.Fatalf("ExecuteScheduledCapability() error = %v", err)
	}
	if handle.calls != 0 {
		t.Fatalf("expected precondition failure to bypass capability invocation, got %d calls", handle.calls)
	}
	if result.Schedule.Decision != ScheduleDecisionFail || result.Schedule.Reason != "precondition_failed" {
		t.Fatalf("expected normalized precondition failure decision, got %+v", result.Schedule)
	}
	if result.Invocation.Status != agentcapability.StatusSkipped || result.Invocation.ErrorClass != agentcapability.ErrorClassValidation {
		t.Fatalf("expected skipped validation invocation result, got %+v", result.Invocation)
	}
	if len(result.Events) != 1 || result.Events[0].EventType != agentstate.EventTypeFailed {
		t.Fatalf("expected failed event for scheduler precondition rejection, got %+v", result.Events)
	}
}

func TestExecuteScheduledCapabilities_RunsParallelSafeBatchConcurrentlyAndPreservesResultOrder(t *testing.T) {
	gate := newParallelGate(2)
	first := &parallelCapabilityHandle{
		spec: agentcapability.Spec{
			Name:             agentcapability.NameWebSearch,
			SupportsParallel: true,
			SupportsResume:   true,
		},
		result: agentcapability.InvocationResult{
			Action:      agentcapability.ActionRecord{Summary: "search first"},
			Observation: agentcapability.ObservationRecord{Summary: "first result"},
			Status:      agentcapability.StatusSucceeded,
		},
		gate: gate,
	}
	second := &parallelCapabilityHandle{
		spec: agentcapability.Spec{
			Name:             agentcapability.NameWebFetch,
			SupportsParallel: true,
			SupportsResume:   true,
		},
		result: agentcapability.InvocationResult{
			Action:      agentcapability.ActionRecord{Summary: "fetch second"},
			Observation: agentcapability.ObservationRecord{Summary: "second result"},
			Status:      agentcapability.StatusSucceeded,
		},
		gate: gate,
	}

	type batchResult struct {
		results []CapabilityExecutionResult
		err     error
	}
	done := make(chan batchResult, 1)
	go func() {
		results, err := ExecuteScheduledCapabilities(context.Background(), []CapabilityExecutionRequest{
			{
				Session: &RuntimeSession{SessionID: "sess-batch"},
				Node:    "search",
				Handle:  first,
				Input:   map[string]any{"query": "first"},
			},
			{
				Session: &RuntimeSession{SessionID: "sess-batch"},
				Node:    "fetch",
				Handle:  second,
				Input:   map[string]any{"urls": []string{"https://example.com/doc"}},
			},
		})
		done <- batchResult{results: results, err: err}
	}()

	select {
	case <-gate.started:
	case <-time.After(250 * time.Millisecond):
		close(gate.release)
		t.Fatal("expected parallel-safe batch to execute concurrently")
	}
	close(gate.release)

	outcome := <-done
	if outcome.err != nil {
		t.Fatalf("ExecuteScheduledCapabilities() error = %v", outcome.err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both capabilities to execute once, got first=%d second=%d", first.calls, second.calls)
	}
	if len(outcome.results) != 2 {
		t.Fatalf("expected two ordered results, got %d", len(outcome.results))
	}
	if outcome.results[0].Invocation.Observation.Summary != "first result" {
		t.Fatalf("expected first input result to remain first, got %+v", outcome.results[0])
	}
	if outcome.results[1].Invocation.Observation.Summary != "second result" {
		t.Fatalf("expected second input result to remain second, got %+v", outcome.results[1])
	}
}

type parallelCapabilityHandle struct {
	spec   agentcapability.Spec
	result agentcapability.InvocationResult
	gate   *parallelGate
	calls  int
}

func (h *parallelCapabilityHandle) Spec() agentcapability.Spec {
	return h.spec
}

func (h *parallelCapabilityHandle) Invoke(_ context.Context, _ agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	h.calls++
	h.gate.enter()
	<-h.gate.release
	return h.result, nil
}

type parallelGate struct {
	target  int
	started chan struct{}
	release chan struct{}

	mu     sync.Mutex
	active int
	once   sync.Once
}

func newParallelGate(target int) *parallelGate {
	return &parallelGate{
		target:  target,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (g *parallelGate) enter() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active++
	if g.active >= g.target {
		g.once.Do(func() {
			close(g.started)
		})
	}
}
