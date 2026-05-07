package builtin

import "strings"

func readStringArg(arguments map[string]any, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func readBoolArg(arguments map[string]any, key string) bool {
	if len(arguments) == 0 {
		return false
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
