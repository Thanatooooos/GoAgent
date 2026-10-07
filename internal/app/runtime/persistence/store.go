// Package persistence owns runtime's durable session and journal boundary.
// Implementations belong to adapters; runtime itself only depends on Store.
package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const StatusRunning = "running"

// Session is the durable identity and sequence owner for one accepted user
// message. Its journal is the source of truth for runtime execution facts.
type Session struct {
	ExecutionManaged bool
	ID               string
	ConversationID   string
	UserMessageID    string
	UserID           string
	TraceID          string
	Status           string
	NextSequence     int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s Session) Validate() error {
	switch {
	case strings.TrimSpace(s.ID) == "":
		return fmt.Errorf("runtime session id is required")
	case strings.TrimSpace(s.ConversationID) == "":
		return fmt.Errorf("conversation id is required")
	case strings.TrimSpace(s.UserMessageID) == "":
		return fmt.Errorf("user message id is required")
	case strings.TrimSpace(s.UserID) == "":
		return fmt.Errorf("user id is required")
	case strings.TrimSpace(s.TraceID) == "":
		return fmt.Errorf("trace id is required")
	case s.Status != StatusRunning:
		return fmt.Errorf("new runtime session must be %q", StatusRunning)
	default:
		return nil
	}
}

// UnsettledToolCall is a projection of the latest journal state for one tool
// call. Recovery changes it to failed; it never re-runs the invocation.
type UnsettledToolCall struct {
	ToolCallID string
	ToolName   string
	State      string
}

// EvidenceRef is a durable reference to a fact accepted by runtime.
type EvidenceRef struct {
	ID         string
	Kind       string
	SourceID   string
	DocumentID string
	URL        string
}

// JournalEntry is an append-only execution fact. Persistence allocates its
// Sequence; runtime supplies the semantic event data.
type JournalEntry struct {
	ID               string
	RuntimeSessionID string
	Sequence         int64
	ConversationID   string
	UserMessageID    string
	TraceID          string
	EventType        string
	ToolCallID       string
	ToolName         string
	ToolState        string
	Evidence         []EvidenceRef
	Detail           string
	CreatedAt        time.Time
}

// Store is runtime's durable execution port. Append allocates the next session
// sequence atomically; callers must not manufacture their own ordering.
type Store interface {
	CreateOrLoadSession(context.Context, Session) (Session, error)
	FindSessionByTraceID(context.Context, string, string) (Session, error)
	SetStatus(ctx context.Context, runtimeSessionID, status string) error
	Append(context.Context, JournalEntry) (JournalEntry, error)
	TransitionTool(ctx context.Context, entry JournalEntry, allowedFrom []string) (JournalEntry, error)
	ListJournal(ctx context.Context, runtimeSessionID string) ([]JournalEntry, error)
	ListUnsettledToolCalls(ctx context.Context, runtimeSessionID string) ([]UnsettledToolCall, error)
}
