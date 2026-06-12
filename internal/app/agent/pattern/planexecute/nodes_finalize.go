package planexecute

import (
	"context"
	"time"

	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newFinalizeNode(configuredOutputMode string) (agentkernel.Node, error) {
	return agentkernel.NewNodeFunc("finalize", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		_ = ctx
		mode := outputMode(session, configuredOutputMode)
		degradeReason := session.Snapshot.Answer.DegradeReason
		if session.Snapshot.Plan.Status == agentstate.PlanStatusDegraded || degradeReason != "" || !session.Snapshot.Evidence.Sufficient {
			if degradeReason == "" {
				degradeReason = firstNonEmpty(session.Snapshot.Plan.LastAssessment, reasonPlanFailed)
			}
			final := "I couldn't gather enough reliable fetched evidence to answer confidently."
			logFinalizedOutput(session, mode, final, degradeReason)
			return agentruntime.NodeResult{
				Events: []agentstate.RuntimeEvent{
					agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "finalize", agentstate.EventTypeDegraded, degradeReason),
				},
				Delta: agentstate.StateDelta{
					Answer: &agentstate.AnswerDelta{
						Final:         stringPtr(final),
						DegradeReason: stringPtr(degradeReason),
					},
					Execution: executionTerminalDelta(),
				},
			}, nil
		}

		if mode == agentstate.OutputModeHandoff {
			note := "handoff ready: explicit plan completed with grounded evidence"
			logFinalizedOutput(session, mode, note, "")
			return agentruntime.NodeResult{
				Events: []agentstate.RuntimeEvent{
					agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "finalize", agentstate.EventTypeHandoffFinalized, note),
				},
				Delta: agentstate.StateDelta{
					Context: &agentstate.ContextDelta{
						Notes: []string{note},
					},
					Execution: executionTerminalDelta(),
				},
			}, nil
		}

		final := buildFinalAnswer(session)
		logFinalizedOutput(session, mode, final, "")
		return agentruntime.NodeResult{
			Events: []agentstate.RuntimeEvent{
				agentstate.NewRuntimeEventAt(time.Now(), session.SessionID, "finalize", agentstate.EventTypeAnswerFinalized, final),
			},
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final: stringPtr(final),
				},
				Execution: executionTerminalDelta(),
			},
		}, nil
	})
}
