package test

import (
	"testing"

	"local/rag-project/internal/infra-ai/chat"
)

func TestParseOpenAIStyleSseLineExtractsToolCallDelta(t *testing.T) {
	event, err := chat.ParseOpenAIStyleSseLine(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","function":{"name":"retrieve_knowledge","arguments":"{\"query\":\""}}]}}]}`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(event.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", event.ToolCalls)
	}
	call := event.ToolCalls[0]
	if call.Index != 0 || call.ID != "call-1" || call.Name != "retrieve_knowledge" || call.Arguments != `{"query":"` {
		t.Fatalf("call = %#v", call)
	}
}
