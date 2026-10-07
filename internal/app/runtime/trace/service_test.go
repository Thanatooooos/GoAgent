package trace

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/persistence"
)

type storeStub struct {
	session persistence.Session
	entries []persistence.JournalEntry
}

func (s storeStub) CountTraceSessions(context.Context, string, string, string) (int, error) {
	return 1, nil
}
func (s storeStub) ListTraceSessions(context.Context, string, string, string, int, int) ([]persistence.Session, error) {
	return []persistence.Session{s.session}, nil
}
func (s storeStub) FindTraceSession(context.Context, string) (persistence.Session, error) {
	return s.session, nil
}
func (s storeStub) ListJournal(context.Context, string) ([]persistence.JournalEntry, error) {
	return s.entries, nil
}

func TestDetailProjectsModelTurnsAndToolsFromJournal(t *testing.T) {
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Second)
	service := NewService(storeStub{
		session: persistence.Session{ID: "s1", TraceID: "t1", ConversationID: "c1", UserMessageID: "m1", UserID: "u1", Status: runtime.StatusCompleted, CreatedAt: start, UpdatedAt: end},
		entries: []persistence.JournalEntry{
			{EventType: runtime.EventModelTurnStarted, Detail: `{"turn":1}`, CreatedAt: start.Add(time.Second)},
			{EventType: runtime.EventThinkingDelta, CreatedAt: start.Add(2 * time.Second)},
			{EventType: runtime.EventAnswerDelta, CreatedAt: start.Add(3 * time.Second)},
			{EventType: runtime.EventModelTurnFinished, Detail: `{"turn":1,"status":"completed","finishReason":"tool_calls"}`, CreatedAt: start.Add(4 * time.Second)},
			{EventType: runtime.EventToolPending, ToolCallID: "call-1", ToolName: "retrieve_knowledge", ToolState: runtime.ToolStatePending, Detail: `{"query":"release"}`, CreatedAt: start.Add(5 * time.Second)},
			{EventType: runtime.EventToolSettled, ToolCallID: "call-1", ToolName: "retrieve_knowledge", ToolState: runtime.ToolStateCompleted, Detail: `{"content":"evidence"}`, Evidence: []persistence.EvidenceRef{{ID: "e1"}}, CreatedAt: start.Add(7 * time.Second)},
		},
	})
	detail, err := service.Detail(context.Background(), "t1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Run.TurnCount != 1 || detail.Run.ToolCallCount != 1 {
		t.Fatalf("run counts = %+v", detail.Run)
	}
	if detail.Run.FirstThinkingAt == nil || detail.Run.FirstContentAt == nil {
		t.Fatalf("first token times = %+v", detail.Run)
	}
	if len(detail.Spans) != 2 {
		t.Fatalf("span count = %d", len(detail.Spans))
	}
	if detail.Spans[0].FinishReason != "tool_calls" || detail.Spans[0].DurationMs == nil {
		t.Fatalf("model span = %+v", detail.Spans[0])
	}
	if detail.Spans[1].ParentID != "model:1" || detail.Spans[1].EvidenceCount != 1 || detail.Spans[1].Status != runtime.ToolStateCompleted || detail.Spans[1].InputSummary == "" || detail.Spans[1].ResultSummary == "" {
		t.Fatalf("tool span = %+v", detail.Spans[1])
	}
}

func TestDetailProjectsHistoryCompressionSpan(t *testing.T) {
	start := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	service := NewService(storeStub{
		session: persistence.Session{ID: "s1", TraceID: "t1", ConversationID: "c1", UserMessageID: "m1", UserID: "u1", Status: runtime.StatusCompleted, CreatedAt: start, UpdatedAt: start.Add(time.Second)},
		entries: []persistence.JournalEntry{
			{EventType: runtime.EventHistoryCompressionStarted, Detail: `{"status":"running"}`, CreatedAt: start},
			{EventType: runtime.EventHistoryCompressionFinished, Detail: `{"status":"completed"}`, CreatedAt: start.Add(time.Second)},
		},
	})
	detail, err := service.Detail(context.Background(), "t1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Spans) != 1 || detail.Spans[0].Kind != "history_compression" || detail.Spans[0].Status != "completed" || detail.Spans[0].DurationMs == nil {
		t.Fatalf("compression span = %+v", detail.Spans)
	}
}
