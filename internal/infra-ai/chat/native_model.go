package chat

import (
	"context"
	"encoding/json"

	"local/rag-project/internal/infra-ai/model"
)

type NativeMessage struct {
	Role       string
	Content    string
	ToolCalls  []NativeToolCall
	ToolCallID string
}
type NativeTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}
type NativeToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}
type NativeRequest struct {
	JSONMode bool
	System   []string
	Messages []NativeMessage
	Tools    []NativeTool
	Thinking bool
}

type NativeStreamCallback interface {
	OnContent(string) error
	OnThinking(string) error
	OnToolCall(ToolCallDelta) error
	OnComplete(string) error
}

type NativeStreamClient interface {
	StreamNative(context.Context, NativeRequest, model.ModelTarget, NativeStreamCallback) error
}
