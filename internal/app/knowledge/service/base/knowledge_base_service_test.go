package base

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type stubKnowledgeBaseRepository struct {
	createFn      func(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	updateFn      func(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	updateWhereFn func(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error)
	deleteFn      func(ctx context.Context, id string) error
	getByIDFn     func(ctx context.Context, id string) (domain.KnowledgeBase, error)
	getByNameFn   func(ctx context.Context, name string) (int, error)
	countFn       func(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error)
	listFn        func(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error)
}

func (s stubKnowledgeBaseRepository) Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	if s.createFn != nil {
		return s.createFn(ctx, knowledgeBase)
	}
	return knowledgeBase, nil
}

func (s stubKnowledgeBaseRepository) Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, knowledgeBase)
	}
	return knowledgeBase, nil
}

func (s stubKnowledgeBaseRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error) {
	if s.updateWhereFn != nil {
		return s.updateWhereFn(ctx, cond, patch)
	}
	return 1, nil
}

func (s stubKnowledgeBaseRepository) Delete(ctx context.Context, id string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

func (s stubKnowledgeBaseRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return domain.KnowledgeBase{}, nil
}

func (s stubKnowledgeBaseRepository) GetByName(ctx context.Context, name string) (int, error) {
	if s.getByNameFn != nil {
		return s.getByNameFn(ctx, name)
	}
	return 0, nil
}

func (s stubKnowledgeBaseRepository) Count(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error) {
	if s.countFn != nil {
		return s.countFn(ctx, filter)
	}
	return 0, nil
}

func (s stubKnowledgeBaseRepository) List(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type stubKnowledgeDocumentRepository struct {
	countByKnowledgeBaseIDFn        func(ctx context.Context, knowledgeBaseID string) (int, error)
	countChunkedByKnowledgeBaseIDFn func(ctx context.Context, knowledgeBaseID string) (int, error)
}

func (s stubKnowledgeDocumentRepository) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentConditions, patch port.KnowledgeDocumentPatch) (int64, error) {
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) UpdateFields(ctx context.Context, where port.UpdatePredicates, set port.UpdateAssignments) (int64, error) {
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (s stubKnowledgeDocumentRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	return domain.KnowledgeDocument{}, nil
}

func (s stubKnowledgeDocumentRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	if s.countByKnowledgeBaseIDFn != nil {
		return s.countByKnowledgeBaseIDFn(ctx, knowledgeBaseID)
	}
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	if s.countChunkedByKnowledgeBaseIDFn != nil {
		return s.countChunkedByKnowledgeBaseIDFn(ctx, knowledgeBaseID)
	}
	return 0, nil
}

func (s stubKnowledgeDocumentRepository) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	return nil, nil
}

func TestKnowledgeBaseServiceCreateGeneratesCollectionName(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			createFn: func(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
				if knowledgeBase.Name != "Knowledge Base" {
					t.Fatalf("unexpected knowledge base name: %q", knowledgeBase.Name)
				}
				if knowledgeBase.EmbeddingModel != "text-embedding-3-large" {
					t.Fatalf("unexpected embedding model: %q", knowledgeBase.EmbeddingModel)
				}
				if !strings.HasPrefix(knowledgeBase.CollectionName, "knowledge_base_") {
					t.Fatalf("unexpected collection name: %q", knowledgeBase.CollectionName)
				}
				if knowledgeBase.CreatedBy != "u-1" || knowledgeBase.UpdatedBy != "u-1" {
					t.Fatalf("unexpected operator ids: created=%q updated=%q", knowledgeBase.CreatedBy, knowledgeBase.UpdatedBy)
				}
				return knowledgeBase, nil
			},
		},
		stubKnowledgeDocumentRepository{},
	)

	created, err := svc.Create(context.Background(), CreateKnowledgeBaseInput{
		Name:           "Knowledge Base",
		EmbeddingModel: "text-embedding-3-large",
		OperatorID:     "u-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create() returned empty id")
	}
}

