package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// CapabilityExecutionRequest is the shared runtime entrypoint for one
// scheduler-controlled capability invocation.
type CapabilityExecutionRequest struct {
	Session             *RuntimeSession
	Node                string
	PatternAction       string
	Handle              agentcapability.Handle
	Input               any
	Metadata            map[string]any
	SkipInputValidation bool
	StartSummary        string
	ResultSummary       string
	EmitStartOnSkip     bool
}

// CapabilityExecutionResult captures the normalized scheduler decision,
// invocation result, and emitted runtime events for one capability call.
type CapabilityExecutionResult struct {
	Schedule   CapabilityScheduleResult         `json:"schedule"`
	Invocation agentcapability.InvocationResult `json:"invocation"`
	Events     []agentstate.RuntimeEvent        `json:"events,omitempty"`
	StartedAt  time.Time                        `json:"started_at"`
}

func ExecuteScheduledCapability(ctx context.Context, req CapabilityExecutionRequest) (CapabilityExecutionResult, error) {
	if req.Handle == nil {
		return CapabilityExecutionResult{}, fmt.Errorf("capability handle is required")
	}

	spec := req.Handle.Spec()
	snapshot := snapshotForCapabilityExecution(req.Session)
	schedule := EvaluateCapabilitySchedule(CapabilityScheduleInput{
		RuntimeOptions:      runtimeOptionsForCapabilityExecution(req.Session),
		Snapshot:            snapshot,
		PatternAction:       req.PatternAction,
		Session:             req.Session,
		Spec:                spec,
		Input:               req.Input,
		SkipInputValidation: req.SkipInputValidation,
	})
	result := CapabilityExecutionResult{
		Schedule: schedule,
	}
	startedAt := time.Now()
	invocation, err := req.Handle.Invoke(ctx, agentcapability.InvocationRequest{
		SessionID: sessionID(req.Session),
		Input:     req.Input,
		Snapshot:  snapshot,
		Metadata:  cloneExecutionMetadata(req.Metadata),
	})
	if err != nil {
		return result, err
	}

	result.Invocation = invocation
	result.StartedAt = startedAt
	result.Events = buildCapabilityExecutionEvents(
		sessionID(req.Session),
		req.Node,
		startedAt,
		invocation,
		req.StartSummary,
		req.ResultSummary,
		req.EmitStartOnSkip,
	)
	return result, nil
}

func buildCapabilityExecutionEvents(sessionID string, node string, startedAt time.Time, invocation agentcapability.InvocationResult, startSummary string, resultSummary string, emitStartOnSkip bool) []agentstate.RuntimeEvent {
	events := make([]agentstate.RuntimeEvent, 0, 2)
	if invocation.Status != agentcapability.StatusSkipped || emitStartOnSkip {
		events = append(events, agentstate.NewRuntimeEventAt(
			startedAt,
			sessionID,
			node,
			agentstate.EventTypeCapabilityStart,
			firstNonEmpty(strings.TrimSpace(invocation.Action.Summary), strings.TrimSpace(startSummary)),
		))
	}
	eventType := agentstate.EventTypeCapabilityResult
	if invocation.Status == agentcapability.StatusSkipped {
		eventType = agentstate.EventTypeCapabilitySkipped
	}
	events = append(events, agentstate.NewRuntimeEvent(
		sessionID,
		node,
		eventType,
		firstNonEmpty(strings.TrimSpace(invocation.Observation.Summary), strings.TrimSpace(resultSummary)),
	))
	return events
}

func snapshotForCapabilityExecution(session *RuntimeSession) agentstate.StateSnapshot {
	if session == nil {
		return agentstate.NormalizeSnapshot(agentstate.StateSnapshot{})
	}
	return agentstate.CloneSnapshot(session.Snapshot)
}

func runtimeOptionsForCapabilityExecution(session *RuntimeSession) agentstate.RuntimeOptions {
	if session == nil {
		return agentstate.RuntimeOptions{}
	}
	if session.Snapshot.Request.RuntimeOptions != (agentstate.RuntimeOptions{}) {
		return session.Snapshot.Request.RuntimeOptions
	}
	return session.Request.Options
}

func cloneExecutionMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
