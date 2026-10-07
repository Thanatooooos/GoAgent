package runtime

import (
	"testing"

	"local/rag-project/internal/app/runtime/capability"
)

func TestTurnBuilderClassifiesTextThinkingAndFragmentedToolCalls(t *testing.T) {
	t.Parallel()
	var builder TurnBuilder
	events := []ModelEvent{
		{Kind: ModelEventThinking, Text: "reason"},
		{Kind: ModelEventContent, Text: "hello"},
		{Kind: ModelEventToolCall, ToolCallIndex: 0, ToolCallID: "call-1", ToolName: "retrieve_knowledge", Arguments: capability.Value(`{"query":`)},
		{Kind: ModelEventToolCall, ToolCallIndex: 0, Arguments: capability.Value(`"release"}`)},
		{Kind: ModelEventFinish, FinishReason: "tool_calls"},
	}
	for _, event := range events {
		if err := builder.Add(event); err != nil {
			t.Fatal(err)
		}
	}
	turn, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if turn.Thinking != "reason" || turn.Content != "hello" || turn.FinishReason != "tool_calls" {
		t.Fatalf("turn = %#v", turn)
	}
	if len(turn.ToolCalls) != 1 || string(turn.ToolCalls[0].Arguments) != `{"query":"release"}` {
		t.Fatalf("calls = %#v", turn.ToolCalls)
	}
}
