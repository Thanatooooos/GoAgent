package rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"local/rag-project/internal/framework/stream"
)

type assertError string

func (e assertError) Error() string { return string(e) }

func streamSinkSSEBody(t *testing.T, fn func(*streamChatSink)) string {
	t.Helper()
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	fn(s)
	events, _, err := m.GetEvents(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	var body strings.Builder
	for _, event := range events {
		body.WriteString("event: " + event.Name + "\n")
		body.WriteString("data: " + string(event.Data) + "\n\n")
	}
	return body.String()
}

func TestStreamChatSinkWireFormat(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		_ = s.SendMeta(chatStreamMeta{ConversationID: "c1", TaskID: "t1"})
		_ = s.SendThinking("ok")
		_ = s.SendMessage("hi")
		_ = s.SendFinish(chatFinishPayload{MessageID: "m1", Title: "t"})
		_ = s.SendDone()
	})
	for _, want := range []string{"event: meta", `"conversationId":"c1"`, `"type":"think"`, `"type":"response"`, "event: finish", "event: done"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestStreamChatSinkToolUsesFriendlyName(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		_ = s.SendTool(chatToolCall{CallID: "call-1", Name: "web_search", Status: "running"})
	})
	for _, want := range []string{`"name":"联网搜索"`, `"originalName":"web_search"`, `"callId":"call-1"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestStreamChatSinkCarriesStructuredToolResult(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		_ = s.SendTool(chatToolCall{CallID: "call-2", Name: "create_scheduled_task", Status: "completed",
			Summary: "已生成定时任务草稿", Data: json.RawMessage(`{"draft":{"id":"d1"}}`)})
	})
	for _, want := range []string{`"name":"创建定时任务"`, `"data":{"draft":{"id":"d1"}}`, `"summary":"已生成定时任务草稿"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestStreamChatSinkTerminalSemantics(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	_ = s.SendError(assertError("boom"))
	_ = s.SendDone()
	events, _, _ := m.GetEvents(context.Background(), "s1", 0)
	if len(events) != 2 || events[0].Done || !events[1].Done {
		t.Fatalf("terminal events = %+v", events)
	}
}
