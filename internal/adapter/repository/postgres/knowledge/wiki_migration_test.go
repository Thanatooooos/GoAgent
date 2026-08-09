package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikiMigrationDefinesTablesAndUniqueSlug(t *testing.T) {
	path := filepath.Join("..", "migrations", "20260809000000_create_wiki_tables.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS t_wiki_page",
		"CREATE TABLE IF NOT EXISTS t_wiki_link",
		"CREATE UNIQUE INDEX IF NOT EXISTS uk_wiki_page_kb_slug_active",
		"from_page_id    VARCHAR(256) NOT NULL",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("migration should contain %q", token)
		}
	}
}
