package resolve

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
)

type Resolver interface {
	Match(selection selectcapability.CapabilitySelection) (MatchedCapability, error)
	Resolve(selection selectcapability.CapabilitySelection) (ResolvedCapability, error)
}

type RegistryResolver struct {
	registry *agentcapability.Registry
}

func NewRegistryResolver(registry *agentcapability.Registry) *RegistryResolver {
	if registry == nil {
		return nil
	}
	return &RegistryResolver{registry: registry}
}

func (r *RegistryResolver) Match(selection selectcapability.CapabilitySelection) (MatchedCapability, error) {
	if r == nil || r.registry == nil {
		return MatchedCapability{}, fmt.Errorf("capability resolver requires registry")
	}
	names, err := r.matchNames(selection)
	if err != nil {
		return MatchedCapability{}, err
	}
	if len(names) == 0 {
		return MatchedCapability{}, NotFoundError{Message: "no capability matched the selection"}
	}
	if len(names) > 1 {
		return MatchedCapability{}, AmbiguousError{Message: fmt.Sprintf("selection matched multiple capabilities: %s", strings.Join(names, ", "))}
	}
	spec, ok := r.registry.Spec(names[0])
	if !ok {
		return MatchedCapability{}, NotFoundError{Message: fmt.Sprintf("capability %q is not registered", names[0])}
	}
	return MatchedCapability{
		Name:      names[0],
		Spec:      spec,
		Selection: selection,
	}, nil
}

func (r *RegistryResolver) Resolve(selection selectcapability.CapabilitySelection) (ResolvedCapability, error) {
	matched, err := r.Match(selection)
	if err != nil {
		return ResolvedCapability{}, err
	}
	handle, err := r.registry.Handle(matched.Name)
	if err != nil {
		return ResolvedCapability{}, err
	}
	input := any(selection.Input)
	if normalizer, ok := handle.(agentcapability.InputNormalizer); ok {
		input, err = normalizer.NormalizeInput(selection.Input)
		if err != nil {
			return ResolvedCapability{}, InvalidInputError{Name: matched.Name, Err: err}
		}
	}
	if err := agentcapability.ValidateInput(matched.Spec, input); err != nil {
		return ResolvedCapability{}, InvalidInputError{Name: matched.Name, Err: err}
	}
	return ResolvedCapability{
		Name:      matched.Name,
		Spec:      matched.Spec,
		Handle:    handle,
		Input:     input,
		Selection: selection,
	}, nil
}

func (r *RegistryResolver) matchNames(selection selectcapability.CapabilitySelection) ([]string, error) {
	name := strings.TrimSpace(selection.Name)
	if name != "" {
		spec, ok := r.registry.Spec(name)
		if !ok {
			return nil, NotFoundError{Message: fmt.Sprintf("capability %q is not registered", name)}
		}
		if !selectionMatchesSpec(selection, spec) {
			return nil, NotFoundError{Message: fmt.Sprintf("capability %q does not satisfy the requested selector", name)}
		}
		return []string{name}, nil
	}

	candidates := r.registry.Specs()
	if len(candidates) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(candidates))
	for _, spec := range candidates {
		if selectionMatchesSpec(selection, spec) {
			names = append(names, spec.Name)
		}
	}
	return names, nil
}

func selectionMatchesSpec(selection selectcapability.CapabilitySelection, spec agentcapability.Spec) bool {
	if kind := strings.TrimSpace(selection.Kind); kind != "" && spec.Kind != kind {
		return false
	}
	if family := strings.TrimSpace(selection.Family); family != "" && spec.Family != family {
		return false
	}
	if role := strings.TrimSpace(selection.Role); role != "" && !agentcapability.HasRole(spec, role) {
		return false
	}
	return true
}
