package history

import (
	"reflect"
	"testing"
)

func TestRepairStructuredSummaryDemotesUnresolvedEstablishedFacts(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion:    0,
		Goal:             "  整理结构化摘要  ",
		EstablishedFacts: []string{"  接口方案还没确认  "},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "schema_version", 1, got.SchemaVersion)
	assertSummaryField(t, "goal", "整理结构化摘要", got.Goal)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
	assertSummaryField(t, "open_questions", []string{"接口方案还没确认"}, got.OpenQuestions)
}

func TestRepairStructuredSummaryPromotesBoundaryStatements(t *testing.T) {
	input := StructuredSummary{
		Goal:             "整理结构化摘要",
		EstablishedFacts: []string{"当前不进入实现", "先不改验证门禁"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "constraints", []string{"当前不进入实现", "先不改验证门禁"}, got.Constraints)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func TestRepairStructuredSummaryBackfillsRecentProgressFromConfirmedItems(t *testing.T) {
	input := StructuredSummary{
		Goal:             "整理结构化摘要",
		EstablishedFacts: []string{"已确认切换到中文回答"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "recent_progress", []string{"已确认切换到中文回答"}, got.RecentProgress)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func TestRepairStructuredSummaryBackfillsOpenQuestionsFromUnresolvedMarkers(t *testing.T) {
	input := StructuredSummary{
		Goal:           "整理结构化摘要",
		RecentProgress: []string{"发布渠道待确认"},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "open_questions", []string{"发布渠道待确认"}, got.OpenQuestions)
	assertSummaryField(t, "recent_progress", nil, got.RecentProgress)
}

func TestRepairStructuredSummaryIsConservativeAndDedupes(t *testing.T) {
	input := StructuredSummary{
		SchemaVersion: 0,
		Goal:          "  整理结构化摘要  ",
		Constraints: []string{
			" 当前不进入实现 ",
			"当前不进入实现",
		},
		EstablishedFacts: []string{
			"已确认使用中文输出",
			"已确认使用中文输出",
		},
		OpenQuestions: []string{
			"待确认发布渠道",
			"待确认发布渠道",
		},
	}

	got := RepairStructuredSummary(input)

	assertSummaryField(t, "schema_version", 1, got.SchemaVersion)
	assertSummaryField(t, "goal", "整理结构化摘要", got.Goal)
	assertSummaryField(t, "constraints", []string{"当前不进入实现"}, got.Constraints)
	assertSummaryField(t, "recent_progress", []string{"已确认使用中文输出"}, got.RecentProgress)
	assertSummaryField(t, "open_questions", []string{"待确认发布渠道"}, got.OpenQuestions)
	assertSummaryField(t, "established_facts", nil, got.EstablishedFacts)
}

func assertSummaryField[T any](t *testing.T, name string, want, got T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected %s: got %#v, want %#v", name, got, want)
	}
}
