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
	if hasState(session.InitialSnapshot) {
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

func hasState(snapshot agentstate.StateSnapshot) bool {
	return snapshot.Request.Question != "" ||
		snapshot.Request.UserID != "" ||
		snapshot.Request.TraceID != "" ||
		snapshot.Request.ConversationID != "" ||
		len(snapshot.Request.KnowledgeBaseIDs) > 0 ||
		snapshot.Request.RuntimeOptions != (agentstate.RuntimeOptions{}) ||
		snapshot.Context.RewrittenQuery != "" ||
		snapshot.Context.SearchQuery != "" ||
		snapshot.Context.SearchProvider != "" ||
		snapshot.Context.SearchProviderActual != "" ||
		len(snapshot.Context.SearchResults) > 0 ||
		len(snapshot.Context.FetchResults) > 0 ||
		len(snapshot.Context.PreferredURLs) > 0 ||
		len(snapshot.Context.AvoidURLs) > 0 ||
		len(snapshot.Context.SeenURLs) > 0 ||
		len(snapshot.Context.MemoryRefs) > 0 ||
		len(snapshot.Context.Notes) > 0 ||
		len(snapshot.Evidence.Items) > 0 ||
		snapshot.Evidence.Sufficient ||
		snapshot.Evidence.SufficiencyReason != "" ||
		snapshot.Evidence.NewItemsThisRound != 0 ||
		len(snapshot.Evidence.OpenQuestions) > 0 ||
		snapshot.Approval.Status != "" ||
		snapshot.Approval.Reason != "" ||
		snapshot.Approval.Node != "" ||
		snapshot.Approval.Capability != "" ||
		snapshot.Approval.CheckpointID != "" ||
		snapshot.Approval.RerunNode != "" ||
		!snapshot.Approval.RequestedAt.IsZero() ||
		!snapshot.Approval.ReviewedAt.IsZero() ||
		snapshot.Approval.DecisionNote != "" ||
		snapshot.Execution.CurrentNode != "" ||
		snapshot.Execution.Iteration != 0 ||
		snapshot.Execution.MaxIterations != 0 ||
		snapshot.Execution.ContinueCount != 0 ||
		snapshot.Execution.LastBranchTarget != "" ||
		snapshot.Execution.LastBranchReason != "" ||
		snapshot.Execution.LastProgressKind != "" ||
		snapshot.Execution.LastNewURLCount != 0 ||
		snapshot.Execution.LastNewEvidenceCount != 0 ||
		snapshot.Execution.ConsecutiveNoProgressRounds != 0 ||
		len(snapshot.Execution.ScheduledActions) > 0 ||
		len(snapshot.Execution.CompletedActions) > 0 ||
		len(snapshot.Execution.FailedActions) > 0 ||
		snapshot.Execution.Interrupted ||
		snapshot.Execution.InterruptReason != "" ||
		snapshot.Answer.Draft != "" ||
		snapshot.Answer.DegradeReason != "" ||
		snapshot.Answer.Final != ""
}
