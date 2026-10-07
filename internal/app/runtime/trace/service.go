// Package trace projects runtime sessions and their durable journals into the
// operator-facing trace view. It owns no execution state and writes nothing.
package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/persistence"
)

type Store interface {
	CountTraceSessions(context.Context, string, string, string) (int, error)
	ListTraceSessions(context.Context, string, string, string, int, int) ([]persistence.Session, error)
	FindTraceSession(context.Context, string) (persistence.Session, error)
	ListJournal(context.Context, string) ([]persistence.JournalEntry, error)
}

type PageInput struct {
	Page           int
	PageSize       int
	TraceID        string
	ConversationID string
	Status         string
}

type Page struct {
	Items    []Run
	Total    int
	Page     int
	PageSize int
}

type Run struct {
	RuntimeSessionID string     `json:"runtimeSessionId"`
	TraceID          string     `json:"traceId"`
	ConversationID   string     `json:"conversationId"`
	UserMessageID    string     `json:"userMessageId"`
	UserID           string     `json:"userId"`
	Status           string     `json:"status"`
	StartTime        time.Time  `json:"startTime"`
	EndTime          *time.Time `json:"endTime,omitempty"`
	DurationMs       *int64     `json:"durationMs,omitempty"`
	TurnCount        int        `json:"turnCount"`
	ToolCallCount    int        `json:"toolCallCount"`
	FirstThinkingAt  *time.Time `json:"firstThinkingAt,omitempty"`
	FirstContentAt   *time.Time `json:"firstContentAt,omitempty"`
	ErrorMessage     string     `json:"errorMessage,omitempty"`
}

type Span struct {
	ID            string     `json:"id"`
	ParentID      string     `json:"parentId,omitempty"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
	DurationMs    *int64     `json:"durationMs,omitempty"`
	Turn          int        `json:"turn,omitempty"`
	FinishReason  string     `json:"finishReason,omitempty"`
	ToolCallID    string     `json:"toolCallId,omitempty"`
	ToolName      string     `json:"toolName,omitempty"`
	EvidenceCount int        `json:"evidenceCount,omitempty"`
	ErrorClass    string     `json:"errorClass,omitempty"`
	InputSummary  string     `json:"inputSummary,omitempty"`
	ResultSummary string     `json:"resultSummary,omitempty"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
}

type Detail struct {
	Run   Run    `json:"run"`
	Spans []Span `json:"spans"`
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Page(ctx context.Context, input PageInput) (Page, error) {
	if s == nil || s.store == nil {
		return Page{}, fmt.Errorf("runtime trace store is required")
	}
	if input.Page <= 0 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = 10
	}
	total, err := s.store.CountTraceSessions(ctx, input.TraceID, input.ConversationID, input.Status)
	if err != nil {
		return Page{}, err
	}
	sessions, err := s.store.ListTraceSessions(ctx, input.TraceID, input.ConversationID, input.Status, (input.Page-1)*input.PageSize, input.PageSize)
	if err != nil {
		return Page{}, err
	}
	items := make([]Run, 0, len(sessions))
	for _, session := range sessions {
		entries, err := s.store.ListJournal(ctx, session.ID)
		if err != nil {
			return Page{}, err
		}
		items = append(items, project(session, entries).Run)
	}
	return Page{Items: items, Total: total, Page: input.Page, PageSize: input.PageSize}, nil
}

func (s *Service) Detail(ctx context.Context, traceID string) (Detail, error) {
	if s == nil || s.store == nil {
		return Detail{}, fmt.Errorf("runtime trace store is required")
	}
	session, err := s.store.FindTraceSession(ctx, strings.TrimSpace(traceID))
	if err != nil {
		return Detail{}, err
	}
	if session.ID == "" {
		return Detail{}, fmt.Errorf("runtime trace %q not found", traceID)
	}
	entries, err := s.store.ListJournal(ctx, session.ID)
	if err != nil {
		return Detail{}, err
	}
	return project(session, entries), nil
}

