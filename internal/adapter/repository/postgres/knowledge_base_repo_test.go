package postgres

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"local/rag-project/internal/adapter/repository/postgres/models"
)

func TestKnowledgeBaseModelUsesSoftDeleteFlag(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeOf(models.KnowledgeBaseModel{}).FieldByName("Deleted")
	if !ok {
		t.Fatal("Deleted field not found")
	}

	tag := field.Tag.Get("gorm")
	if !strings.Contains(tag, "column:deleted") {
		t.Fatalf("Deleted field tag should target deleted column, got %q", tag)
	}
	if !strings.Contains(tag, "softDelete:flag") {
		t.Fatalf("Deleted field tag should use soft delete flag semantics, got %q", tag)
	}
}

func TestKnowledgeBaseMigrationDefinesUniqueCollectionNameAndLogicalDelete(t *testing.T) {
	t.Parallel()

	path := filepath.Join("migrations", "20260426212000_create_knowledge_tables.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(content)
	if !strings.Contains(text, "CONSTRAINT uk_collection_name UNIQUE (collection_name)") {
		t.Fatal("migration should enforce unique collection_name")
	}
	if !strings.Contains(text, "deleted         SMALLINT     NOT NULL DEFAULT 0") {
		t.Fatal("migration should define deleted flag with default 0")
	}
}
