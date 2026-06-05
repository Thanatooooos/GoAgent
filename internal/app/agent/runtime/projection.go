package runtime

import (
	"fmt"

	agentstate "local/rag-project/internal/app/agent/state"
)

// ProjectionPoint captures the projected state immediately after one journal event.
type ProjectionPoint struct {
	Sequence  int                      `json:"sequence,omitempty"`
	Node      string                   `json:"node,omitempty"`
	EventType string                   `json:"event_type,omitempty"`
	Snapshot  agentstate.StateSnapshot `json:"snapshot"`
}

// ProjectSnapshotAt rebuilds the runtime snapshot at the supplied event offset.
func ProjectSnapshotAt(session *RuntimeSession, sequence int, reducer agentstate.Reducer) (agentstate.StateSnapshot, error) {
	if session == nil {
		return agentstate.StateSnapshot{}, fmt.Errorf("runtime session is required")
	}

	base := projectionBaseSnapshot(session)
	if sequence <= 0 {
		return base, nil
	}

	r := projectionReducer(reducer)
	current := agentstate.CloneSnapshot(base)
	for _, event := range session.Journal {
		if event.Sequence > sequence {
			break
		}
		if event.EventType != agentstate.EventTypeStateApplied || event.Delta == nil {
			continue
		}
		next, err := r.Apply(current, agentstate.CloneDelta(*event.Delta))
		if err != nil {
			return agentstate.StateSnapshot{}, fmt.Errorf("apply projected state at sequence %d: %w", event.Sequence, err)
		}
		current = next
	}
	return current, nil
}

// BuildProjectionTimeline rebuilds state after every journal event.
func BuildProjectionTimeline(session *RuntimeSession, reducer agentstate.Reducer) ([]ProjectionPoint, error) {
	if session == nil {
		return nil, fmt.Errorf("runtime session is required")
	}

	r := projectionReducer(reducer)
	current := projectionBaseSnapshot(session)
	points := make([]ProjectionPoint, 0, len(session.Journal))
	for _, event := range session.Journal {
		if event.EventType == agentstate.EventTypeStateApplied && event.Delta != nil {
			next, err := r.Apply(current, agentstate.CloneDelta(*event.Delta))
			if err != nil {
				return nil, fmt.Errorf("apply projected state at sequence %d: %w", event.Sequence, err)
			}
			current = next
		}
		points = append(points, ProjectionPoint{
			Sequence:  event.Sequence,
			Node:      event.Node,
			EventType: event.EventType,
			Snapshot:  agentstate.CloneSnapshot(current),
		})
	}
	return points, nil
}

func projectionBaseSnapshot(session *RuntimeSession) agentstate.StateSnapshot {
	if session == nil {
		return agentstate.StateSnapshot{}
	}
	if agentstate.HasContent(session.InitialSnapshot) {
		return agentstate.CloneSnapshot(session.InitialSnapshot)
	}
	return agentstate.CloneSnapshot(session.Snapshot)
}

func projectionReducer(reducer agentstate.Reducer) agentstate.Reducer {
	if reducer != nil {
		return reducer
	}
	return agentstate.DefaultReducer{}
}
