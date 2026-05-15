package tool

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

const (
	maxLLMSummaryItems   = 3
	maxLLMSummaryTextLen = 160
)

func readStringArg(arguments map[string]any, key string) string {
	return ragcore.ReadStringArg(arguments, key)
}

func readBoolArg(arguments map[string]any, key string) bool {
	return ragcore.ReadBoolArg(arguments, key)
}

func firstNonEmpty(values ...string) string {
	return ragcore.FirstNonEmpty(values...)
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func trimSummaryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maxLLMSummaryTextLen {
		return value
	}
	return strings.TrimSpace(value[:maxLLMSummaryTextLen-3]) + "..."
}

func limitStrings(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	if max <= 0 || len(items) <= max {
		return items
	}
	return items[:max]
}

func trimAndFilterStrings(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := trimSummaryText(item); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func readDataInt(data map[string]any, key string) int {
	return ragcore.ReadDataInt(data, key)
}

func readDataBool(data map[string]any, key string) bool {
	return ragcore.ReadDataBool(data, key)
}

func readDataFloat(data map[string]any, key string) float64 {
	return ragcore.ReadDataFloat(data, key)
}

func readDataString(data map[string]any, key string) string {
	return ragcore.ReadDataString(data, key)
}

func readDataStringSlice(data map[string]any, key string) []string {
	return ragcore.ReadDataStringSlice(data, key)
}

func preferDataStringSlice(data map[string]any, primary string, fallback string) []string {
	return ragcore.PreferDataStringSlice(data, primary, fallback)
}

func readMapItems(raw any) []map[string]any {
	return ragcore.ReadMapItems(raw)
}

func readDataMap(data map[string]any, key string) map[string]any {
	return ragcore.ReadDataMap(data, key)
}
