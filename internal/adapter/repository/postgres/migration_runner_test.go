package postgres

import "testing"

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
