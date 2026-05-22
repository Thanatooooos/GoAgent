package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

func cloneAgentState(state AgentState) AgentState {
	normalized := state.Normalize()
	return AgentState{
		Phase:         normalized.Phase,
		Hypothesis:    normalized.Hypothesis,
		Confidence:    normalized.Confidence,
		OpenQuestions: append([]string(nil), normalized.OpenQuestions...),
		CheckedTools:  append([]string(nil), normalized.CheckedTools...),
		NextHintCalls: append([]HintCall(nil), normalized.NextHintCalls...),
	}.Normalize()
}

func normalizeObservationState(observation ObserveResult, previous AgentState) AgentState {
	state := observation.State.Normalize()
	if state.Empty() {
		state = cloneAgentState(previous)
	}
	if observation.Done {
		state.NextHintCalls = nil
		state.NextHint = ""
		if strings.TrimSpace(state.Phase) == "" {
			state.Phase = "complete"
		}
	} else if strings.TrimSpace(state.Phase) == "" {
		state.Phase = "deep_dive"
	}
	return state.Normalize()
}
