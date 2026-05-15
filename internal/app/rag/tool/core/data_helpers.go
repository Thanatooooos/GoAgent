package core

import (
	"fmt"
	"strings"
)

func ReadDataInt(data map[string]any, key string) int {
	if len(data) == 0 {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func ReadDataBool(data map[string]any, key string) bool {
	if len(data) == 0 {
		return false
	}
	value, ok := data[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func ReadDataFloat(data map[string]any, key string) float64 {
	if len(data) == 0 {
		return 0
	}
	value, ok := data[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func ReadDataString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func ReadDataStringSlice(data map[string]any, key string) []string {
	if len(data) == 0 {
		return []string{}
	}
	value, ok := data[key]
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := fmt.Sprintf("%v", item)
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	default:
		return []string{}
	}
}

func PreferDataStringSlice(data map[string]any, primary string, fallback string) []string {
	items := ReadDataStringSlice(data, primary)
	if len(items) > 0 {
		return items
	}
	return ReadDataStringSlice(data, fallback)
}

func ReadMapItems(raw any) []map[string]any {
	switch typed := raw.(type) {
	case []map[string]any:
		return typed
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if ok {
				items = append(items, mapped)
			}
		}
		return items
	default:
		return nil
	}
}

func ReadDataMap(data map[string]any, key string) map[string]any {
	if len(data) == 0 {
		return nil
	}
	value, ok := data[key]
	if !ok || value == nil {
		return nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return mapped
}
