package rag

import (
	"context"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/stream"
)

// runtimeStreamSink projects durable runtime facts onto the established chat
// SSE wire format. It does not persist anything itself: Runtime has already
// appended the journal entry before calling Append.
type runtimeStreamSink struct{ stream *streamChatSink }

func newRuntimeStreamSink(manager stream.StreamManager, streamID string) *runtimeStreamSink {
	return &runtimeStreamSink{stream: &streamChatSink{manager: manager, streamID: streamID}}
}

func (s *runtimeStreamSink) Append(_ context.Context, entry conversationruntime.JournalEntry) error {
	if s == nil || s.stream == nil || s.stream.Terminated() {
		return nil
	}
	switch entry.EventType {
	case conversationruntime.EventThinkingDelta:
		return s.stream.SendThinking(entry.Detail)
	case conversationruntime.EventAnswerDelta:
		return s.stream.SendMessage(entry.Detail)
	case conversationruntime.EventToolPending:
		return s.stream.SendTool(chatToolCall{CallID: entry.ToolCallID, Name: entry.ToolName, Status: "pending"})
	case conversationruntime.EventToolExecuting:
		return s.stream.SendTool(chatToolCall{CallID: entry.ToolCallID, Name: entry.ToolName, Status: "running"})
	case conversationruntime.EventToolSettled:
		content, value := splitToolDetail(entry.Detail)
		return s.stream.SendTool(chatToolCall{CallID: entry.ToolCallID, Name: entry.ToolName, Status: entry.ToolState, Summary: content, Data: value})
	case conversationruntime.EventInterrupted:
		if err := s.stream.append("error", gin.H{"error": entry.Detail, "status": "interrupted", "runtimeEventId": entry.ID}, false); err != nil {
			return err
		}
		return s.stream.SendDone()
	case conversationruntime.EventFailed:
		if err := s.stream.append("error", gin.H{"error": entry.Detail, "status": "failed", "runtimeEventId": entry.ID}, false); err != nil {
			return err
		}
		return s.stream.SendDone()
	case conversationruntime.EventCancelled:
		if err := s.stream.append("cancel", gin.H{"runtimeEventId": entry.ID}, false); err != nil {
			return err
		}
		return s.stream.SendDone()
	case conversationruntime.EventCompleted:
		var finish struct {
			MessageID string                 `json:"messageId"`
			Title     string                 `json:"title"`
			Content   *string                `json:"content"`
			Sources   []domain.MessageSource `json:"sources"`
		}
		if entry.Detail != "" && json.Unmarshal([]byte(entry.Detail), &finish) == nil && finish.MessageID != "" {
			if err := s.stream.SendFinish(chatFinishPayload{MessageID: finish.MessageID, Title: finish.Title, Content: finish.Content, Sources: finish.Sources}); err != nil {
				return err
			}
		}
		return s.stream.SendDone()
	default:
		return nil
	}
}

var _ conversationruntime.EventSink = (*runtimeStreamSink)(nil)

// splitToolDetail unwraps the durable {content, value} result ToolRunner writes
// for a settled call. A settled failure records the bare error instead, so it
// is shown as the summary with no structured value.
func splitToolDetail(detail string) (string, json.RawMessage) {
	var payload struct {
		Content string          `json:"content"`
		Value   json.RawMessage `json:"value"`
	}
	if json.Unmarshal([]byte(detail), &payload) == nil && (payload.Content != "" || len(payload.Value) > 0) {
		return payload.Content, payload.Value
	}
	return detail, nil
}
