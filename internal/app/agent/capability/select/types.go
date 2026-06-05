package selectcapability

import (
	"context"

	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
)

type SelectionInput struct {
	UserRequest   string              `json:"user_request"`
	ContextNotes  []string            `json:"context_notes,omitempty"`
	Capabilities  []agentcatalog.Card `json:"capabilities"`
	MaxSelections int                 `json:"max_selections,omitempty"`
}

type SelectionOutput struct {
	Selections []CapabilitySelection `json:"selections,omitempty"`
}

type CapabilitySelection struct {
	Name       string         `json:"name,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Family     string         `json:"family,omitempty"`
	Role       string         `json:"role,omitempty"`
	Input      map[string]any `json:"input,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
}

type Selector interface {
	Select(ctx context.Context, input SelectionInput) (SelectionOutput, error)
}
