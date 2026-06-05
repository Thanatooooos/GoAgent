package pattern

import (
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
)

// MergeInterruptBeforeNodes preserves declared interrupt targets while adding
// required runtime-managed approval gates without duplicates.
func MergeInterruptBeforeNodes(existing []string, required []string) []string {
	if len(required) == 0 {
		return append([]string(nil), existing...)
	}
	seen := make(map[string]struct{}, len(existing)+len(required))
	merged := make([]string, 0, len(existing)+len(required))
	appendNode := func(node string) {
		trimmed := strings.TrimSpace(node)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}
	for _, node := range existing {
		appendNode(node)
	}
	for _, node := range required {
		appendNode(node)
	}
	return merged
}

// ResolveNamedBinding resolves one required role binding and keeps a stable
// pattern-scoped error prefix for outer builders.
func ResolveNamedBinding(registry *agentcapability.Registry, bindings agentcapability.RoleBindings, scope string, role string) (string, error) {
	name, err := agentcapability.ResolveBinding(registry, bindings, role)
	if err != nil {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			scope = "pattern"
		}
		return "", fmt.Errorf("%s %s binding: %w", scope, role, err)
	}
	return name, nil
}

// ResolveNamedBindings resolves several required role bindings into a normalized
// binding map that downstream patterns can consume consistently.
func ResolveNamedBindings(registry *agentcapability.Registry, bindings agentcapability.RoleBindings, scope string, roles ...string) (agentcapability.RoleBindings, error) {
	resolved := make(agentcapability.RoleBindings, len(roles))
	for _, role := range roles {
		name, err := ResolveNamedBinding(registry, bindings, scope, role)
		if err != nil {
			return nil, err
		}
		resolved[role] = name
	}
	return resolved, nil
}
