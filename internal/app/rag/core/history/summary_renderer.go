package history

import "strings"

func RenderStructuredSummary(summary StructuredSummary, maxChars int) string {
	summary.Normalize()

	var sections []string
	if summary.Goal != "" {
		sections = append(sections, "目标："+summary.Goal)
	}
	if len(summary.Constraints) > 0 {
		sections = append(sections, "约束：\n- "+strings.Join(summary.Constraints, "\n- "))
	}
	if len(summary.UserPreferences) > 0 {
		sections = append(sections, "用户偏好：\n- "+strings.Join(summary.UserPreferences, "\n- "))
	}
	if len(summary.EstablishedFacts) > 0 {
		sections = append(sections, "已确认事实：\n- "+strings.Join(summary.EstablishedFacts, "\n- "))
	}
	if len(summary.RecentProgress) > 0 {
		sections = append(sections, "最近进展：\n- "+strings.Join(summary.RecentProgress, "\n- "))
	}
	if len(summary.OpenQuestions) > 0 {
		sections = append(sections, "待确认问题：\n- "+strings.Join(summary.OpenQuestions, "\n- "))
	}

	return trimRunes(strings.Join(sections, "\n"), maxChars)
}

func trimRunes(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if maxChars <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxChars {
		return value
	}
	return strings.TrimSpace(string(runes[:maxChars]))
}
