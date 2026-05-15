package core

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var (
	DocumentIDPattern = regexp.MustCompile(`(?i)\b((?:doc|document)[-_][a-z0-9]+(?:[-_][a-z0-9]+)*)\b`)
	TaskIDPattern     = regexp.MustCompile(`(?i)\b(task[-_][a-z0-9]+(?:[-_][a-z0-9]+)*)\b`)
	TraceIDPattern    = regexp.MustCompile(`(?i)\b(trace[-_][a-z0-9]+(?:[-_][a-z0-9]+)*)\b`)
)

func ContainsAny(text string, keywords ...string) bool {
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func FirstMatchedID(pattern *regexp.Regexp, text string) string {
	if pattern == nil {
		return ""
	}
	match := pattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return ""
	}
	return strings.TrimSpace(match[0])
}

func CallKey(call Call) string {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return ""
	}
	if len(call.Arguments) == 0 {
		return name
	}

	keys := make([]string, 0, len(call.Arguments))
	for key := range call.Arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalizedArgs := make(map[string]any, len(keys))
	for _, key := range keys {
		normalizedArgs[key] = call.Arguments[key]
	}

	encoded, err := json.Marshal(normalizedArgs)
	if err != nil {
		return name
	}
	return name + ":" + string(encoded)
}

func NormalizeHintCalls(calls []HintCall) []HintCall {
	if len(calls) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	normalized := make([]HintCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		clonedArgs := CloneMap(call.Arguments)
		key := CallKey(Call{Name: name, Arguments: clonedArgs})
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, HintCall{
			Name:      name,
			Arguments: clonedArgs,
		})
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func BuildHintCall(toolName string, arguments map[string]any) []HintCall {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	return []HintCall{{
		Name:      toolName,
		Arguments: CloneMap(arguments),
	}}
}

func ParseHintCallsFromLegacyString(hint string) []HintCall {
	name, arguments := ParseNextHint(hint)
	if name == "" {
		return nil
	}
	return NormalizeHintCalls([]HintCall{{
		Name:      name,
		Arguments: arguments,
	}})
}

func SerializeHintCalls(calls []HintCall) string {
	calls = NormalizeHintCalls(calls)
	if len(calls) == 0 {
		return ""
	}
	call := calls[0]
	var builder strings.Builder
	builder.WriteString("tool:")
	builder.WriteString(strings.TrimSpace(call.Name))
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId", "includeNodes"} {
		value := ReadHintArg(call.Arguments, key)
		if value == "" {
			continue
		}
		builder.WriteString("|")
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(value)
	}

	keys := make([]string, 0, len(call.Arguments))
	for key := range call.Arguments {
		switch key {
		case "documentId", "taskId", "nodeId", "traceId", "includeNodes":
			continue
		default:
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := ReadHintArg(call.Arguments, key)
		if value == "" {
			continue
		}
		builder.WriteString("|")
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(value)
	}
	return builder.String()
}
