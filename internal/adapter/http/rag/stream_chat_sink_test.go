package rag

import (
	"context"
	"strings"
	"testing"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/stream"
)

type assertError string

func (e assertError) Error() string { return string(e) }

func streamSinkSSEBody(t *testing.T, fn func(s *streamChatSink)) string {
	t.Helper()
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	fn(s)
	events, _, err := m.GetEvents(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString("event: " + e.Name + "\n")
		sb.WriteString("data: " + string(e.Data) + "\n\n")
	}
	return sb.String()
}

func ragServiceRagChatMetaForTest() ragservice.RagChatMeta {
	return ragservice.RagChatMeta{ConversationID: "c1", TaskID: "t1"}
}

func ragServiceFinishForTest() ragservice.RagChatFinishPayload {
	return ragservice.RagChatFinishPayload{MessageID: "m1", Title: "t"}
}

func TestStreamChatSinkWireFormatParity(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		if err := s.SendMeta(ragServiceRagChatMetaForTest()); err != nil {
			t.Fatalf("meta: %v", err)
		}
		if err := s.SendThinking("ok"); err != nil {
			t.Fatalf("thinking: %v", err)
		}
		if err := s.SendMessage("hi"); err != nil {
			t.Fatalf("message: %v", err)
		}
		if err := s.SendTitle("t"); err != nil {
			t.Fatalf("title: %v", err)
		}
		if err := s.SendFinish(ragServiceFinishForTest()); err != nil {
			t.Fatalf("finish: %v", err)
		}
		if err := s.SendDone(); err != nil {
			t.Fatalf("done: %v", err)
		}
	})

	want := `event: meta
data: {"conversationId":"c1","taskId":"t1"}

event: message
data: {"delta":"ok","type":"think"}

event: message
data: {"delta":"hi","type":"response"}

event: title
data: {"title":"t"}

event: finish
data: {"messageId":"m1","title":"t"}

event: done
data: {}

`
	if body != want {
		t.Fatalf("wire mismatch:\n--- got ---\n%s--- want ---\n%s", body, want)
	}
}

func TestStreamChatSinkTerminalSemantics(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	s := &streamChatSink{manager: m, streamID: "s1"}
	_ = s.SendError(assertError("boom"))
	_ = s.SendDone()
	events, _, _ := m.GetEvents(context.Background(), "s1", 0)
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Name != "error" || events[0].Done {
		t.Fatalf("error event must be non-terminal, got %+v", events[0])
	}
	if events[1].Name != "done" || !events[1].Done {
		t.Fatalf("done event must be terminal, got %+v", events[1])
	}
	if string(events[0].Data) != `{"error":"boom"}` {
		t.Fatalf("error payload = %s", events[0].Data)
	}
	s2 := &streamChatSink{manager: m, streamID: "s2"}
	_ = s2.SendCancel(ragServiceFinishForTest())
	_ = s2.SendDone()
	events2, _, _ := m.GetEvents(context.Background(), "s2", 0)
	if len(events2) != 2 || events2[0].Name != "cancel" || events2[0].Done {
		t.Fatalf("cancel event must be non-terminal, got %+v", events2)
	}
	if events2[1].Name != "done" || !events2[1].Done {
		t.Fatalf("done event must be terminal, got %+v", events2[1])
	}
}

func TestStreamChatSinkAgentOutcomeEmitsTwoEvents(t *testing.T) {
	body := streamSinkSSEBody(t, func(s *streamChatSink) {
		if err := s.SendAgentOutcome(ragservice.RagChatAgentOutcomePayload{
			Status:       "awaiting_approval",
			Interrupted:  true,
			CheckpointID: "cp-1",
		}); err != nil {
			t.Fatalf("outcome: %v", err)
		}
	})
	if !strings.Contains(body, "event: agent_outcome\n") || !strings.Contains(body, "event: agent_status\n") {
		t.Fatalf("expected agent_outcome + agent_status events, got: %s", body)
	}
	if !strings.Contains(body, `"status":"awaiting_approval"`) || !strings.Contains(body, `"checkpointId":"cp-1"`) {
		t.Fatalf("outcome payload missing fields: %s", body)
	}
	if !strings.Contains(body, `"type":"outcome"`) {
		t.Fatalf("agent_status projection missing type: %s", body)
	}
}
