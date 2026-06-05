package reactive

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newExternalEvidenceNode(workflowCapability agentcapability.Handle) (agentkernel.Node, error) {
	if workflowCapability == nil {
		return nil, fmt.Errorf("external evidence capability is required")
	}
	return agentkernel.NewNodeFunc("external_evidence", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := normalizeQuery(session)
		startedAt := time.Now()
		result, err := workflowCapability.Invoke(ctx, agentcapability.InvocationRequest{
			SessionID: session.SessionID,
			Snapshot:  session.Snapshot,
			Input:     agentexternal.CapabilityInput{Query: query},
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}

		events := []agentstate.RuntimeEvent{
			agentstate.NewRuntimeEventAt(startedAt, session.SessionID, "external_evidence", agentstate.EventTypeCapabilityStart, capabilityActionSummary(result.Action, query)),
		}
		eventType := agentstate.EventTypeCapabilityResult
		if result.Status == agentcapability.StatusSkipped {
			eventType = agentstate.EventTypeCapabilitySkipped
		}
		events = append(events, agentstate.NewRuntimeEvent(session.SessionID, "external_evidence", eventType, capabilityObservationSummary(result.Observation, strings.TrimSpace(result.ErrorClass))))

		return agentruntime.NodeResult{
			Events: events,
			Delta:  withExecutionNodeDelta(result.Delta, "external_evidence"),
		}, nil
	})
}
