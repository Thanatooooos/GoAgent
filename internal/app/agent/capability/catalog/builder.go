package catalog

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

// Builder creates an LLM-facing capability catalog from the registry.
type Builder interface {
	Build(registry *agentcapability.Registry) ([]Card, error)
}

type RegistryBuilder struct{}

func NewBuilder() *RegistryBuilder {
	return &RegistryBuilder{}
}

func (b *RegistryBuilder) Build(registry *agentcapability.Registry) ([]Card, error) {
	if registry == nil {
		return nil, fmt.Errorf("capability catalog builder requires registry")
	}
	specs := registry.Specs()
	if len(specs) == 0 {
		return nil, nil
	}
	cards := make([]Card, 0, len(specs))
	for _, spec := range specs {
		cards = append(cards, Card{
			Name:             spec.Name,
			Kind:             spec.Kind,
			Family:           spec.Family,
			Roles:            append([]string(nil), spec.Roles...),
			Summary:          strings.TrimSpace(spec.Description),
			InputHints:       buildInputHints(spec),
			RequiresApproval: spec.RequiresApproval,
			SupportsResume:   spec.SupportsResume,
			ProducesEvidence: spec.ProducesEvidence,
		})
	}
	return cards, nil
}

func buildInputHints(spec agentcapability.Spec) []string {
	if len(spec.Preconditions) == 0 {
		return nil
	}
	hints := make([]string, 0, len(spec.Preconditions))
	for _, precondition := range spec.Preconditions {
		field := strings.TrimSpace(precondition.Field)
		requirement := strings.TrimSpace(precondition.Requirement)
		description := strings.TrimSpace(precondition.Description)
		switch {
		case description != "":
			hints = append(hints, description)
		case field != "" && requirement != "":
			hints = append(hints, fmt.Sprintf("requires %s to satisfy %s", field, requirement))
		case field != "":
			hints = append(hints, "requires "+field)
		}
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}
