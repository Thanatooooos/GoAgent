package history

import (
	"reflect"
	"testing"
)

func TestRepairStructuredSummaryDemotesUnresolvedEstablishedFacts(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    0,
		Goal:             "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981",
		EstablishedFacts: []string{"  \u63a5\u53e3\u65b9\u6848\u8fd8\u6ca1\u786e\u8ba4  "},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "schema_version", 1, got.SchemaVersion)
	assertSummaryField(t, "goal", "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981", got.Goal)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
	assertSummaryField(t, "open_questions", []string{"\u63a5\u53e3\u65b9\u6848\u8fd8\u6ca1\u786e\u8ba4"}, got.OpenQuestions)
}

func TestRepairStructuredSummaryPromotesBoundaryStatements(t *testing.T) {
	input := StructuredSummary{
		Goal:             "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981",
		EstablishedFacts: []string{"\u5f53\u524d\u4e0d\u8fdb\u5165\u5b9e\u73b0", "\u5148\u4e0d\u6539\u9a8c\u8bc1\u95e8\u7981"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "constraints", []string{"\u5f53\u524d\u4e0d\u8fdb\u5165\u5b9e\u73b0", "\u5148\u4e0d\u6539\u9a8c\u8bc1\u95e8\u7981"}, got.Constraints)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func TestRepairStructuredSummaryBackfillsRecentProgressFromConfirmedItems(t *testing.T) {
	input := StructuredSummary{
		Goal:             "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981",
		EstablishedFacts: []string{"\u5df2\u786e\u8ba4\u5207\u6362\u5230\u4e2d\u6587\u56de\u7b54"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "recent_progress", []string{"\u5df2\u786e\u8ba4\u5207\u6362\u5230\u4e2d\u6587\u56de\u7b54"}, got.RecentProgress)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func TestRepairStructuredSummaryPreservesUserPreferences(t *testing.T) {
	input := StructuredSummary{
		Goal:            "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981",
		UserPreferences: []string{"  \u7528\u6237\u504f\u597d A  ", "\u7528\u6237\u504f\u597d A", "\u7528\u6237\u504f\u597d B"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "user_preferences", []string{"\u7528\u6237\u504f\u597d A", "\u7528\u6237\u504f\u597d B"}, got.UserPreferences)
}

func TestRepairStructuredSummaryBackfillsOpenQuestionsFromUnresolvedMarkers(t *testing.T) {
	input := StructuredSummary{
		Goal:           "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981",
		RecentProgress: []string{"\u53d1\u5e03\u6e20\u9053\u5f85\u786e\u8ba4"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "open_questions", []string{"\u53d1\u5e03\u6e20\u9053\u5f85\u786e\u8ba4"}, got.OpenQuestions)
	assertSummaryField(t, "recent_progress", nil, got.RecentProgress)
}

func TestRepairStructuredSummaryIsConservativeAndDedupes(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion: 0,
		Goal:          "  \u6574\u7406\u7ed3\u6784\u5316\u6458\u8981  ",
		Constraints: []string{
			" \u5f53\u524d\u4e0d\u8fdb\u5165\u5b9e\u73b0 ",
			"\u5f53\u524d\u4e0d\u8fdb\u5165\u5b9e\u73b0",
		},
		EstablishedFacts: []string{
			"\u5df2\u786e\u8ba4\u4f7f\u7528\u4e2d\u6587\u8f93\u51fa",
			"\u5df2\u786e\u8ba4\u4f7f\u7528\u4e2d\u6587\u8f93\u51fa",
		},
		OpenQuestions: []string{
			"\u5f85\u786e\u8ba4\u53d1\u5e03\u6e20\u9053",
			"\u5f85\u786e\u8ba4\u53d1\u5e03\u6e20\u9053",
		},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "schema_version", 1, got.SchemaVersion)
	assertSummaryField(t, "goal", "\u6574\u7406\u7ed3\u6784\u5316\u6458\u8981", got.Goal)
	assertSummaryField(t, "constraints", []string{"\u5f53\u524d\u4e0d\u8fdb\u5165\u5b9e\u73b0"}, got.Constraints)
	assertSummaryField(t, "recent_progress", []string{"\u5df2\u786e\u8ba4\u4f7f\u7528\u4e2d\u6587\u8f93\u51fa"}, got.RecentProgress)
	assertSummaryField(t, "open_questions", []string{"\u5f85\u786e\u8ba4\u53d1\u5e03\u6e20\u9053"}, got.OpenQuestions)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func assertSummaryField[T any](t *testing.T, name string, want, got T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected %s: got %#v, want %#v", name, got, want)
	}
}
