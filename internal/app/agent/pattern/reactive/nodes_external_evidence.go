package reactive

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
)

func newExternalEvidenceNode(workflowCapability agentcapability.Handle) (agentkernel.Node, error) {
	if workflowCapability == nil {
		return nil, fmt.Errorf("external evidence capability is required")
	}
	return agentkernel.NewNodeFunc("external_evidence", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := normalizeQuery(session)
		execution, err := agentruntime.ExecuteScheduledCapability(ctx, agentruntime.CapabilityExecutionRequest{
			Session:         session,
			Node:            "external_evidence",
			PatternAction:   "reactive_external_evidence",
			Handle:          workflowCapability,
			Input:           agentexternal.CapabilityInput{Query: query},
			StartSummary:    query,
			ResultSummary:   query,
			EmitStartOnSkip: true,
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		if execution.Invocation.Status == agentcapability.StatusSkipped {
			execution.Events[len(execution.Events)-1].PayloadText = firstNonEmpty(
				execution.Invocation.Observation.Summary,
				strings.TrimSpace(execution.Invocation.ErrorClass),
			)
		}

		return agentruntime.NodeResult{
			Events: execution.Events,
			Delta:  withExecutionNodeDelta(execution.Invocation.Delta, "external_evidence"),
		}, nil
	})
}
