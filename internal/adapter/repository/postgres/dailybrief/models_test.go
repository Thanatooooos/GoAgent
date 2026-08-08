package dailybrief

import (
	"testing"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
)

func TestDailyBriefModelTableNames(t *testing.T) {
	t.Parallel()

	if got := (models.SubscriptionModel{}).TableName(); got != "t_daily_brief_subscription" {
		t.Fatalf("unexpected subscription table name: %q", got)
	}
	if got := (models.IssueModel{}).TableName(); got != "t_daily_brief_issue" {
		t.Fatalf("unexpected issue table name: %q", got)
	}
	if got := (models.ItemModel{}).TableName(); got != "t_daily_brief_item" {
		t.Fatalf("unexpected item table name: %q", got)
	}
	if got := (models.GenerationRunModel{}).TableName(); got != "t_daily_brief_generation_run" {
		t.Fatalf("unexpected generation run table name: %q", got)
	}
}
