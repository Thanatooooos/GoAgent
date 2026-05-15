package schedule

import (
	"context"
	"errors"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type stubKnowledgeDocumentRepository struct {
	updateWhereFn  func(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error)
	updateFieldsFn func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error)
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

func (s stubKnowledgeDocumentRepository) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	if s.updateFieldsFn != nil {
		return s.updateFieldsFn(ctx, where, set)
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
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
			assertPredicate(t, where, port.KnowledgeDocument.ID.Key, port.OperatorEQ, "doc-1")
			assertPredicate(t, where, port.KnowledgeDocument.Enabled.Key, port.OperatorEQ, true)
			assertPredicate(t, where, port.KnowledgeDocument.Deleted.Key, port.OperatorEQ, false)
			assertInPredicate(t, where, port.KnowledgeDocument.Status.Key, domain.KnowledgeDocumentStatusPending, domain.KnowledgeDocumentStatusFailed, domain.KnowledgeDocumentStatusSuccess)
			assertAssignment(t, set, port.KnowledgeDocument.Status.Key, domain.KnowledgeDocumentStatusRunning)
			assertAssignment(t, set, port.KnowledgeDocument.UpdatedBy.Key, systemUser)
			if !hasAssignment(set, port.KnowledgeDocument.UpdatedAt.Key) {
				t.Fatal("expected updated_at assignment to be set")
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
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
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
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
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

func TestDocumentStatusHelperRecoverStuckRunningUsesDocumentStatuses(t *testing.T) {
	t.Parallel()

	var updatedAtAssigned bool
	helper := NewDocumentStatusHelper(stubKnowledgeDocumentRepository{
		updateFieldsFn: func(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
			assertPredicate(t, where, port.KnowledgeDocument.Status.Key, port.OperatorEQ, domain.KnowledgeDocumentStatusRunning)
			assertAssignment(t, set, port.KnowledgeDocument.Status.Key, domain.KnowledgeDocumentStatusFailed)
			assertAssignment(t, set, port.KnowledgeDocument.UpdatedBy.Key, systemUser)
			updatedAtAssigned = hasAssignment(set, port.KnowledgeDocument.UpdatedAt.Key)
			return 2, nil
		},
	})

	affected, err := helper.RecoverStuckRunning(context.Background(), 30)
	if err != nil {
		t.Fatalf("RecoverStuckRunning() error = %v", err)
	}
	if affected != 2 {
		t.Fatalf("RecoverStuckRunning() affected = %d, want 2", affected)
	}
	if updatedAtAssigned {
		t.Fatal("RecoverStuckRunning() should not assign updated_at")
	}
}

func assertPredicate(t *testing.T, predicates port.UpdatePredicates, field port.FieldKey, operator port.PredicateOperator, value any) {
	t.Helper()
	for _, predicate := range predicates {
		if predicate.Field == field && predicate.Operator == operator && predicate.Value == value {
			return
		}
	}
	t.Fatalf("missing predicate field=%s operator=%s value=%v in %+v", field, operator, value, predicates)
}

func assertAssignment(t *testing.T, assignments port.UpdateAssignments, field port.FieldKey, value any) {
	t.Helper()
	for _, assignment := range assignments {
		if assignment.Field == field && assignment.Value == value {
			return
		}
	}
	t.Fatalf("missing assignment field=%s value=%v in %+v", field, value, assignments)
}

func assertInPredicate(t *testing.T, predicates port.UpdatePredicates, field port.FieldKey, values ...string) {
	t.Helper()
	for _, predicate := range predicates {
		if predicate.Field != field || predicate.Operator != port.OperatorIn {
			continue
		}
		if len(predicate.Values) != len(values) {
			continue
		}
		matched := true
		for i, value := range values {
			if predicate.Values[i] != value {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("missing in predicate field=%s values=%v in %+v", field, values, predicates)
}

func hasAssignment(assignments port.UpdateAssignments, field port.FieldKey) bool {
	for _, assignment := range assignments {
		if assignment.Field == field {
			return true
		}
	}
	return false
}
