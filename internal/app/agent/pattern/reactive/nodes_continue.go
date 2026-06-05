package reactive

import (
	"context"
	"strings"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

const defaultMaxIterations = 2

func newContinueNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("continue", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		contextDelta := continueContextDelta(session)
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Context:   contextDelta,
				Execution: executionContinueDelta(),
			},
		}, nil
	})
}

func continueContextDelta(session *agentruntime.RuntimeSession) *agentstate.ContextDelta {
	nextQuery := normalizeWhitespace(sessionQuery(session))
	baseQuery := normalizeWhitespace(basePreparedQuery(session))

	delta := &agentstate.ContextDelta{
		Notes:            []string{"continuing reactive loop for another evidence pass"},
		SearchErrorClass: stringPtr(""),
		FetchErrorClass:  stringPtr(""),
	}
	if nextQuery != "" && baseQuery != "" && nextQuery != baseQuery {
		delta.ResetSearchResults = true
		delta.ResetFetchResults = true
		delta.SearchProvider = stringPtr("")
		delta.SearchProviderActual = stringPtr("")
		delta.Notes = []string{"continuing reactive loop with a refined search query"}
	}
	return delta
}

func sessionQuery(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	return session.Snapshot.Context.SearchQuery
}

func basePreparedQuery(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	return firstNonEmpty(session.Snapshot.Context.RewrittenQuery, session.Snapshot.Request.Question)
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
