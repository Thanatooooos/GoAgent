package runtime

import (
	"fmt"
	"strings"

	"local/rag-project/internal/app/runtime/capability"
)

type ModelEventKind string

const (
	ModelEventThinking ModelEventKind = "thinking"
	ModelEventContent  ModelEventKind = "content"
	ModelEventToolCall ModelEventKind = "tool_call"
	ModelEventFinish   ModelEventKind = "finish"
)

// ModelEvent is provider-normalized. Tool calls may arrive in fragments; Index
// identifies the call being assembled and only the completed Turn is executable.
type ModelEvent struct {
	Kind          ModelEventKind
	Text          string
	ToolCallIndex int
	ToolCallID    string
	ToolName      string
	Arguments     capability.Value
	FinishReason  string
}

type Turn struct {
	Content      string
	Thinking     string
	ToolCalls    []ToolCall
	FinishReason string
}

// TurnBuilder keeps provider chunking outside runtime. Thinking remains out of
// model-visible history; Content and completed tool calls form the assistant turn.
type TurnBuilder struct {
	turn  Turn
	calls []ToolCall
}

func (b *TurnBuilder) Add(event ModelEvent) error {
	switch event.Kind {
	case ModelEventThinking:
		b.turn.Thinking += event.Text
	case ModelEventContent:
		b.turn.Content += event.Text
	case ModelEventToolCall:
		if event.ToolCallIndex < 0 {
			return fmt.Errorf("negative tool call index")
		}
		for len(b.calls) <= event.ToolCallIndex {
			b.calls = append(b.calls, ToolCall{})
		}
		call := &b.calls[event.ToolCallIndex]
		if event.ToolCallID != "" {
			call.ID = event.ToolCallID
		}
		if event.ToolName != "" {
			call.CapabilityID = event.ToolName
		}
		if len(event.Arguments) > 0 {
			call.Arguments = append(call.Arguments, event.Arguments...)
		}
	case ModelEventFinish:
		b.turn.FinishReason = event.FinishReason
	default:
		return fmt.Errorf("unknown model event kind %q", event.Kind)
	}
	return nil
}

func (b *TurnBuilder) Build() (Turn, error) {
	turn := b.turn
	turn.ToolCalls = make([]ToolCall, 0, len(b.calls))
	for _, call := range b.calls {
		if strings.TrimSpace(call.ID) == "" {
			return Turn{}, fmt.Errorf("tool call id is required")
		}
		if strings.TrimSpace(call.CapabilityID) == "" {
			return Turn{}, fmt.Errorf("tool call name is required")
		}
		turn.ToolCalls = append(turn.ToolCalls, call)
	}
	return turn, nil
}
