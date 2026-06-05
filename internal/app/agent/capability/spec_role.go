package capability

import "strings"

// HasRole reports whether the capability spec declares the given role.
func HasRole(spec Spec, role string) bool {
	trimmed := strings.TrimSpace(role)
	if trimmed == "" {
		return false
	}
	return specHasRole(spec, trimmed)
}
