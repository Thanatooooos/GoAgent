package runtime

import (
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

// RuntimeSession is the top-level container for one runtime execution.
type RuntimeSession struct {
	SessionID       string                    `json:"session_id"`
	Request         RequestEnvelope           `json:"request"`
	InitialSnapshot agentstate.StateSnapshot  `json:"initial_snapshot"`
	Snapshot        agentstate.StateSnapshot  `json:"snapshot"`
	Journal         []agentstate.RuntimeEvent `json:"journal,omitempty"`
	Checkpoint      *CheckpointRef            `json:"checkpoint,omitempty"`
	Metadata        SessionMetadata           `json:"metadata"`
}

// RequestEnvelope is the stable external input to a runtime session.
// It is the request-facing contract before data is projected into snapshot
// state domains.
type RequestEnvelope struct {
	Question       string                    `json:"question"`
	UserID         string                    `json:"user_id,omitempty"`
	TraceID        string                    `json:"trace_id,omitempty"`
	ConversationID string                    `json:"conversation_id,omitempty"`
	KnowledgeBases []string                  `json:"knowledge_bases,omitempty"`
	Options        agentstate.RuntimeOptions `json:"options"`
}

// CheckpointRef identifies the latest persisted checkpoint for the session
// without embedding the checkpoint payload itself into RuntimeSession.
type CheckpointRef struct {
	ID          string    `json:"id"`
	Node        string    `json:"node,omitempty"`
	EventOffset int       `json:"event_offset,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// SessionMetadata stores operational details about the current run rather than
// user-facing or evidence-bearing state.
type SessionMetadata struct {
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	ResumedFrom      string    `json:"resumed_from,omitempty"`
	ResumeCount      int       `json:"resume_count,omitempty"`
	RuntimeName      string    `json:"runtime_name,omitempty"`
	RuntimeVersion   string    `json:"runtime_version,omitempty"`
	ApprovalDecision string    `json:"approval_decision,omitempty"`
	ApprovalNote     string    `json:"approval_note,omitempty"`
}
