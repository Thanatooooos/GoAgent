package reactive

import (
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

func withExecutionNodeDelta(delta agentstate.StateDelta, node string) agentstate.StateDelta {
	delta.Execution = executionNodeDelta(node)
	return delta
}

func capabilityActionSummary(action agentcapability.ActionRecord, fallback string) string {
	if trimmed := strings.TrimSpace(action.Summary); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func capabilityObservationSummary(observation agentcapability.ObservationRecord, fallback string) string {
	if trimmed := strings.TrimSpace(observation.Summary); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}