func TestKnowledgeBaseServiceCreateUsesProvidedCollectionName(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			createFn: func(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error) {
				if knowledgeBase.CollectionName != "productdocs" {
					t.Fatalf("unexpected collection name: %q", knowledgeBase.CollectionName)
				}
				return knowledgeBase, nil
			},
		},
		stubKnowledgeDocumentRepository{},
	)

	_, err := svc.Create(context.Background(), CreateKnowledgeBaseInput{
		Name:           "Product Docs",
		EmbeddingModel: "model",
		CollectionName: "productdocs",
		OperatorID:     "u-1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestKnowledgeBaseServiceCreateRejectsDuplicateName(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			getByNameFn: func(ctx context.Context, name string) (int, error) {
				return 1, nil
			},
		},
		stubKnowledgeDocumentRepository{},
	)

	_, err := svc.Create(context.Background(), CreateKnowledgeBaseInput{
		Name:           "dup",
		EmbeddingModel: "model",
		OperatorID:     "u-1",
	})
	if err == nil || err.Error() != "knowledge base name already exists" {
		t.Fatalf("Create() duplicate error = %v", err)
	}
}

func TestKnowledgeBaseServiceUpdateRejectsEmbeddingModelChangeAfterChunkedDocsExist(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeBase, error) {
				return domain.NewKnowledgeBase(id, "kb", "old-model", "kb_1", "creator"), nil
			},
			updateWhereFn: func(ctx context.Context, cond port.KnowledgeBaseConditions, patch port.KnowledgeBasePatch) (int64, error) {
				return 1, nil
			},
		},
		stubKnowledgeDocumentRepository{
			countChunkedByKnowledgeBaseIDFn: func(ctx context.Context, knowledgeBaseID string) (int, error) {
				return 2, nil
			},
		},
	)

	_, err := svc.Update(context.Background(), UpdateKnowledgeBaseInput{
		ID:             "1",
		EmbeddingModel: "new-model",
		OperatorID:     "u-2",
	})
	if err == nil || err.Error() != "embedding model cannot be modified after chunked documents exist" {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestKnowledgeBaseServiceDeleteRejectsWhenDocumentsExist(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			getByIDFn: func(ctx context.Context, id string) (domain.KnowledgeBase, error) {
				return domain.NewKnowledgeBase(id, "kb", "model", "kb_1", "creator"), nil
			},
		},
		stubKnowledgeDocumentRepository{
			countByKnowledgeBaseIDFn: func(ctx context.Context, knowledgeBaseID string) (int, error) {
				return 1, nil
			},
		},
	)

	err := svc.Delete(context.Background(), DeleteKnowledgeBaseInput{ID: "1"})
	if err == nil || err.Error() != "knowledge base cannot be deleted while documents exist" {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestKnowledgeBaseServicePageNormalizesPagination(t *testing.T) {
	t.Parallel()

	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			countFn: func(ctx context.Context, filter port.KnowledgeBaseListFilter) (int, error) {
				if filter.Query != "kb" {
					t.Fatalf("unexpected filter query: %q", filter.Query)
				}
				if filter.Offset != 0 || filter.Limit != maxKnowledgeBasePageSize {
					t.Fatalf("unexpected pagination: offset=%d limit=%d", filter.Offset, filter.Limit)
				}
				return 1, nil
			},
			listFn: func(ctx context.Context, filter port.KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error) {
				return []domain.KnowledgeBase{{ID: "1", Name: "kb"}}, nil
			},
		},
		stubKnowledgeDocumentRepository{},
	)

	result, err := svc.Page(context.Background(), PageKnowledgeBaseInput{
		Page:     0,
		PageSize: 999,
		Query:    " kb ",
	})
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	if result.Page != 1 || result.PageSize != maxKnowledgeBasePageSize || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected page result: %+v", result)
	}
}

func TestKnowledgeBaseServiceWrapsRepositoryError(t *testing.T) {
	t.Parallel()

	repoErr := errors.New("boom")
	svc := NewKnowledgeBaseService(
		stubKnowledgeBaseRepository{
			getByNameFn: func(ctx context.Context, name string) (int, error) {
				return 0, repoErr
			},
		},
		stubKnowledgeDocumentRepository{},
	)

	_, err := svc.Create(context.Background(), CreateKnowledgeBaseInput{
		Name:           "kb",
		EmbeddingModel: "model",
		OperatorID:     "u-1",
	})
	if err == nil || err.Error() != "failed to check knowledge base name" {
		t.Fatalf("Create() wrapped error = %v", err)
	}
	if !errors.Is(err, repoErr) {
		t.Fatalf("Create() should unwrap repository error, got %v", err)
	}
}
