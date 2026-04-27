package schedule

import (
	"context"
	"errors"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type stubKnowledgeDocumentRepository struct {
	updateWhereFn func(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error)
}

func (s stubKnowledgeDocumentRepository) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	if s.updateWhereFn != nil {
		return s.updateWhereFn(ctx, cond, patch)
	}
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (s stubKnowledgeDocumentRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	return nil, nil
}

func TestDocumentStatusHelperTryMarkRunning(t *testing.T) {
	t.Parallel()

	helper := NewDocumentStatusHelper(stubKnowledgeDocumentRepository{
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
			if cond.ID != "doc-1" {
				t.Fatalf("unexpected doc id: %q", cond.ID)
			}
			if cond.Enabled == nil || !*cond.Enabled {
				t.Fatalf("expected enabled condition, got %+v", cond.Enabled)
			}
			if cond.Deleted == nil || *cond.Deleted {
				t.Fatalf("expected deleted=false condition, got %+v", cond.Deleted)
			}
			if cond.StatusNE != domain.KnowledgeDocumentStatusRunning {
				t.Fatalf("unexpected status condition: %q", cond.StatusNE)
			}
			if !patch.Status.Set || patch.Status.Value != domain.KnowledgeDocumentStatusRunning {
				t.Fatalf("unexpected status patch: %+v", patch.Status)
			}
			if !patch.UpdatedBy.Set || patch.UpdatedBy.Value != systemUser {
				t.Fatalf("unexpected updated_by patch: %+v", patch.UpdatedBy)
			}
			if !patch.UpdatedAt.Set {
				t.Fatal("expected updated_at patch to be set")
			}
			return 1, nil
		},
	})

	ok, err := helper.TryMarkRunning(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("TryMarkRunning() error = %v", err)
	}
	if !ok {
		t.Fatal("TryMarkRunning() should return true when a row is updated")
	}
}

func TestDocumentStatusHelperTryMarkRunningReturnsFalseWhenNoRowsUpdated(t *testing.T) {
	t.Parallel()

	helper := NewDocumentStatusHelper(stubKnowledgeDocumentRepository{
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
			return 0, nil
		},
	})

	ok, err := helper.TryMarkRunning(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("TryMarkRunning() error = %v", err)
	}
	if ok {
		t.Fatal("TryMarkRunning() should return false when no row is updated")
	}
}

func TestDocumentStatusHelperTryMarkRunningWrapsRepositoryError(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("boom")
	helper := NewDocumentStatusHelper(stubKnowledgeDocumentRepository{
		updateWhereFn: func(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
			return 0, repoErr
		},
	})

	ok, err := helper.TryMarkRunning(context.Background(), "doc-1")
	if err == nil {
		t.Fatal("TryMarkRunning() should return error")
	}
	if ok {
		t.Fatal("TryMarkRunning() should return false on error")
	}
}
