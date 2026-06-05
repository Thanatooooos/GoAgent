package reactive

import (
	"context"
	"fmt"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newSearchNode(searchCapability agentcapability.Handle) (agentkernel.Node, error) {
	if searchCapability == nil {
		return nil, fmt.Errorf("search capability is required")
	}
	return agentkernel.NewNodeFunc("search", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := normalizeQuery(session)
		startedAt := time.Now()
		result, err := searchCapability.Invoke(ctx, agentcapability.InvocationRequest{
			SessionID: session.SessionID,
			Snapshot:  session.Snapshot,
			Input:     agentsearch.CapabilityInput{Query: query},
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		output, ok := result.Output.(agentsearch.SearchOutput)
		if !ok {
			return agentruntime.NodeResult{}, fmt.Errorf("search capability returned unexpected output type %T", result.Output)
		}

		return agentruntime.NodeResult{
			Events: []agentstate.RuntimeEvent{
				agentstate.NewRuntimeEventAt(startedAt, session.SessionID, "search", agentstate.EventTypeCapabilityStart, capabilityActionSummary(result.Action, query)),
				agentstate.NewRuntimeEvent(session.SessionID, "search", agentstate.EventTypeCapabilityResult, capabilityObservationSummary(result.Observation, output.Summary)),
			},
			Delta: withExecutionNodeDelta(result.Delta, "search"),
		}, nil
	})
}