func project(session persistence.Session, entries []persistence.JournalEntry) Detail {
	run := Run{RuntimeSessionID: session.ID, TraceID: session.TraceID, ConversationID: session.ConversationID, UserMessageID: session.UserMessageID, UserID: session.UserID, Status: session.Status, StartTime: session.CreatedAt}
	if session.Status != persistence.StatusRunning {
		end := session.UpdatedAt
		run.EndTime = &end
		duration := end.Sub(session.CreatedAt).Milliseconds()
		run.DurationMs = &duration
	}
	spans := make([]Span, 0)
	turns := map[int]int{}
	tools := map[string]int{}
	compression := -1
	for _, entry := range entries {
		switch entry.EventType {
		case runtime.EventThinkingDelta:
			if run.FirstThinkingAt == nil {
				at := entry.CreatedAt
				run.FirstThinkingAt = &at
			}
		case runtime.EventAnswerDelta:
			if run.FirstContentAt == nil {
				at := entry.CreatedAt
				run.FirstContentAt = &at
			}
		case runtime.EventModelTurnStarted:
			var detail struct {
				Turn int `json:"turn"`
			}
			_ = json.Unmarshal([]byte(entry.Detail), &detail)
			if detail.Turn <= 0 {
				continue
			}
			turns[detail.Turn] = len(spans)
			spans = append(spans, Span{ID: fmt.Sprintf("model:%d", detail.Turn), Kind: "model_turn", Name: "model", Status: "running", StartTime: entry.CreatedAt, Turn: detail.Turn})
		case runtime.EventModelTurnFinished:
			var detail struct {
				Turn         int    `json:"turn"`
				Status       string `json:"status"`
				ErrorClass   string `json:"errorClass"`
				FinishReason string `json:"finishReason"`
			}
			_ = json.Unmarshal([]byte(entry.Detail), &detail)
			if index, ok := turns[detail.Turn]; ok {
				status := detail.Status
				if status == "" {
					status = "completed"
				}
				settleSpan(&spans[index], entry.CreatedAt, status, detail.ErrorClass, detail.FinishReason)
			}
		case runtime.EventHistoryCompressionStarted:
			compression = len(spans)
			spans = append(spans, Span{ID: "history_compression", Kind: "history_compression", Name: "history compression", Status: "running", StartTime: entry.CreatedAt})
		case runtime.EventHistoryCompressionFinished:
			if compression >= 0 {
				var detail struct {
					Status string `json:"status"`
				}
				_ = json.Unmarshal([]byte(entry.Detail), &detail)
				status := detail.Status
				if status == "" {
					status = "completed"
				}
				settleSpan(&spans[compression], entry.CreatedAt, status, "", "")
				compression = -1
			}
		case runtime.EventToolPending:
			tools[entry.ToolCallID] = len(spans)
			spans = append(spans, Span{ID: "tool:" + entry.ToolCallID, ParentID: latestTurnID(spans), Kind: "tool_call", Name: entry.ToolName, Status: entry.ToolState, StartTime: entry.CreatedAt, ToolCallID: entry.ToolCallID, ToolName: entry.ToolName, InputSummary: summarize(entry.Detail)})
		case runtime.EventToolExecuting:
			if index, ok := tools[entry.ToolCallID]; ok {
				spans[index].Status = entry.ToolState
			}
		case runtime.EventToolSettled:
			if index, ok := tools[entry.ToolCallID]; ok {
				settleSpan(&spans[index], entry.CreatedAt, entry.ToolState, errorClass(entry.ToolState, entry.Detail), "")
				spans[index].EvidenceCount = len(entry.Evidence)
				if entry.ToolState == runtime.ToolStateCompleted {
					spans[index].ResultSummary = summarize(entry.Detail)
				} else {
					spans[index].ErrorMessage = summarize(entry.Detail)
				}
			}
		case runtime.EventFailed, runtime.EventCancelled, runtime.EventInterrupted:
			run.ErrorMessage = summarize(entry.Detail)
		}
	}
	run.TurnCount, run.ToolCallCount = len(turns), len(tools)
	return Detail{Run: run, Spans: spans}
}

const summaryLimit = 480

func summarize(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) <= summaryLimit {
		return detail
	}
	return detail[:summaryLimit] + "…"
}

func latestTurnID(spans []Span) string {
	for index := len(spans) - 1; index >= 0; index-- {
		if spans[index].Kind == "model_turn" {
			return spans[index].ID
		}
	}
	return ""
}

func settleSpan(span *Span, end time.Time, status, errorClass, finishReason string) {
	span.Status, span.EndTime, span.ErrorClass, span.FinishReason = status, &end, errorClass, finishReason
	duration := end.Sub(span.StartTime).Milliseconds()
	span.DurationMs = &duration
}

func errorClass(state, detail string) string {
	if state != runtime.ToolStateFailed && state != runtime.ToolStateDenied {
		return ""
	}
	if state == runtime.ToolStateDenied {
		return "denied"
	}
	if strings.Contains(strings.ToLower(detail), "context canceled") {
		return "cancelled"
	}
	return "tool"
}
