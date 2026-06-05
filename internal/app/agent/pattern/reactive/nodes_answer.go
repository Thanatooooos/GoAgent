package reactive

import (
	"context"
	"time"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newHandoffNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("handoff", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		note := "handoff ready: grounded evidence bundle available for rag chat answer synthesis"
		return agentruntime.NodeResult{
			Events: []agentstate.RuntimeEvent{
				agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "handoff", agentstate.EventTypeHandoffFinalized, note),
			},
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					Notes: []string{note},
				},
				Execution: executionTerminalDelta("handoff"),
			},
		}, nil
	})
}

func newAnswerNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("answer", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		final := buildAnswer(session)
		return agentruntime.NodeResult{
			Events: []agentstate.RuntimeEvent{
				agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "answer", agentstate.EventTypeAnswerFinalized, final),
			},
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final: &final,
				},
				Execution: executionTerminalDelta("answer"),
			},
		}, nil
	})
}

func newDegradeNode() (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("degrade", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		reason := session.Snapshot.Evidence.SufficiencyReason
		if reason == "" {
			reason = "insufficient_evidence"
		}
		final := "I couldn't gather enough reliable fetched evidence to answer confidently."
		return agentruntime.NodeResult{
			Events: []agentstate.RuntimeEvent{
				agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "degrade", agentstate.EventTypeDegraded, reason),
			},
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final:         &final,
					DegradeReason: &reason,
				},
				Execution: executionTerminalDelta("degrade"),
			},
		}, nil
	})
}

func buildAnswer(session *agentruntime.RuntimeSession) string {
	if session == nil || len(session.Snapshot.Evidence.Items) == 0 {
		return "I found supporting evidence, but the final answer content is unavailable."
	}
	return "Based on fetched evidence: " + session.Snapshot.Evidence.Items[0].Content
}

func appendNonEmpty(values []string, candidate string) []string {
	if candidate == "" {
		return values
	}
	return append(values, candidate)
}
