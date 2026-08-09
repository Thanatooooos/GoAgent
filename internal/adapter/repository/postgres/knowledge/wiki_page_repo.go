package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/knowledge/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

type WikiPageRepository struct {
	db *gorm.DB
}

func NewWikiPageRepository(db *gorm.DB) port.WikiPageRepository {
	return &WikiPageRepository{db: db}
}

func (r *WikiPageRepository) Upsert(ctx context.Context, page domain.WikiPage) (domain.WikiPage, error) {
	var existing models.WikiPageModel
	err := r.db.WithContext(ctx).Where("kb_id = ? AND slug = ?", page.KnowledgeBaseID, page.Slug).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.WikiPage{}, fmt.Errorf("find wiki page by slug for upsert: %w", err)
	}
	page.ID, err = r.resolvePageID(existing.ID, page.ID)
	if err != nil {
		return domain.WikiPage{}, err
	}
	if existing.ID != "" {
		page.CreatedAt = existing.CreateTime
	} else if page.CreatedAt.IsZero() {
		page.CreatedAt = time.Now()
	}
	page.UpdatedAt = time.Now()
	model := toWikiPageModel(page)
	if err := r.db.WithContext(ctx).Save(&model).Error; err != nil {
		return domain.WikiPage{}, fmt.Errorf("upsert wiki page: %w", err)
	}
	return toWikiPageDomain(model), nil
}

// resolvePageID 决定 upsert 使用的页面 ID：已存在行优先，其次调用方传入，
// 否则生成新的分布式 ID。
func (r *WikiPageRepository) resolvePageID(existingID, pageID string) (string, error) {
	if existingID != "" {
		return existingID, nil
	}
	if pageID != "" {
		return pageID, nil
	}
	id, err := distributedid.NextID()
	if err != nil {
		return "", fmt.Errorf("generate wiki page id: %w", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func (r *WikiPageRepository) GetBySlug(ctx context.Context, kbID, slug string) (domain.WikiPage, error) {
	var model models.WikiPageModel
	err := r.db.WithContext(ctx).Where("kb_id = ? AND slug = ?", kbID, slug).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

func (r *WikiPageRepository) ListBySlugs(ctx context.Context, kbID string, slugs []string) ([]domain.WikiPage, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	var rows []models.WikiPageModel
	if err := r.db.WithContext(ctx).Where("kb_id = ? AND slug IN ?", kbID, slugs).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list wiki pages by slugs: %w", err)
	}
	pages := make([]domain.WikiPage, 0, len(rows))
	for _, row := range rows {
		pages = append(pages, toWikiPageDomain(row))
	}
	return pages, nil
}

func (r *WikiPageRepository) UpdateLinkCounts(ctx context.Context, kbID string, counts map[string]domain.WikiLinkCounts) error {
	if len(counts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for pageID, c := range counts {
			if err := tx.Model(&models.WikiPageModel{}).
				Where("kb_id = ? AND id = ?", kbID, pageID).
				UpdateColumns(map[string]any{"in_links": c.In, "out_links": c.Out, "update_time": time.Now()}).Error; err != nil {
				return fmt.Errorf("update wiki page link counts: %w", err)
			}
		}
		return nil
	})
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
		InLinks:           page.InLinks,
		OutLinks:          page.OutLinks,
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
		InLinks:           model.InLinks,
		OutLinks:          model.OutLinks,
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
