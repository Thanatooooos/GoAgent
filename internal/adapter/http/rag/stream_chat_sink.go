package rag

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/stream"
)

type chatStreamMeta struct {
	ConversationID string `json:"conversationId"`
	TaskID         string `json:"taskId"`
}

type chatFinishPayload struct {
	MessageID string                 `json:"messageId"`
	Title     string                 `json:"title"`
	Content   *string                `json:"content,omitempty"`
	Sources   []domain.MessageSource `json:"sources,omitempty"`
}

// streamChatSink writes runtime facts to the durable SSE stream buffer.
type streamChatSink struct {
	manager    stream.StreamManager
	streamID   string
	seq        int64
	terminated bool
}

func (s *streamChatSink) append(name string, payload interface{}, done bool) error {
	if s == nil || s.manager == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	s.seq++
	if err := s.manager.AppendEvent(context.Background(), s.streamID, stream.StreamEvent{
		ID: strconv.FormatInt(s.seq, 10), Name: name, Data: data, Done: done, Timestamp: time.Now(),
	}); err != nil {
		log.Printf("stream chat sink append %q: %v", s.streamID, err)
		return err
	}
	if done {
		s.terminated = true
	}
	return nil
}

func (s *streamChatSink) Terminated() bool { return s != nil && s.terminated }

func (s *streamChatSink) SendMeta(meta chatStreamMeta) error { return s.append("meta", meta, false) }

func (s *streamChatSink) SendThinking(delta string) error {
	return s.append("message", gin.H{"type": "think", "delta": delta}, false)
}

func (s *streamChatSink) SendMessage(delta string) error {
	return s.append("message", gin.H{"type": "response", "delta": delta}, false)
}

// chatToolCall is the wire shape of one "tool" SSE event. CallID lets the
// client merge pending, running, and settled into a single row, and Data
// carries the structured result of a settled tool so the UI can render it.
type chatToolCall struct {
	CallID  string
	Name    string
	Status  string
	Summary string
	Data    json.RawMessage
}

// toolSummaryLimit keeps a settled result's encoded JSON from flooding the
// collapsed tool row; the structured data field still carries the full value.
const toolSummaryLimit = 400

func (s *streamChatSink) SendTool(call chatToolCall) error {
	originalName := strings.TrimSpace(call.Name)
	friendlyName := friendlyToolDisplayName(originalName)
	payload := gin.H{"name": friendlyName, "status": call.Status}
	if callID := strings.TrimSpace(call.CallID); callID != "" {
		payload["callId"] = callID
	}
	if friendlyName != originalName {
		payload["originalName"] = originalName
	}
	if summary := strings.TrimSpace(call.Summary); summary != "" {
		payload["summary"] = truncateToolSummary(summary, toolSummaryLimit)
	}
	if len(call.Data) > 0 {
		payload["data"] = call.Data
	}
	return s.append("tool", payload, false)
}

func truncateToolSummary(summary string, limit int) string {
	runes := []rune(summary)
	if limit <= 0 || len(runes) <= limit {
		return summary
	}
	return string(runes[:limit]) + "…"
}

func friendlyToolDisplayName(name string) string {
	switch strings.TrimSpace(name) {
	case "retrieve_knowledge":
		return "知识库检索"
	case "web_search":
		return "联网搜索"
	case "web_fetch":
		return "网页内容抓取"
	case "create_scheduled_task":
		return "创建定时任务"
	case "list_scheduled_tasks":
		return "查询定时任务"
	case "pause_scheduled_task":
		return "暂停定时任务"
	default:
		return name
	}
}

func (s *streamChatSink) SendFinish(payload chatFinishPayload) error {
	finish := gin.H{"messageId": payload.MessageID, "title": payload.Title}
	if payload.Content != nil {
		finish["content"] = *payload.Content
	}
	if len(payload.Sources) > 0 {
		finish["sources"] = payload.Sources
	}
	return s.append("finish", finish, false)
}

func (s *streamChatSink) SendError(err error) error {
	if err == nil {
		return nil
	}
	return s.append("error", gin.H{"error": err.Error()}, false)
}

func (s *streamChatSink) SendDone() error { return s.append("done", gin.H{}, true) }
