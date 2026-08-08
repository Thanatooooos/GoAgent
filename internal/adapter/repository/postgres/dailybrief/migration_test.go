package dailybrief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDailyBriefMigrationDefinesCoreTablesAndIndexes(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "migrations", "20260629100000_create_daily_brief_tables.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(content)
	required := []string{
		"CREATE TABLE IF NOT EXISTS t_daily_brief_subscription",
		"CREATE TABLE IF NOT EXISTS t_daily_brief_issue",
		"CREATE TABLE IF NOT EXISTS t_daily_brief_item",
		"CREATE TABLE IF NOT EXISTS t_daily_brief_generation_run",
		"CONSTRAINT uk_daily_brief_issue_user_date UNIQUE (user_id, brief_date)",
		"lock_owner",
		"lock_until",
		"idx_daily_brief_subscription_enabled",
		"idx_daily_brief_issue_user_status",
		"idx_daily_brief_item_issue_section_rank",
		"idx_daily_brief_generation_run_user_date_started",
	}
	for _, token := range required {
		if !strings.Contains(text, token) {
			t.Fatalf("migration should contain %q", token)
		}
	}
}
