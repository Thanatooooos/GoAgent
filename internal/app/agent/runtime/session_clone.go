package runtime

import agentstate "local/rag-project/internal/app/agent/state"

func cloneRequestEnvelope(request RequestEnvelope) RequestEnvelope {
	cloned := request
	cloned.KnowledgeBases = append([]string(nil), request.KnowledgeBases...)
	return cloned
}

func cloneSnapshot(snapshot agentstate.StateSnapshot) agentstate.StateSnapshot {
	return agentstate.CloneSnapshot(snapshot)
}

func cloneRuntimeCheckpoint(ref *CheckpointRef) *CheckpointRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
}

func cloneRuntimeEvents(events []agentstate.RuntimeEvent) []agentstate.RuntimeEvent {
	if len(events) == 0 {
		return nil
	}
	cloned := make([]agentstate.RuntimeEvent, len(events))
	for idx, event := range events {
		cloned[idx] = event
		cloned[idx].EvidenceRef = append([]agentstate.EvidenceRef(nil), event.EvidenceRef...)
		if event.Decision != nil {
			decision := *event.Decision
			cloned[idx].Decision = &decision
		}
		if event.Checkpoint != nil {
			checkpoint := *event.Checkpoint
			cloned[idx].Checkpoint = &checkpoint
		}
		if event.Delta != nil {
			delta := agentstate.CloneDelta(*event.Delta)
			cloned[idx].Delta = &delta
		}
	}
	return cloned
}
