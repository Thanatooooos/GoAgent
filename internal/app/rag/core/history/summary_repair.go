package history

import "strings"

type summaryRepairSection int

const (
	summaryRepairSectionEstablishedFacts summaryRepairSection = iota
	summaryRepairSectionConstraints
	summaryRepairSectionRecentProgress
	summaryRepairSectionActivePriorities
	summaryRepairSectionBackgroundIssues
	summaryRepairSectionOpenQuestions
)

type summaryRepairItem struct {
	text    string
	section summaryRepairSection
}

var summaryRepairBoundaryMarkers = []string{
	"当前不",
	"当前仅",
	"当前先",
	"暂不",
	"先不",
	"不要",
	"不能",
	"不进入",
	"不做",
	"不改",
	"避免",
	"只保留",
	"仅保留",
	"当前边界",
}

var summaryRepairUnresolvedMarkers = []string{
	"待确认",
	"待核实",
	"待验证",
	"未确认",
	"未核实",
	"未验证",
	"还没确认",
	"还未确认",
	"没确认",
	"需要确认",
	"有待确认",
	"暂不确定",
	"不确定",
	"候选",
	"疑似",
}

var summaryRepairProgressMarkers = []string{
	"已确认",
	"已经确认",
	"已完成",
	"已经完成",
	"已修复",
	"已经修复",
	"已更新",
	"已经更新",
	"已切换",
	"已经切换",
	"已整理",
	"已收敛",
	"已落地",
	"刚确认",
	"刚完成",
	"刚修复",
	"刚更新",
	"确认完成",
	"完成了",
	"修复完成",
}

var summaryRepairBackgroundMarkers = []string{
	"不是当前重点",
	"只是背景问题",
	"背景问题",
	"暂不处理",
	"不是当前主线",
}

var summaryRepairActivePriorityMarkers = []string{
	"当前真正活跃的目标",
	"当前活跃目标",
	"当前重点",
	"本周重点",
	"当前主线",
}

// RepairStructuredSummary conservatively reuses only already-present content.
func RepairStructuredSummary(summary StructuredSummary) StructuredSummary {
	summary.Normalize()

	repaired := StructuredSummary{
		SchemaVersion:   summary.SchemaVersion,
		Goal:            summary.Goal,
		UserPreferences: dedupeSummaryItems(summary.UserPreferences),
	}

	items := make([]summaryRepairItem, 0, len(summary.ActivePriorities)+len(summary.Constraints)+len(summary.EstablishedFacts)+len(summary.RecentProgress)+len(summary.OpenQuestions)+len(summary.BackgroundIssues))
	seen := map[string]int{}

	appendItem := func(source summaryRepairSection, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		section := repairSummarySectionForItem(source, text)
		key := strings.ToLower(text)
		if idx, exists := seen[key]; exists {
			if summaryRepairSectionPriority(section) > summaryRepairSectionPriority(items[idx].section) {
				items[idx].section = section
			}
			return
		}
		seen[key] = len(items)
		items = append(items, summaryRepairItem{text: text, section: section})
	}

	for _, item := range summary.ActivePriorities {
		appendItem(summaryRepairSectionActivePriorities, item)
	}
	for _, item := range summary.Constraints {
		appendItem(summaryRepairSectionConstraints, item)
	}
	for _, item := range summary.EstablishedFacts {
		appendItem(summaryRepairSectionEstablishedFacts, item)
	}
	for _, item := range summary.RecentProgress {
		appendItem(summaryRepairSectionRecentProgress, item)
	}
	for _, item := range summary.OpenQuestions {
		appendItem(summaryRepairSectionOpenQuestions, item)
	}
	for _, item := range summary.BackgroundIssues {
		appendItem(summaryRepairSectionBackgroundIssues, item)
	}

	for _, item := range items {
		switch item.section {
		case summaryRepairSectionActivePriorities:
			repaired.ActivePriorities = append(repaired.ActivePriorities, item.text)
		case summaryRepairSectionConstraints:
			repaired.Constraints = append(repaired.Constraints, item.text)
		case summaryRepairSectionEstablishedFacts:
			repaired.EstablishedFacts = append(repaired.EstablishedFacts, item.text)
		case summaryRepairSectionRecentProgress:
			repaired.RecentProgress = append(repaired.RecentProgress, item.text)
		case summaryRepairSectionOpenQuestions:
			repaired.OpenQuestions = append(repaired.OpenQuestions, item.text)
		case summaryRepairSectionBackgroundIssues:
			repaired.BackgroundIssues = append(repaired.BackgroundIssues, item.text)
		}
	}

	repaired.ActivePriorities = backfillActivePriorities(summary, repaired.ActivePriorities)
	repaired.Normalize()
	return repaired
}

func backfillActivePriorities(original StructuredSummary, current []string) []string {
	result := append([]string(nil), current...)
	result = prependUniqueSummaryItem(result, original.Goal)
	for _, item := range original.RecentProgress {
		if containsAnySummaryRepairMarker(item, summaryRepairActivePriorityMarkers) {
			result = appendUniqueSummaryItem(result, item)
		}
	}
	result = dedupeSummaryItems(result)
	return result
}

func prependUniqueSummaryItem(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(strings.TrimSpace(existing), item) {
			return items
		}
	}
	return append([]string{item}, items...)
}

func appendUniqueSummaryItem(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if strings.EqualFold(strings.TrimSpace(existing), item) {
			return items
		}
	}
	return append(items, item)
}


func summaryRepairSectionPriority(section summaryRepairSection) int {
	switch section {
	case summaryRepairSectionOpenQuestions:
		return 5
	case summaryRepairSectionBackgroundIssues:
		return 4
	case summaryRepairSectionActivePriorities:
		return 3
	case summaryRepairSectionRecentProgress:
		return 2
	case summaryRepairSectionConstraints:
		return 1
	case summaryRepairSectionEstablishedFacts:
		return 0
	default:
		return -1
	}
}

func repairSummarySectionForItem(source summaryRepairSection, item string) summaryRepairSection {
	if isSummaryRepairBackgroundOnlyItem(item) {
		return summaryRepairSectionBackgroundIssues
	}
	// Unresolved content is safer than boundary language, so it wins first.
	if isSummaryRepairUnresolvedItem(item) {
		return summaryRepairSectionOpenQuestions
	}
	if isSummaryRepairRecentProgressItem(item) {
		return summaryRepairSectionRecentProgress
	}
	if isSummaryRepairBoundaryStatement(item) {
		return summaryRepairSectionConstraints
	}
	return source
}

func isSummaryRepairBoundaryStatement(item string) bool {
	return containsAnySummaryRepairMarker(item, summaryRepairBoundaryMarkers)
}

func isSummaryRepairUnresolvedItem(item string) bool {
	return containsAnySummaryRepairMarker(item, summaryRepairUnresolvedMarkers)
}

func isSummaryRepairRecentProgressItem(item string) bool {
	return containsAnySummaryRepairMarker(item, summaryRepairProgressMarkers)
}

func isSummaryRepairBackgroundOnlyItem(item string) bool {
	return containsAnySummaryRepairMarker(item, summaryRepairBackgroundMarkers)
}

func containsAnySummaryRepairMarker(item string, markers []string) bool {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if strings.Contains(trimmed, marker) || strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func dedupeSummaryItems(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}


