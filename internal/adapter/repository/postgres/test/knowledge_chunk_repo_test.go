package postgres_test

import (
	"context"
	"testing"
	"time"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	"local/rag-project/internal/app/knowledge/port"
)

func TestKnowledgeChunkRepositoryImplementsPort(t *testing.T) {
	t.Parallel()

	var _ port.KnowledgeChunkRepository = postgresknowledge.NewKnowledgeChunkRepository(nil)
}

func TestKnowledgeChunkRepositoryCreateBatchAcceptsEmptySlice(t *testing.T) {
	t.Parallel()

	repo := postgresknowledge.NewKnowledgeChunkRepository(nil)
	if err := repo.CreateBatch(context.Background(), nil); err != nil {
		t.Fatalf("CreateBatch(nil) error = %v", err)
	}
}

func TestKnowledgeChunkRepositoryUpdateEnabledByIDsAcceptsEmptySlice(t *testing.T) {
	t.Parallel()

	repo := postgresknowledge.NewKnowledgeChunkRepository(nil)
	rows, err := repo.UpdateEnabledByIDs(context.Background(), "doc-1", nil, true, "tester", time.Now())
	if err != nil {
		t.Fatalf("UpdateEnabledByIDs(nil) error = %v", err)
	}
	if rows != 0 {
		t.Fatalf("expected 0 affected rows, got %d", rows)
	}
}
