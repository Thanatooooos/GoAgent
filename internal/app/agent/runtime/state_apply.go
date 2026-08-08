package runtime

import (
	"strings"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

type stateApplier struct {
	reducer agentstate.Reducer
}

func newStateApplier(reducer agentstate.Reducer) stateApplier {
	if reducer == nil {
		reducer = agentstate.DefaultReducer{}
	}
	return stateApplier{reducer: reducer}
}

func (a stateApplier) apply(session *RuntimeSession, node string, delta agentstate.StateDelta, appliedAt time.Time) error {
	if session == nil {
		return nil
	}
	nextSnapshot, err := a.reducer.Apply(session.Snapshot, delta)
	if err != nil {
		return err
	}
	session.Snapshot = nextSnapshot
	if appliedAt.IsZero() {
		appliedAt = time.Now()
	}
	session.Metadata.UpdatedAt = appliedAt

	event := agentstate.NewRuntimeEventAt(
		appliedAt,
		session.SessionID,
		strings.TrimSpace(node),
		agentstate.EventTypeStateApplied,
		"",
	)
	cloned := agentstate.CloneDelta(delta)
	event.Delta = &cloned
	appendRuntimeEvent(session, event)
	return nil
}
