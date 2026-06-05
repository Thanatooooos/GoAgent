package capability

import (
	"fmt"
	"sort"
	"strings"
)

// RoleBindings maps pattern roles to named registered capabilities.
type RoleBindings map[string]string

// Resolve returns the bound capability name for a role.
func (b RoleBindings) Resolve(role string) string {
	if len(b) == 0 {
		return ""
	}
	return strings.TrimSpace(b[strings.TrimSpace(role)])
}

// Names returns all bound capability names in stable order.
func (b RoleBindings) Names() []string {
	if len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(b))
	names := make([]string, 0, len(b))
	for _, name := range b {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		names = append(names, trimmed)
	}
	sort.Strings(names)
	return names
}

// Validate ensures all explicit bindings point at registered capabilities that
// actually implement the declared role.
func (b RoleBindings) Validate(registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("capability registry is not initialized")
	}
	for role, name := range b {
		trimmedRole := strings.TrimSpace(role)
		trimmedName := strings.TrimSpace(name)
		if trimmedRole == "" || trimmedName == "" {
			return fmt.Errorf("role bindings require non-empty role and capability name")
		}
		spec, ok := registry.Spec(trimmedName)
		if !ok {
			return fmt.Errorf("capability %q bound to role %q is not registered", trimmedName, trimmedRole)
		}
		if !specHasRole(spec, trimmedRole) {
			return fmt.Errorf("capability %q does not declare role %q", trimmedName, trimmedRole)
		}
	}
	return nil
}

// ResolveBinding returns the capability name bound to a role, falling back to a
// unique registered capability for that role when no explicit binding exists.
func ResolveBinding(registry *Registry, bindings RoleBindings, role string) (string, error) {
	trimmedRole := strings.TrimSpace(role)
	if trimmedRole == "" {
		return "", fmt.Errorf("role is required")
	}
	if registry == nil {
		return "", fmt.Errorf("capability registry is not initialized")
	}
	if bindings != nil {
		if err := bindings.Validate(registry); err != nil {
			return "", err
		}
		if name := bindings.Resolve(trimmedRole); name != "" {
			return name, nil
		}
	}

	names := registry.NamesByRole(trimmedRole)
	switch len(names) {
	case 0:
		return "", fmt.Errorf("pattern requires a capability for role %q", trimmedRole)
	case 1:
		return names[0], nil
	default:
		return "", fmt.Errorf("role %q has multiple candidates %v; explicit binding is required", trimmedRole, names)
	}
}
