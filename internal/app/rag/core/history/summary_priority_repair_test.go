package history

import "testing"

func TestRepairStructuredSummaryDemotesNonCurrentFocusIntoBackgroundIssues(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    2,
		ActivePriorities: []string{"CI flaky 不是当前重点"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "active_priorities", nil, got.ActivePriorities)
	assertSummaryField(t, "background_issues", []string{"CI flaky 不是当前重点"}, got.BackgroundIssues)
}

func TestRepairStructuredSummaryBackfillsGoalAndFocusIntoActivePriorities(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:  2,
		Goal:           "起草 summary 样本，并明确 must_cover 和 critical_contract 的边界",
		RecentProgress: []string{"确认本周重点是完成 spec、design、tasks 而不是实现"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "active_priorities", []string{
		"起草 summary 样本，并明确 must_cover 和 critical_contract 的边界",
		"确认本周重点是完成 spec、design、tasks 而不是实现",
	}, got.ActivePriorities)
}


