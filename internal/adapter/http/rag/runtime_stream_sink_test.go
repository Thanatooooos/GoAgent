package rag

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/stream"
)

func TestRuntimeStreamSinkPreservesChatWireEvents(t *testing.T) {
	m := stream.NewMemoryStreamManager()
	sink := newRuntimeStreamSink(m, "runtime-stream")
	entries := []conversationruntime.JournalEntry{
		{EventType: conversationruntime.EventThinkingDelta, Detail: "reason"},
		{EventType: conversationruntime.EventAnswerDelta, Detail: "answer"},
		{EventType: conversationruntime.EventToolExecuting, ToolName: "retrieve_knowledge"},
		{EventType: conversationruntime.EventToolSettled, ToolName: "retrieve_knowledge", ToolState: conversationruntime.ToolStateCompleted, Detail: "found evidence"},
		{EventType: conversationruntime.EventCompleted, Detail: `{"messageId":"m1","sources":[{"type":"kb","title":"Guide","chunkId":"chunk-1"}]}`},
	}
	for _, entry := range entries {
		if err := sink.Append(context.Background(), entry); err != nil {
			t.Fatal(err)
		}
	}
	events, _, err := m.GetEvents(context.Background(), "runtime-stream", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 || events[0].Name != "message" || events[1].Name != "message" || events[2].Name != "tool" || events[3].Name != "tool" || events[4].Name != "finish" || events[5].Name != "done" || !events[5].Done {
		t.Fatalf("events = %#v", events)
	}
	if !strings.Contains(string(events[0].Data), `"type":"think"`) || !strings.Contains(string(events[1].Data), `"type":"response"`) {
		t.Fatalf("message events = %#v", events[:2])
	}
	if !strings.Contains(string(events[4].Data), `"chunkId":"chunk-1"`) {
		t.Fatalf("finish event missing sources: %s", events[4].Data)
	}
}

func TestRuntimeFinishCarriesCanonicalBodyAndSupportsOlderEvents(t *testing.T) {
	for _, detail := range []string{
		`{"messageId":"m1","content":"查询过程。\n\n完整结论。"}`,
		`{"messageId":"m1"}`,
	} {
		manager := stream.NewMemoryStreamManager()
		sink := newRuntimeStreamSink(manager, "canonical")
		if err := sink.Append(context.Background(), conversationruntime.JournalEntry{EventType: conversationruntime.EventCompleted, Detail: detail}); err != nil {
			t.Fatal(err)
		}
		events, _, err := manager.GetEvents(context.Background(), "canonical", 0)
		if err != nil || len(events) != 2 || events[0].Name != "finish" || events[1].Name != "done" {
			t.Fatalf("events=%#v err=%v", events, err)
		}
		var source, wire map[string]any
		if err := json.Unmarshal([]byte(detail), &source); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(events[0].Data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["content"] != source["content"] {
			t.Fatalf("finish=%s", events[0].Data)
		}
	}
}
