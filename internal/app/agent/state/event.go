package state

import "time"

const (
	EventTypeNodeStart         = "node_start"
	EventTypeNodeFinish        = "node_finish"
	EventTypeNodeError         = "node_error"
	EventTypeReducerError      = "reducer_error"
	EventTypeStateApplied      = "state_applied"
	EventTypeDecisionEmitted   = "decision_emitted"
	EventTypeBranchSelected    = "branch_selected"
	EventTypeCapabilityStart   = "capability_start"
	EventTypeCapabilityResult  = "capability_result"
	EventTypeCapabilitySkipped = "capability_skipped"
	EventTypeHandoffFinalized  = "handoff_finalized"
	EventTypeAnswerFinalized   = "answer_finalized"
	EventTypeDegraded          = "degraded"
	EventTypeInterrupt         = "interrupt"
	EventTypeResumeCompleted   = "resume_completed"
)

// RuntimeEvent is the append-only event record stored in a session journal.
// M0 keeps it minimal so RuntimeSession can reference a concrete type.
type RuntimeEvent struct {
	ID          string         `json:"id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Sequence    int            `json:"sequence,omitempty"`
	Node        string         `json:"node,omitempty"`
	EventType   string         `json:"event_type,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	PayloadText string         `json:"payload_text,omitempty"`
	EvidenceRef []EvidenceRef  `json:"evidence_ref,omitempty"`
	Decision    *DecisionRef   `json:"decision,omitempty"`
	Checkpoint  *CheckpointRef `json:"checkpoint,omitempty"`
	Delta       *StateDelta    `json:"delta,omitempty"`
}

// EvidenceRef points from an event back to evidence already tracked in state.
type EvidenceRef struct {
	EvidenceID string `json:"evidence_id,omitempty"`
	SourceRef  string `json:"source_ref,omitempty"`
}

// DecisionRef stores the structured decision summary attached to a runtime event.
type DecisionRef struct {
	Kind       string  `json:"kind,omitempty"`
	Target     string  `json:"target,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// CheckpointRef stores checkpoint metadata attached to a runtime event.
type CheckpointRef struct {
	ID   string `json:"id,omitempty"`
	Node string `json:"node,omitempty"`
}

// NewRuntimeEventAt builds a runtime event with the supplied timestamp and
// summary payload. Sequence and ID are assigned when the event is appended.
func NewRuntimeEventAt(ts time.Time, sessionID, node, eventType, payloadText string) RuntimeEvent {
	return RuntimeEvent{
		SessionID:   sessionID,
		Node:        node,
		EventType:   eventType,
		Timestamp:   ts,
		PayloadText: payloadText,
	}
}

// NewRuntimeEvent builds a runtime event using the current wall-clock time.
func NewRuntimeEvent(sessionID, node, eventType, payloadText string) RuntimeEvent {
	return NewRuntimeEventAt(time.Now(), sessionID, node, eventType, payloadText)
}

// NewDecisionRef stores the structured decision summary attached to a runtime event.
func NewDecisionRef(kind, target string, confidence float64, reasoning string) *DecisionRef {
	return &DecisionRef{
		Kind:       kind,
		Target:     target,
		Confidence: confidence,
		Reasoning:  reasoning,
	}
}

// NewCheckpointRef stores checkpoint metadata attached to a runtime event.
func NewCheckpointRef(id, node string) *CheckpointRef {
	return &CheckpointRef{
		ID:   id,
		Node: node,
	}
}
