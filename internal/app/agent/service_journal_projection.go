package agent

import agentstate "local/rag-project/internal/app/agent/state"

func cloneJournal(events []agentstate.RuntimeEvent) []agentstate.RuntimeEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]agentstate.RuntimeEvent, len(events))
	copy(cloned, events)
	return cloned
}
