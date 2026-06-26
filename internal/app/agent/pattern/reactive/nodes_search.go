package reactive

import (
	"context"
	"fmt"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
)

func newSearchNode(searchCapability agentcapability.Handle) (agentkernel.Node, error) {
	if searchCapability == nil {
		return nil, fmt.Errorf("search capability is required")
	}
	return agentkernel.NewNodeFunc("search", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := normalizeQuery(session)
		execution, err := agentruntime.ExecuteScheduledCapability(ctx, agentruntime.CapabilityExecutionRequest{
			Session:       session,
			Node:          "search",
			PatternAction: "reactive_search",
			Handle:        searchCapability,
			Input:         agentsearch.CapabilityInput{Query: query},
			StartSummary:  query,
			ResultSummary: query,
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		if _, ok := execution.Invocation.Output.(agentsearch.SearchOutput); !ok {
			return agentruntime.NodeResult{}, fmt.Errorf("search capability returned unexpected output type %T", execution.Invocation.Output)
		}

		return agentruntime.NodeResult{
			Events: execution.Events,
			Delta:  withExecutionNodeDelta(execution.Invocation.Delta, "search"),
		}, nil
	})
}
