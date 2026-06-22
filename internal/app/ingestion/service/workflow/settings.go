package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

func ReadStringSetting(values map[string]any, key string) string {
	return readStringSetting(values, key)
}

func ReadIntSetting(values map[string]any, key string) int {
	return readIntSetting(values, key)
}

func readStringSetting(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func readIntSetting(values map[string]any, key string) int {
	if len(values) == 0 {
		return 0
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return 0
	}
	switch typed := raw.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		value, _ := strconv.Atoi(strings.TrimSpace(typed))
		return value
	default:
		return 0
	}
}
