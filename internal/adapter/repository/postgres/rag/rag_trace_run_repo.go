package rag

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type RagTraceRunRepository struct {
	db *gorm.DB
}

func NewRagTraceRunRepository(db *gorm.DB) *RagTraceRunRepository {
	return &RagTraceRunRepository{db: db}
}

func (r *RagTraceRunRepository) Create(ctx context.Context, run domain.RagTraceRun) (domain.RagTraceRun, error) {
	model := toRagTraceRunModel(run)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.RagTraceRun{}, fmt.Errorf("create rag trace run: %w", err)
	}
	return toRagTraceRunDomain(model), nil
}

func (r *RagTraceRunRepository) UpdateByTraceID(ctx context.Context, traceID string, run domain.RagTraceRun) error {
	_, err := r.UpdateWhere(ctx, port.RagTraceRunConditions{TraceID: traceID}, port.RagTraceRunPatch{
		TraceName:      port.ValueOf(run.TraceName),
		EntryMethod:    port.ValueOf(run.EntryMethod),
		ConversationID: port.ValueOf(run.ConversationID),
		TaskID:         port.ValueOf(run.TaskID),
		UserID:         port.ValueOf(run.UserID),
		Status:         port.ValueOf(run.Status),
		ErrorMessage:   port.ValueOf(run.ErrorMessage),
		StartTime:      port.ValueOf(run.StartTime),
		EndTime:        port.ValueOf(run.EndTime),
		DurationMs:     port.ValueOf(run.DurationMs),
		ExtraData:      port.ValueOf(run.ExtraData),
		UpdateTime:     port.ValueOf(run.UpdateTime),
	})
	if err != nil {
		return fmt.Errorf("update rag trace run by trace id: %w", err)
	}
	return nil
}

func (r *RagTraceRunRepository) UpdateWhere(ctx context.Context, cond port.RagTraceRunConditions, patch port.RagTraceRunPatch) (int64, error) {
	updates := buildRagTraceRunUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}
	if !hasRagTraceRunConditions(cond) {
		return 0, conditionalUpdateRequiresConditions("rag trace run")
	}

	query := applyRagTraceRunConditions(r.db.WithContext(ctx).Model(&models.RagTraceRunModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update rag trace run where: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *RagTraceRunRepository) GetByTraceID(ctx context.Context, traceID string) (domain.RagTraceRun, error) {
	var model models.RagTraceRunModel
	err := r.db.WithContext(ctx).
		Where("trace_id = ?", traceID).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.RagTraceRun{}, nil
	}
	if err != nil {
		return domain.RagTraceRun{}, fmt.Errorf("get rag trace run by trace id: %w", err)
	}
	return toRagTraceRunDomain(model), nil
}

func (r *RagTraceRunRepository) Count(ctx context.Context, filter port.RagTraceRunListFilter) (int, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.RagTraceRunModel{}), filter)
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count rag trace runs: %w", err)
	}
	return int(count), nil
}

func (r *RagTraceRunRepository) List(ctx context.Context, filter port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
	query := r.applyFilter(r.db.WithContext(ctx).Model(&models.RagTraceRunModel{}), filter).
		Order("coalesce(start_time, create_time) desc").
		Order("create_time desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.RagTraceRunModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list rag trace runs: %w", err)
	}

	result := make([]domain.RagTraceRun, 0, len(items))
	for _, item := range items {
		result = append(result, toRagTraceRunDomain(item))
	}
	return result, nil
}

func (r *RagTraceRunRepository) applyFilter(query *gorm.DB, filter port.RagTraceRunListFilter) *gorm.DB {
	if filter.TraceID != "" {
		query = query.Where("trace_id = ?", filter.TraceID)
	}
	if filter.ConversationID != "" {
		query = query.Where("conversation_id = ?", filter.ConversationID)
	}
	if filter.TaskID != "" {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	return query
}

func applyRagTraceRunConditions(query *gorm.DB, cond port.RagTraceRunConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.TraceID != "" {
		query = query.Where("trace_id = ?", cond.TraceID)
	}
	if cond.ConversationID != "" {
		query = query.Where("conversation_id = ?", cond.ConversationID)
	}
	if cond.TaskID != "" {
		query = query.Where("task_id = ?", cond.TaskID)
	}
	if cond.UserID != "" {
		query = query.Where("user_id = ?", cond.UserID)
	}
	if cond.StatusEQ != "" {
		query = query.Where("status = ?", cond.StatusEQ)
	}
	if cond.StatusNE != "" {
		query = query.Where("status <> ?", cond.StatusNE)
	}
	return query
}

func hasRagTraceRunConditions(cond port.RagTraceRunConditions) bool {
	return cond.ID != "" ||
		cond.TraceID != "" ||
		cond.ConversationID != "" ||
		cond.TaskID != "" ||
		cond.UserID != "" ||
		cond.StatusEQ != "" ||
		cond.StatusNE != ""
}

func buildRagTraceRunUpdates(patch port.RagTraceRunPatch) map[string]any {
	updates := map[string]any{}
	if patch.TraceName.Set {
		updates["trace_name"] = patch.TraceName.Value
	}
	if patch.EntryMethod.Set {
		updates["entry_method"] = patch.EntryMethod.Value
	}
	if patch.ConversationID.Set {
		updates["conversation_id"] = patch.ConversationID.Value
	}
	if patch.TaskID.Set {
		updates["task_id"] = patch.TaskID.Value
	}
	if patch.UserID.Set {
		updates["user_id"] = patch.UserID.Value
	}
	if patch.Status.Set {
		updates["status"] = patch.Status.Value
	}
	if patch.ErrorMessage.Set {
		updates["error_message"] = patch.ErrorMessage.Value
	}
	if patch.StartTime.Set {
		updates["start_time"] = patch.StartTime.Value
	}
	if patch.EndTime.Set {
		updates["end_time"] = patch.EndTime.Value
	}
	if patch.DurationMs.Set {
		updates["duration_ms"] = patch.DurationMs.Value
	}
	if patch.ExtraData.Set {
		updates["extra_data"] = patch.ExtraData.Value
	}
	if patch.UpdateTime.Set {
		updates["update_time"] = patch.UpdateTime.Value
	}
	return updates
}
