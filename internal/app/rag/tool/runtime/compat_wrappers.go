package runtime

import (
	"regexp"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

var (
	documentIDPattern = ragcore.DocumentIDPattern
	taskIDPattern     = ragcore.TaskIDPattern
	traceIDPattern    = ragcore.TraceIDPattern
)

func containsAny(text string, keywords ...string) bool {
	return ragcore.ContainsAny(text, keywords...)
}

func firstMatchedID(pattern *regexp.Regexp, text string) string {
	return ragcore.FirstMatchedID(pattern, text)
}

func callKey(call ragcore.Call) string {
	return ragcore.CallKey(call)
}

func normalizeHintCalls(calls []ragcore.HintCall) []ragcore.HintCall {
	return ragcore.NormalizeHintCalls(calls)
}

func parseHintCallsFromLegacyString(hint string) []ragcore.HintCall {
	return ragcore.ParseHintCallsFromLegacyString(hint)
}

func uniqueTrimmedStrings(items []string) []string {
	return ragcore.UniqueTrimmedStrings(items)
}

func clampConfidence(value float64) float64 {
	return ragcore.ClampConfidence(value)
}

func readStringArg(arguments map[string]any, key string) string {
	return ragcore.ReadStringArg(arguments, key)
}

func readBoolArg(arguments map[string]any, key string) bool {
	return ragcore.ReadBoolArg(arguments, key)
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

func readMapItems(raw any) []map[string]any {
	return ragcore.ReadMapItems(raw)
}

func readDataMap(data map[string]any, key string) map[string]any {
	return ragcore.ReadDataMap(data, key)
}
