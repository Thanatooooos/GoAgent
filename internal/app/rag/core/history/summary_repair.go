package history

import "strings"

type summaryRepairSection int

const (
	summaryRepairSectionConstraints summaryRepairSection = iota
	summaryRepairSectionEstablishedFacts
	summaryRepairSectionRecentProgress
	summaryRepairSectionOpenQuestions
)

var summaryRepairBoundaryMarkers = []string{
	"\u5f53\u524d\u4e0d",
	"\u5f53\u524d\u4ec5",
	"\u5f53\u524d\u5148",
	"\u6682\u4e0d",
	"\u5148\u4e0d",
	"\u4e0d\u8981",
	"\u4e0d\u80fd",
	"\u4e0d\u8fdb\u5165",
	"\u4e0d\u505a",
	"\u4e0d\u6539",
	"\u907f\u514d",
	"\u53ea\u4fdd\u7559",
	"\u4ec5\u4fdd\u7559",
	"\u5f53\u524d\u8fb9\u754c",
}

var summaryRepairUnresolvedMarkers = []string{
	"\u5f85\u786e\u8ba4",
	"\u5f85\u6838\u5b9e",
	"\u5f85\u9a8c\u8bc1",
	"\u672a\u786e\u8ba4",
	"\u672a\u6838\u5b9e",
	"\u672a\u9a8c\u8bc1",
	"\u8fd8\u6ca1\u786e\u8ba4",
	"\u8fd8\u672a\u786e\u8ba4",
	"\u6ca1\u786e\u8ba4",
	"\u9700\u8981\u786e\u8ba4",
	"\u6709\u5f85\u786e\u8ba4",
	"\u6682\u4e0d\u786e\u5b9a",
	"\u4e0d\u786e\u5b9a",
	"\u5019\u9009",
	"\u7591\u4f3c",
}

var summaryRepairProgressMarkers = []string{
	"\u5df2\u786e\u8ba4",
	"\u5df2\u7ecf\u786e\u8ba4",
	"\u5df2\u5b8c\u6210",
	"\u5df2\u7ecf\u5b8c\u6210",
	"\u5df2\u4fee\u590d",
	"\u5df2\u7ecf\u4fee\u590d",
	"\u5df2\u66f4\u65b0",
	"\u5df2\u7ecf\u66f4\u65b0",
	"\u5df2\u5207\u6362",
	"\u5df2\u7ecf\u5207\u6362",
	"\u5df2\u6574\u7406",
	"\u5df2\u6536\u655b",
	"\u5df2\u843d\u5730",
	"\u521a\u786e\u8ba4",
	"\u521a\u5b8c\u6210",
	"\u521a\u4fee\u590d",
	"\u521a\u66f4\u65b0",
	"\u786e\u8ba4\u5b8c\u6210",
	"\u5b8c\u6210\u4e86",
	"\u4fee\u590d\u5b8c\u6210",
}

// RepairStructuredSummary conservatively reuses only already-present content.
func RepairStructuredSummary(summary StructuredSummary) StructuredSummary {
	summary.Normalize()

	repaired := StructuredSummary{
		SchemaVersion: summary.SchemaVersion,
		Goal:          summary.Goal,
	}

	seen := map[string]struct{}{}
	appendItem := func(section summaryRepairSection, item string) {
		item = strings.TrimSpace(item)
		if item == "" {
			return
		}
		key := strings.ToLower(item)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}

		switch section {
		case summaryRepairSectionConstraints:
			repaired.Constraints = append(repaired.Constraints, item)
		case summaryRepairSectionEstablishedFacts:
			repaired.EstablishedFacts = append(repaired.EstablishedFacts, item)
		case summaryRepairSectionRecentProgress:
			repaired.RecentProgress = append(repaired.RecentProgress, item)
		case summaryRepairSectionOpenQuestions:
			repaired.OpenQuestions = append(repaired.OpenQuestions, item)
		}
	}

	for _, item := range summary.Constraints {
		appendItem(repairSummarySectionForItem(summaryRepairSectionConstraints, item), item)
	}
	for _, item := range summary.EstablishedFacts {
		appendItem(repairSummarySectionForItem(summaryRepairSectionEstablishedFacts, item), item)
	}
	for _, item := range summary.RecentProgress {
		appendItem(repairSummarySectionForItem(summaryRepairSectionRecentProgress, item), item)
	}
	for _, item := range summary.OpenQuestions {
		appendItem(repairSummarySectionForItem(summaryRepairSectionOpenQuestions, item), item)
	}

	repaired.Normalize()
	return repaired
}

func repairSummarySectionForItem(source summaryRepairSection, item string) summaryRepairSection {
	if isSummaryRepairBoundaryStatement(item) {
		return summaryRepairSectionConstraints
	}
	if isSummaryRepairUnresolvedItem(item) {
		return summaryRepairSectionOpenQuestions
	}
	if isSummaryRepairRecentProgressItem(item) {
		return summaryRepairSectionRecentProgress
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
