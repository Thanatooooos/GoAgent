package core

import (
	"strconv"
	"strings"
)

func TruncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func CloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func ParseNextHint(hint string) (string, map[string]any) {
	hint = strings.TrimSpace(hint)
	if hint == "" || !strings.HasPrefix(hint, "tool:") {
		return "", nil
	}
	parts := strings.Split(hint, "|")
	if len(parts) == 0 {
		return "", nil
	}
	name := strings.TrimSpace(strings.TrimPrefix(parts[0], "tool:"))
	if name == "" {
		return "", nil
	}
	arguments := map[string]any{}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		arguments[key] = value
	}
	return name, arguments
}

func NewObserveResult(done bool, reasoning string, state AgentState) ObserveResult {
	normalized := state.Normalize()
	return ObserveResult{
		Done:          done,
		Reasoning:     strings.TrimSpace(reasoning),
		NextHintCalls: append([]HintCall(nil), normalized.NextHintCalls...),
		NextHint:      normalized.NextHint,
		Confidence:    normalized.Confidence,
		State:         normalized,
	}
}

func ObserveState(phase string, hypothesis string, confidence float64, nextHintCalls []HintCall, checkedTools []string, openQuestions ...string) AgentState {
	return AgentState{
		Phase:         phase,
		Hypothesis:    hypothesis,
		Confidence:    confidence,
		OpenQuestions: openQuestions,
		CheckedTools:  checkedTools,
		NextHintCalls: nextHintCalls,
	}.Normalize()
}

func CoerceHintArgument(value any, paramType string) (any, bool) {
	switch strings.TrimSpace(paramType) {
	case "", ParamTypeString:
		switch typed := value.(type) {
		case string:
			typed = strings.TrimSpace(typed)
			return typed, typed != ""
		default:
			return "", false
		}
	case ParamTypeBoolean:
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return false, false
			}
			parsed, err := strconv.ParseBool(typed)
			if err != nil {
				return false, false
			}
			return parsed, true
		default:
			return false, false
		}
	case ParamTypeInteger:
		switch typed := value.(type) {
		case int:
			return typed, true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return 0, false
			}
			parsed, err := strconv.Atoi(typed)
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	case ParamTypeNumber:
		switch typed := value.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return 0, false
			}
			parsed, err := strconv.ParseFloat(typed, 64)
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	case ParamTypeObject, ParamTypeArray:
		return value, true
	default:
		return value, true
	}
}
