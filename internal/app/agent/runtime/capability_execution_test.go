package runtime

import (
	"context"
	"testing"

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
	if handle.calls != 1 {
		t.Fatalf("expected capability invocation to proceed through shared runtime path, got %d calls", handle.calls)
	}
	if result.Schedule.Decision != ScheduleDecisionWaitApproval || result.Schedule.Reason != "approval_required" {
		t.Fatalf("expected scheduler metadata to preserve approval decision, got %+v", result.Schedule)
	}
}
