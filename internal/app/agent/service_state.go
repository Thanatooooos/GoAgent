package agent

import (
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func (s *Service) applySessionDelta(session *agentruntime.RuntimeSession, node string, delta agentstate.StateDelta, now time.Time) error {
	if session == nil {
		return nil
	}
	reducer := s.reducer
	if reducer == nil {
		reducer = agentstate.DefaultReducer{}
	}
	nextSnapshot, err := reducer.Apply(session.Snapshot, delta)
	if err != nil {
		return err
	}
	session.Snapshot = nextSnapshot
	session.Metadata.UpdatedAt = now
	event := agentstate.NewRuntimeEventAt(now, session.SessionID, node, agentstate.EventTypeStateApplied, "")
	cloned := agentstate.CloneDelta(delta)
	event.Delta = &cloned
	appendRuntimeEvent(session, event)
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
