package history

import "testing"

func TestParseStructuredSummaryRejectsUnknownFields(t *testing.T) {
	_, err := ParseStructuredSummary(`{"schema_version":1,"goal":"x","unknown":"y"}`)
	if err == nil {
		t.Fatal("expected unknown fields to be rejected")
	}
}

func TestParseStructuredSummaryAcceptsStringSchemaVersion(t *testing.T) {
	summary, err := ParseStructuredSummary(`{"schema_version":"1","goal":"当前主目标是先做 spec"}`)
	if err != nil {
		t.Fatalf("ParseStructuredSummary() error = %v", err)
	}
	if summary.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", summary.SchemaVersion)
	}
	if summary.Goal != "当前主目标是先做 spec" {
		t.Fatalf("Goal = %q, want 当前主目标是先做 spec", summary.Goal)
	}
}

func TestParseStructuredSummaryAcceptsDecimalStringSchemaVersion(t *testing.T) {
	summary, err := ParseStructuredSummary(`{"schema_version":"1.0","goal":"当前主目标是先做 spec"}`)
	if err != nil {
		t.Fatalf("ParseStructuredSummary() error = %v", err)
	}
	if summary.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", summary.SchemaVersion)
	}
}

func TestParseStructuredSummaryAcceptsDecimalNumericSchemaVersion(t *testing.T) {
	summary, err := ParseStructuredSummary(`{"schema_version":0.1,"goal":"当前主目标是先做 spec"}`)
	if err != nil {
		t.Fatalf("ParseStructuredSummary() error = %v", err)
	}
	if summary.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", summary.SchemaVersion)
	}
}

func TestParseStructuredSummarySupportsPriorityHierarchyFields(t *testing.T) {
	summary, err := ParseStructuredSummary(`{
		"schema_version": 2,
		"goal": "收敛 summary 方案",
		"active_priorities": ["先完成 spec 和 tasks"],
		"background_issues": ["CI flaky 不是当前重点"]
	}`)
	if err != nil {
		t.Fatalf("ParseStructuredSummary() error = %v", err)
	}
	if len(summary.ActivePriorities) != 1 || summary.ActivePriorities[0] != "先完成 spec 和 tasks" {
		t.Fatalf("ActivePriorities = %#v", summary.ActivePriorities)
	}
	if len(summary.BackgroundIssues) != 1 || summary.BackgroundIssues[0] != "CI flaky 不是当前重点" {
		t.Fatalf("BackgroundIssues = %#v", summary.BackgroundIssues)
	}
}
