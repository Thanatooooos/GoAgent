package knowledge

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/exception"
)

type WikiPageRepository struct {
	db *gorm.DB
}

func NewWikiPageRepository(db *gorm.DB) port.WikiPageRepository {
	return &WikiPageRepository{db: db}
}

func (r *WikiPageRepository) Upsert(ctx context.Context, page domain.WikiPage) (domain.WikiPage, error) {
	model := toWikiPageModel(page)
	err := r.db.WithContext(ctx).Where("kb_id = ? AND slug = ?", page.KnowledgeBaseID, page.Slug).Save(model).Error
	if err != nil {
		return domain.WikiPage{}, fmt.Errorf("upsert wiki page: %w", err)
	}
	return page, nil
}

func (r *WikiPageRepository) GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error) {
	var model models.WikiPageModel
	err := r.db.WithContext(ctx).Where("kb_id = ? AND slug = ?", kbID, slug).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.WikiPage{}, exception.NewClientException("wiki page not found", nil)
		}
		return domain.WikiPage{}, fmt.Errorf("get wiki page by slug: %w", err)
	}
	return toWikiPageDomain(model), nil
}

func (r *WikiPageRepository) ListByKB(ctx context.Context, kbID string, offset, limit int) ([]domain.WikiPage, int, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.WikiPageModel{}).Where("kb_id = ?", kbID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count wiki pages: %w", err)
	}
	var rows []models.WikiPageModel
	err := r.db.WithContext(ctx).Where("kb_id = ?", kbID).Order("update_time DESC").Offset(offset).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list wiki pages: %w", err)
	}
	pages := make([]domain.WikiPage, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, toWikiPageDomain(row))
	}
	return pages, int(total), nil
}

func (r *WikiPageRepository) DeleteByKB(ctx context.Context, kbID string) error {
	return r.db.WithContext(ctx).Where("kb_id = ?", kbID).Delete(&models.WikiPageModel{}).Error
}

func toWikiPageModel(page domain.WikiPage) models.WikiPageModel {
	return models.WikiPageModel{
		ID:                page.ID,
		KnowledgeBaseID:   page.KnowledgeBaseID,
		Slug:              page.Slug,
		Title:             page.Title,
		PageType:          page.PageType,
		Status:            page.Status,
		Content:           page.Content,
		Summary:           page.Summary,
		SourceDocumentIDs: mustJSONBytes(page.SourceDocumentIDs),
		SourceChunkIDs:    mustJSONBytes(page.SourceChunkIDs),
		CreatedBy:         page.CreatedBy,
		UpdatedBy:         page.UpdatedBy,
		CreateTime:        page.CreatedAt,
		UpdateTime:        page.UpdatedAt,
	}
}

func toWikiPageDomain(model models.WikiPageModel) domain.WikiPage {
	return domain.WikiPage{
		ID:                model.ID,
		KnowledgeBaseID:   model.KnowledgeBaseID,
		Slug:              model.Slug,
		Title:             model.Title,
		PageType:          model.PageType,
		Status:            model.Status,
		Content:           model.Content,
		Summary:           model.Summary,
		SourceDocumentIDs: mustStringSlice(model.SourceDocumentIDs),
		SourceChunkIDs:    mustStringSlice(model.SourceChunkIDs),
		CreatedBy:         model.CreatedBy,
		UpdatedBy:         model.UpdatedBy,
		CreatedAt:         model.CreateTime,
		UpdatedAt:         model.UpdateTime,
	}
}

func mustJSONBytes(values []string) []byte {
	if values == nil {
		return []byte("[]")
	}
	data, _ := json.Marshal(values)
	return data
}

func mustStringSlice(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var values []string
	_ = json.Unmarshal(data, &values)
	return values
}
