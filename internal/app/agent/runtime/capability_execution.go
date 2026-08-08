package runtime

import (
	"context"
	"fmt"
	"sync"
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
	results, err := ExecuteScheduledCapabilities(ctx, []CapabilityExecutionRequest{req})
	if err != nil {
		return CapabilityExecutionResult{}, err
	}
	if len(results) == 0 {
		return CapabilityExecutionResult{}, nil
	}
	return results[0], nil
}

func ExecuteScheduledCapabilities(ctx context.Context, reqs []CapabilityExecutionRequest) ([]CapabilityExecutionResult, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	inputs := make([]CapabilityScheduleInput, len(reqs))
	for i, req := range reqs {
		if req.Handle == nil {
			return nil, fmt.Errorf("capability handle is required")
		}
		inputs[i] = CapabilityScheduleInput{
			RuntimeOptions:      runtimeOptionsForCapabilityExecution(req.Session),
			Snapshot:            snapshotForCapabilityExecution(req.Session),
			PatternAction:       req.PatternAction,
			Session:             req.Session,
			Spec:                req.Handle.Spec(),
			Input:               req.Input,
			SkipInputValidation: req.SkipInputValidation,
		}
	}

	batches := BuildCapabilityScheduleBatches(inputs)
	results := make([]CapabilityExecutionResult, len(reqs))
	cursor := 0
	for _, batch := range batches {
		if batch.Decision == ScheduleDecisionExecute && batch.Parallel {
			if err := executeParallelCapabilityBatch(ctx, reqs[cursor:cursor+len(batch.Results)], batch.Results, results[cursor:cursor+len(batch.Results)]); err != nil {
				return results, err
			}
			cursor += len(batch.Results)
			continue
		}
		for offset, schedule := range batch.Results {
			result, err := executeScheduledCapabilityRequest(ctx, reqs[cursor+offset], schedule)
			if err != nil {
				return results, err
			}
			results[cursor+offset] = result
		}
		cursor += len(batch.Results)
	}
	return results, nil
}

func executeParallelCapabilityBatch(ctx context.Context, reqs []CapabilityExecutionRequest, schedules []CapabilityScheduleResult, results []CapabilityExecutionResult) error {
	var (
		wg       sync.WaitGroup
		firstErr error
		errMu    sync.Mutex
	)
	for i := range reqs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := executeScheduledCapabilityRequest(ctx, reqs[index], schedules[index])
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			results[index] = result
		}(i)
	}
	wg.Wait()
	return firstErr
}

func executeScheduledCapabilityRequest(ctx context.Context, req CapabilityExecutionRequest, schedule CapabilityScheduleResult) (CapabilityExecutionResult, error) {
	if req.Handle == nil {
		return CapabilityExecutionResult{}, fmt.Errorf("capability handle is required")
	}

	snapshot := snapshotForCapabilityExecution(req.Session)
	result := CapabilityExecutionResult{
		Schedule: schedule,
	}
	startedAt := time.Now()
	result.StartedAt = startedAt
	if schedule.Decision != ScheduleDecisionExecute {
		result.Invocation = buildScheduledInvocation(schedule)
		result.Events = buildScheduledCapabilityEvents(
			sessionID(req.Session),
			req.Node,
			startedAt,
			schedule,
			result.Invocation,
			req.ResultSummary,
		)
		return result, nil
	}
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

func buildScheduledInvocation(schedule CapabilityScheduleResult) agentcapability.InvocationResult {
	errorClass := scheduledCapabilityErrorClass(schedule.ErrorClass)
	observation := agentcapability.ObservationRecord{
		Summary:    strings.TrimSpace(schedule.Reason),
		ErrorClass: errorClass,
	}
	switch schedule.Decision {
	case ScheduleDecisionWaitApproval:
		return agentcapability.InvocationResult{
			Observation: observation,
			Status:      agentcapability.StatusSkipped,
			ErrorClass:  errorClass,
		}
	case ScheduleDecisionDegrade:
		observation.Degraded = true
		return agentcapability.InvocationResult{
			Observation: observation,
			Status:      agentcapability.StatusDegraded,
			ErrorClass:  errorClass,
		}
	case ScheduleDecisionFail, ScheduleDecisionSkip, ScheduleDecisionRetry:
		return agentcapability.InvocationResult{
			Observation: observation,
			Status:      agentcapability.StatusSkipped,
			ErrorClass:  errorClass,
		}
	default:
		return agentcapability.InvocationResult{}
	}
}

func buildScheduledCapabilityEvents(sessionID string, node string, startedAt time.Time, schedule CapabilityScheduleResult, invocation agentcapability.InvocationResult, resultSummary string) []agentstate.RuntimeEvent {
	payload := firstNonEmpty(strings.TrimSpace(invocation.Observation.Summary), strings.TrimSpace(resultSummary), strings.TrimSpace(schedule.Reason))
	eventType := agentstate.EventTypeCapabilitySkipped
	switch schedule.Decision {
	case ScheduleDecisionDegrade:
		eventType = agentstate.EventTypeDegraded
	case ScheduleDecisionFail:
		eventType = agentstate.EventTypeFailed
	}
	return []agentstate.RuntimeEvent{
		agentstate.NewRuntimeEventAt(startedAt, sessionID, node, eventType, payload),
	}
}

func scheduledCapabilityErrorClass(errorClass string) string {
	switch strings.TrimSpace(errorClass) {
	case ErrorClassValidation:
		return agentcapability.ErrorClassValidation
	case ErrorClassPermission:
		return agentcapability.ErrorClassPermission
	case ErrorClassDependency:
		return agentcapability.ErrorClassDependency
	case ErrorClassExternal:
		return agentcapability.ErrorClassExternal
	default:
		return ""
	}
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
