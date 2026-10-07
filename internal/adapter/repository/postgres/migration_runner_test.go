package postgres

import (
	"strings"
	"testing"
)

func TestSplitSQLStatementsKeepsDollarQuotedBlocksIntact(t *testing.T) {
	sql := `
CREATE TABLE demo (
    id INT PRIMARY KEY
);

DO $$
BEGIN
    IF EXISTS (SELECT 1) THEN
        PERFORM 1;
    END IF;
END
$$;

CREATE INDEX idx_demo_id ON demo (id);
`

	got := splitSQLStatements(sql)
	if len(got) != 3 {
		t.Fatalf("expected 3 statements, got %d: %#v", len(got), got)
	}
}

func TestGooseMigrationRunsOnlyUpStatements(t *testing.T) {
	sql := `-- +goose Up
ALTER TABLE t_message ADD COLUMN IF NOT EXISTS sources JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose Down
ALTER TABLE t_message DROP COLUMN IF EXISTS sources;`
	got := splitSQLStatements(migrationUpSQL(sql))
	if len(got) != 1 || !strings.Contains(got[0], "ADD COLUMN") {
		t.Fatalf("expected only Up statement, got %#v", got)
	}
}
