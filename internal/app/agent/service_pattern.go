package agent

import "strings"

const (
	PatternReactive    = "reactive"
	PatternPlanExecute = "plan_execute"
)

func normalizePattern(pattern string) string {
	switch strings.TrimSpace(pattern) {
	case "", PatternReactive:
		return PatternReactive
	case PatternPlanExecute:
		return PatternPlanExecute
	default:
		return PatternReactive
	}
}

func runtimeNameForPattern(pattern string) string {
	switch normalizePattern(pattern) {
	case PatternPlanExecute:
		return "agent_service_plan_execute"
	default:
		return "agent_service_reactive"
	}
}
