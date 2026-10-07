package runtime

import (
	"context"

	"local/rag-project/internal/app/runtime/capability"
)

type ModelRole string

const (
	ModelRoleSystem    ModelRole = "system"
	ModelRoleUser      ModelRole = "user"
	ModelRoleAssistant ModelRole = "assistant"
	ModelRoleTool      ModelRole = "tool"
)

// ModelMessage is the provider-neutral protocol projection. An assistant
// message carries text and requested calls; tool messages answer one call.
type ModelMessage struct {
	Role       ModelRole
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ModelRequest struct {
	JSONMode bool
	// Thinking overrides the adapter default for this call only. Auxiliary
	// summary/task calls can leave it nil to retain their existing behavior.
	Thinking *bool
	System   []string
	Messages []ModelMessage
	Tools    []capability.ModelDefinition
}

// StreamModel delivers normalized events synchronously as they arrive, then
// returns the exact completed turn assembled from those events.
type StreamModel interface {
	Stream(context.Context, ModelRequest, func(ModelEvent) error) (Turn, error)
}
