package capability

import "strings"

// StringPtr returns a pointer to the given string value.
func StringPtr(value string) *string {
	return &value
}

// StringPtrIfNotEmpty returns a pointer to the given string value, or nil when
// the trimmed value is empty.
func StringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// FirstNonEmpty returns the first candidate whose trimmed value is non-empty,
// or an empty string when no candidate qualifies.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// AppendNonEmpty appends each trimmed non-empty candidate to values and returns
// the resulting slice. When the final slice is empty it returns nil.
func AppendNonEmpty(values []string, candidates ...string) []string {
	for _, candidate := range candidates {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// ApplyOptions mutates spec by applying each non-nil option in order.
func ApplyOptions(spec *Spec, options ...Option) {
	for _, option := range options {
		if option != nil {
			option(spec)
		}
	}
}

// MatchesPermissionError reports whether any of the supplied values contains a
// keyword associated with permission or authorization failures.
func MatchesPermissionError(values ...string) bool {
	return containsClassificationKeyword(values, "permission", "forbidden", "unauthorized", "approval", "access denied", "not allowed")
}

// MatchesDependencyError reports whether any of the supplied values contains a
// keyword associated with upstream dependency unavailability.
func MatchesDependencyError(values ...string) bool {
	return containsClassificationKeyword(values, "dependency", "upstream unavailable", "provider unavailable")
}

func containsClassificationKeyword(values []string, keywords ...string) bool {
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		for _, keyword := range keywords {
			if strings.Contains(normalized, keyword) {
				return true
			}
		}
	}
	return false
}
