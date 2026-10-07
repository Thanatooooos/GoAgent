package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"local/rag-project/internal/app/runtime/persistence"
)

func TestLifecycleCreatesOneSessionPerAcceptedMessage(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	request := validRunRequest()

	first, err := lifecycle.StartSession(context.Background(), request)
	if err != nil {
		t.Fatalf("start first session: %v", err)
	}
	second, err := lifecycle.StartSession(context.Background(), request)
	if err != nil {
		t.Fatalf("start duplicate session: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate message created sessions %q and %q", first.ID, second.ID)
	}
}

func TestLifecyclePersistsToolStatesInStrictOrder(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)

	if _, err := lifecycle.BeginTool(context.Background(), session, "call-1", "retrieve_knowledge", "query"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := lifecycle.StartTool(context.Background(), session, "call-1", "retrieve_knowledge"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := lifecycle.SettleTool(context.Background(), session, "call-1", "retrieve_knowledge", ToolStateCompleted, "done", nil); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if _, err := lifecycle.StartTool(context.Background(), session, "call-1", "retrieve_knowledge"); err == nil {
		t.Fatal("terminal tool call must not restart")
	}

	entries := store.entries(session.ID)
	if len(entries) != 3 {
		t.Fatalf("journal entries = %d, want 3", len(entries))
	}
	for i, want := range []string{ToolStatePending, ToolStateExecuting, ToolStateCompleted} {
		if entries[i].Sequence != int64(i+1) || entries[i].ToolState != want {
			t.Fatalf("entry %d = sequence %d state %q", i, entries[i].Sequence, entries[i].ToolState)
		}
	}
}

func TestLifecycleRecoveryFailsUnsettledToolWithoutReexecution(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)
	if _, err := lifecycle.BeginTool(context.Background(), session, "call-1", "web_search", "query"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := lifecycle.StartTool(context.Background(), session, "call-1", "web_search"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := lifecycle.RecoverUnsettledTools(context.Background(), session); err != nil {
		t.Fatalf("recover: %v", err)
	}

	entries := store.entries(session.ID)
	if got := entries[len(entries)-1].ToolState; got != ToolStateFailed {
		t.Fatalf("recovered state = %q, want failed", got)
	}
	if got := len(entries); got != 3 {
		t.Fatalf("recovery added %d entries, want 3", got)
	}
}

func validRunRequest() RunRequest {
	return RunRequest{ConversationID: "conversation-1", UserID: "user-1", UserMessageID: "message-1", Question: "hello", TraceID: "trace-1"}
}
func mustStartSession(t *testing.T, lifecycle *Lifecycle) persistence.Session {
	t.Helper()
	session, err := lifecycle.StartSession(context.Background(), validRunRequest())
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	return session
}
func sequentialIDs() IDFactory {
	var next int
	return func() (string, error) { next++; return fmt.Sprintf("id-%d", next), nil }
}

type memoryStore struct {
	mu        sync.Mutex
	sessions  map[string]persistence.Session
	byMessage map[string]string
	journal   map[string][]JournalEntry
}

func newMemoryStore() *memoryStore {
	return &memoryStore{sessions: map[string]persistence.Session{}, byMessage: map[string]string{}, journal: map[string][]JournalEntry{}}
}
func (s *memoryStore) CreateOrLoadSession(_ context.Context, session persistence.Session) (persistence.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := session.ConversationID + ":" + session.UserMessageID
	if id := s.byMessage[key]; id != "" {
		return s.sessions[id], nil
	}
	s.sessions[session.ID] = session
	s.byMessage[key] = session.ID
	return session, nil
}
func (s *memoryStore) FindSessionByTraceID(_ context.Context, userID, traceID string) (persistence.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if session.UserID == userID && session.TraceID == traceID {
			return session, nil
		}
	}
	return persistence.Session{}, nil
}
func (s *memoryStore) SetStatus(_ context.Context, sessionID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || session.Status != persistence.StatusRunning {
		return fmt.Errorf("running session not found")
	}
	session.Status = status
	s.sessions[sessionID] = session
	return nil
}
func (s *memoryStore) Append(_ context.Context, entry JournalEntry) (JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.journal[entry.RuntimeSessionID]
	entry.Sequence = int64(len(entries) + 1)
	s.journal[entry.RuntimeSessionID] = append(entries, entry)
	return entry, nil
}
func (s *memoryStore) TransitionTool(_ context.Context, entry JournalEntry, allowedFrom []string) (JournalEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.journal[entry.RuntimeSessionID]
	from := ""
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ToolCallID == entry.ToolCallID && entries[i].ToolState != "" {
			from = entries[i].ToolState
			break
		}
	}
	if !containsState(allowedFrom, from) {
		return JournalEntry{}, fmt.Errorf("invalid tool state transition %q -> %q", from, entry.ToolState)
	}
	entry.Sequence = int64(len(entries) + 1)
	s.journal[entry.RuntimeSessionID] = append(entries, entry)
	return entry, nil
}
func (s *memoryStore) ListJournal(_ context.Context, sessionID string) ([]JournalEntry, error) {
	return s.entries(sessionID), nil
}
func containsState(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func (s *memoryStore) ListUnsettledToolCalls(_ context.Context, sessionID string) ([]persistence.UnsettledToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	latest := map[string]JournalEntry{}
	for _, entry := range s.journal[sessionID] {
		if entry.ToolCallID != "" && entry.ToolState != "" {
			latest[entry.ToolCallID] = entry
		}
	}
	var result []persistence.UnsettledToolCall
	for _, entry := range latest {
		if entry.ToolState == ToolStatePending || entry.ToolState == ToolStateExecuting {
			result = append(result, persistence.UnsettledToolCall{ToolCallID: entry.ToolCallID, ToolName: entry.ToolName, State: entry.ToolState})
		}
	}
	return result, nil
}
func (s *memoryStore) entries(sessionID string) []JournalEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]JournalEntry(nil), s.journal[sessionID]...)
}
