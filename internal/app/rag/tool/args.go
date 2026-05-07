package tool

import "strings"

func readStringArg(arguments map[string]any, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
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
