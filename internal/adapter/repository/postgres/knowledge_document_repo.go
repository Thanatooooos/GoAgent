package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/models"
	sqlcqueries "local/rag-project/internal/adapter/repository/postgres/sqlc"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentRepository struct {
	db      *gorm.DB
	queries *sqlcqueries.Queries
}

func NewKnowledgeDocumentRepository(db *gorm.DB, queries *sqlcqueries.Queries) *KnowledgeDocumentRepository {
	return &KnowledgeDocumentRepository{
		db:      db,
		queries: queries,
	}
}

func (r *KnowledgeDocumentRepository) Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	model := toKnowledgeDocumentModel(document)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeDocument{}, fmt.Errorf("create knowledge document: %w", err)
	}
	return toKnowledgeDocumentDomain(model), nil
}

func (r *KnowledgeDocumentRepository) Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error) {
	model := toKnowledgeDocumentModel(document)
	if err := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentModel{}).
		Where("id = ?", model.ID).
		Updates(model).
		Error; err != nil {
		return domain.KnowledgeDocument{}, fmt.Errorf("update knowledge document: %w", err)
	}
	return document, nil
}

func (r *KnowledgeDocumentRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeDocumentModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete knowledge document: %w", err)
	}
	return nil
}

func (r *KnowledgeDocumentRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error) {
	var model models.KnowledgeDocumentModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeDocument{}, nil
	}
	if err != nil {
		return domain.KnowledgeDocument{}, fmt.Errorf("get knowledge document by id: %w", err)
	}
	return toKnowledgeDocumentDomain(model), nil
}

func (r *KnowledgeDocumentRepository) CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentModel{}).
		Where("kb_id = ?", knowledgeBaseID).
		Count(&count).
		Error
	if err != nil {
		return 0, fmt.Errorf("count knowledge documents by knowledge base id: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentRepository) CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error) {
	if r.queries == nil {
		return 0, fmt.Errorf("sqlc queries is required")
	}

	count, err := r.queries.CountChunkedDocumentsByKnowledgeBaseID(ctx, knowledgeBaseID)
	if err != nil {
		return 0, fmt.Errorf("count chunked knowledge documents by knowledge base id: %w", err)
	}
	return int(count), nil
}

func (r *KnowledgeDocumentRepository) List(ctx context.Context, filter port.KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error) {
	query := r.applyKnowledgeDocumentListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentModel{}), filter).
		Order("create_time desc")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.KnowledgeDocumentModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge documents: %w", err)
	}

	result := make([]domain.KnowledgeDocument, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeDocumentDomain(item))
	}
	return result, nil
}

func (r *KnowledgeDocumentRepository) applyKnowledgeDocumentListFilter(query *gorm.DB, filter port.KnowledgeDocumentListFilter) *gorm.DB {
	if filter.KnowledgeBaseID != "" {
		query = query.Where("kb_id = ?", filter.KnowledgeBaseID)
	}
	if filter.SourceType != "" {
		query = query.Where("source_type = ?", filter.SourceType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Enabled != nil {
		if *filter.Enabled {
			query = query.Where("enabled = 1")
		} else {
			query = query.Where("enabled = 0")
		}
	}
	if filter.Query != "" {
		like := "%" + filter.Query + "%"
		query = query.Where("(doc_name ILIKE ? OR source_location ILIKE ?)", like, like)
	}
	return query
}
