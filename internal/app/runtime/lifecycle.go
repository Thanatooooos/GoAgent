package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/runtime/persistence"
)

type IDFactory func() (string, error)

// Lifecycle makes the tool journal ordering executable. It deliberately owns
// no tool registry or model loop yet; those arrive only after the durable
// semantics are in place.
type Lifecycle struct {
	store persistence.Store
	ids   IDFactory
	now   func() time.Time
}

func NewLifecycle(store persistence.Store, ids IDFactory) *Lifecycle {
	return &Lifecycle{store: store, ids: ids, now: time.Now}
}

func (l *Lifecycle) StartSession(ctx context.Context, request RunRequest) (persistence.Session, error) {
	if l == nil || l.store == nil || l.ids == nil {
		return persistence.Session{}, fmt.Errorf("runtime lifecycle is not configured")
	}
	if err := request.Validate(); err != nil {
		return persistence.Session{}, err
	}
	id, err := l.ids()
	if err != nil {
		return persistence.Session{}, fmt.Errorf("generate runtime session id: %w", err)
	}
	if strings.TrimSpace(id) == "" {
		return persistence.Session{}, fmt.Errorf("generated runtime session id is empty")
	}
	now := l.now().UTC()
	return l.store.CreateOrLoadSession(ctx, persistence.Session{
		ExecutionManaged: request.publishAnswer,
		ID:               id,
		ConversationID:   request.ConversationID,
		UserMessageID:    request.UserMessageID,
		UserID:           request.UserID,
		TraceID:          request.TraceID,
		Status:           persistence.StatusRunning,
		NextSequence:     1,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (l *Lifecycle) FinishSession(ctx context.Context, session persistence.Session, status string) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("runtime lifecycle is not configured")
	}
	switch status {
	case StatusCompleted, StatusDegraded, StatusFailed, StatusCancelled:
	default:
		return fmt.Errorf("invalid runtime terminal status %q", status)
	}
	if err := l.store.SetStatus(ctx, session.ID, status); err != nil {
		return fmt.Errorf("set runtime session status: %w", err)
	}
	return nil
}

func (l *Lifecycle) BeginTool(ctx context.Context, session persistence.Session, toolCallID, toolName, detail string) (JournalEntry, error) {
	if err := requireToolCall(session, toolCallID, toolName); err != nil {
		return JournalEntry{}, err
	}
	return l.transition(ctx, session, JournalEntry{
		EventType:  EventToolPending,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		ToolState:  ToolStatePending,
		Detail:     detail,
	}, []string{""})
}

func (l *Lifecycle) StartTool(ctx context.Context, session persistence.Session, toolCallID, toolName string) (JournalEntry, error) {
	return l.transitionTool(ctx, session, toolCallID, toolName, ToolStateExecuting, "")
}

func (l *Lifecycle) SettleTool(ctx context.Context, session persistence.Session, toolCallID, toolName, state, detail string, evidence []EvidenceRef) (JournalEntry, error) {
	if state != ToolStateCompleted && state != ToolStateDenied && state != ToolStateFailed {
		return JournalEntry{}, fmt.Errorf("invalid terminal tool state %q", state)
	}
	return l.transitionTool(ctx, session, toolCallID, toolName, state, detail, evidence)
}

// RecoverUnsettledTools records interruption as a terminal failure. Because
// tools may grow side effects in the future, recovery never invokes them again.
func (l *Lifecycle) RecoverUnsettledTools(ctx context.Context, session persistence.Session) error {
	if l == nil || l.store == nil {
		return fmt.Errorf("runtime lifecycle is not configured")
	}
	calls, err := l.store.ListUnsettledToolCalls(ctx, session.ID)
	if err != nil {
		return fmt.Errorf("list unsettled tool calls: %w", err)
	}
	for _, call := range calls {
		if _, err := l.transitionTool(ctx, session, call.ToolCallID, call.ToolName, ToolStateFailed, "runtime interrupted before tool call settled"); err != nil {
			return fmt.Errorf("recover tool call %q: %w", call.ToolCallID, err)
		}
	}
	return nil
}

func (l *Lifecycle) transitionTool(ctx context.Context, session persistence.Session, toolCallID, toolName, to, detail string, evidence ...[]EvidenceRef) (JournalEntry, error) {
	if err := requireToolCall(session, toolCallID, toolName); err != nil {
		return JournalEntry{}, err
	}
	if l == nil || l.store == nil {
		return JournalEntry{}, fmt.Errorf("runtime lifecycle is not configured")
	}
	var refs []EvidenceRef
	if len(evidence) > 0 {
		refs = evidence[0]
	}
	eventType := EventToolSettled
	if to == ToolStateExecuting {
		eventType = EventToolExecuting
	}
	return l.transition(ctx, session, JournalEntry{
		EventType:  eventType,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		ToolState:  to,
		Detail:     detail,
		Evidence:   refs,
	}, allowedToolPredecessors(to))
}

func (l *Lifecycle) transition(ctx context.Context, session persistence.Session, entry JournalEntry, allowedFrom []string) (JournalEntry, error) {
	if l == nil || l.store == nil || l.ids == nil {
		return JournalEntry{}, fmt.Errorf("runtime lifecycle is not configured")
	}
	id, err := l.ids()
	if err != nil {
		return JournalEntry{}, fmt.Errorf("generate runtime journal id: %w", err)
	}
	entry.ID = id
	entry.RuntimeSessionID = session.ID
	entry.ConversationID = session.ConversationID
	entry.UserMessageID = session.UserMessageID
	entry.TraceID = session.TraceID
	entry.CreatedAt = l.now().UTC()
	result, err := l.store.TransitionTool(ctx, entry, allowedFrom)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("append runtime journal: %w", err)
	}
	return result, nil
}

func allowedToolPredecessors(to string) []string {
	switch to {
	case ToolStateExecuting:
		return []string{ToolStatePending}
	case ToolStateCompleted:
		return []string{ToolStateExecuting}
	case ToolStateDenied:
		return []string{ToolStatePending}
	case ToolStateFailed:
		return []string{ToolStatePending, ToolStateExecuting}
	default:
		return nil
	}
}

func requireToolCall(session persistence.Session, toolCallID, toolName string) error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("runtime session id is required")
	}
	if strings.TrimSpace(toolCallID) == "" {
		return fmt.Errorf("tool call id is required")
	}
	if strings.TrimSpace(toolName) == "" {
		return fmt.Errorf("tool name is required")
	}
	return nil
}
