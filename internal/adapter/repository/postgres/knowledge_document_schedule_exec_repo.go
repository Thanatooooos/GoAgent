package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentScheduleExecRepository struct {
	db *gorm.DB
}

func NewKnowledgeDocumentScheduleExecRepository(db *gorm.DB) *KnowledgeDocumentScheduleExecRepository {
	return &KnowledgeDocumentScheduleExecRepository{db: db}
}

func (r *KnowledgeDocumentScheduleExecRepository) Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	model := toKnowledgeDocumentScheduleExecModel(exec)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeDocumentScheduleExec{}, fmt.Errorf("create knowledge document schedule exec: %w", err)
	}
	return toKnowledgeDocumentScheduleExecDomain(model), nil
}

func (r *KnowledgeDocumentScheduleExecRepository) Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error) {
	rows, err := r.UpdateWhere(ctx, port.KnowledgeDocumentScheduleExecConditions{ID: exec.ID}, port.KnowledgeDocumentScheduleExecPatch{
		Status:       port.ValueOf(exec.Status),
		Message:      port.ValueOf(exec.Message),
		StartTime:    port.ValueOf(exec.StartTime),
		EndTime:      port.ValueOf(exec.EndTime),
		FileName:     port.ValueOf(exec.FileName),
		FileSize:     port.ValueOf(exec.FileSize),
		ContentHash:  port.ValueOf(exec.ContentHash),
		ETag:         port.ValueOf(exec.ETag),
		LastModified: port.ValueOf(exec.LastModified),
		UpdatedAt:    port.ValueOf(exec.UpdatedAt),
	})
	if err != nil {
		return domain.KnowledgeDocumentScheduleExec{}, err
	}
	if rows == 0 {
		return domain.KnowledgeDocumentScheduleExec{}, fmt.Errorf("update knowledge document schedule exec: no rows affected")
	}
	return exec, nil
}

func (r *KnowledgeDocumentScheduleExecRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleExecConditions, patch port.KnowledgeDocumentScheduleExecPatch) (int64, error) {
	updates := buildKnowledgeDocumentScheduleExecUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}

	query := r.applyKnowledgeDocumentScheduleExecConditions(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentScheduleExecModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge document schedule exec with conditions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeDocumentScheduleExecRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error) {
	var model models.KnowledgeDocumentScheduleExecModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeDocumentScheduleExec{}, nil
	}
	if err != nil {
		return domain.KnowledgeDocumentScheduleExec{}, fmt.Errorf("get knowledge document schedule exec by id: %w", err)
	}
	return toKnowledgeDocumentScheduleExecDomain(model), nil
}

func (r *KnowledgeDocumentScheduleExecRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeDocumentScheduleExecModel{}, "doc_id = ?", documentID).Error; err != nil {
		return fmt.Errorf("delete knowledge document schedule exec by doc id: %w", err)
	}
	return nil
}

func (r *KnowledgeDocumentScheduleExecRepository) List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error) {
	query := r.applyKnowledgeDocumentScheduleExecListFilter(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentScheduleExecModel{}), filter).
		Order("start_time desc")

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.KnowledgeDocumentScheduleExecModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list knowledge document schedule execs: %w", err)
	}

	result := make([]domain.KnowledgeDocumentScheduleExec, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeDocumentScheduleExecDomain(item))
	}
	return result, nil
}

func (r *KnowledgeDocumentScheduleExecRepository) applyKnowledgeDocumentScheduleExecListFilter(query *gorm.DB, filter port.KnowledgeDocumentScheduleExecListFilter) *gorm.DB {
	if filter.ScheduleID != "" {
		query = query.Where("schedule_id = ?", filter.ScheduleID)
	}
	if filter.DocumentID != "" {
		query = query.Where("doc_id = ?", filter.DocumentID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	return query
}

func (r *KnowledgeDocumentScheduleExecRepository) applyKnowledgeDocumentScheduleExecConditions(query *gorm.DB, cond port.KnowledgeDocumentScheduleExecConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.ScheduleID != "" {
		query = query.Where("schedule_id = ?", cond.ScheduleID)
	}
	if cond.DocumentID != "" {
		query = query.Where("doc_id = ?", cond.DocumentID)
	}
	if cond.StatusEQ != "" {
		query = query.Where("status = ?", cond.StatusEQ)
	}
	if cond.StatusNE != "" {
		query = query.Where("status <> ?", cond.StatusNE)
	}
	return query
}

func buildKnowledgeDocumentScheduleExecUpdates(patch port.KnowledgeDocumentScheduleExecPatch) map[string]any {
	updates := map[string]any{}
	if patch.Status.Set {
		updates["status"] = patch.Status.Value
	}
	if patch.Message.Set {
		updates["message"] = patch.Message.Value
	}
	if patch.StartTime.Set {
		updates["start_time"] = patch.StartTime.Value
	}
	if patch.EndTime.Set {
		updates["end_time"] = patch.EndTime.Value
	}
	if patch.FileName.Set {
		updates["file_name"] = patch.FileName.Value
	}
	if patch.FileSize.Set {
		updates["file_size"] = patch.FileSize.Value
	}
	if patch.ContentHash.Set {
		updates["content_hash"] = patch.ContentHash.Value
	}
	if patch.ETag.Set {
		updates["etag"] = patch.ETag.Value
	}
	if patch.LastModified.Set {
		updates["last_modified"] = patch.LastModified.Value
	}
	if patch.UpdatedAt.Set {
		updates["update_time"] = patch.UpdatedAt.Value
	}
	return updates
}
