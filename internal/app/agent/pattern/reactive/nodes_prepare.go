package reactive

import (
	"context"
	"fmt"
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"

	agentkernel "local/rag-project/internal/app/agent/kernel"
)

func newPrepareNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("prepare", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		query := normalizeQuery(session)
		if query == "" {
			return agentruntime.NodeResult{}, fmt.Errorf("question is required")
		}
		nodeName := "prepare"
		note := "prepared reactive search query"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					RewrittenQuery: &query,
					SearchQuery:    &query,
					Notes:          []string{note},
				},
				Execution: executionNodeDelta(nodeName),
			},
		}, nil
	})
}

func normalizeQuery(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(session.Snapshot.Context.SearchQuery); trimmed != "" {
		return strings.Join(strings.Fields(trimmed), " ")
	}
	if trimmed := strings.TrimSpace(session.Snapshot.Context.RewrittenQuery); trimmed != "" {
		return strings.Join(strings.Fields(trimmed), " ")
	}
	if trimmed := strings.TrimSpace(session.Request.Question); trimmed != "" {
		return strings.Join(strings.Fields(trimmed), " ")
	}
	return strings.Join(strings.Fields(strings.TrimSpace(session.Snapshot.Request.Question)), " ")
}
