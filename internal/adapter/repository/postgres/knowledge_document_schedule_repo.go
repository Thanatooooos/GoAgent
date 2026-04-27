package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"local/rag-project/internal/adapter/repository/postgres/models"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type KnowledgeDocumentScheduleRepository struct {
	db *gorm.DB
}

func NewKnowledgeDocumentScheduleRepository(db *gorm.DB) *KnowledgeDocumentScheduleRepository {
	return &KnowledgeDocumentScheduleRepository{db: db}
}

func (r *KnowledgeDocumentScheduleRepository) Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	model := toKnowledgeDocumentScheduleModel(schedule)
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return domain.KnowledgeDocumentSchedule{}, fmt.Errorf("create knowledge document schedule: %w", err)
	}
	return toKnowledgeDocumentScheduleDomain(model), nil
}

func (r *KnowledgeDocumentScheduleRepository) Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error) {
	rows, err := r.UpdateWhere(ctx, port.KnowledgeDocumentScheduleConditions{ID: schedule.ID}, port.KnowledgeDocumentSchedulePatch{
		KnowledgeBaseID: port.ValueOf(schedule.KnowledgeBaseID),
		CronExpr:        port.ValueOf(schedule.CronExpr),
		Enabled:         port.ValueOf(schedule.Enabled),
		NextRunTime:     port.ValueOf(schedule.NextRunTime),
		LastRunTime:     port.ValueOf(schedule.LastRunTime),
		LastSuccessTime: port.ValueOf(schedule.LastSuccessTime),
		LastStatus:      port.ValueOf(schedule.LastStatus),
		LastError:       port.ValueOf(schedule.LastError),
		LastETag:        port.ValueOf(schedule.LastETag),
		LastModified:    port.ValueOf(schedule.LastModified),
		LastContentHash: port.ValueOf(schedule.LastContentHash),
		LockOwner:       port.ValueOf(stringPointer(schedule.LockOwner)),
		LockUntil:       port.ValueOf(schedule.LockUntil),
		UpdatedAt:       port.ValueOf(schedule.UpdatedAt),
	})
	if err != nil {
		return domain.KnowledgeDocumentSchedule{}, err
	}
	if rows == 0 {
		return domain.KnowledgeDocumentSchedule{}, fmt.Errorf("update knowledge document schedule: no rows affected")
	}
	return schedule, nil
}

func (r *KnowledgeDocumentScheduleRepository) UpdateWhere(ctx context.Context, cond port.KnowledgeDocumentScheduleConditions, patch port.KnowledgeDocumentSchedulePatch) (int64, error) {
	updates := buildKnowledgeDocumentScheduleUpdates(patch)
	if len(updates) == 0 {
		return 0, nil
	}

	query := r.applyKnowledgeDocumentScheduleConditions(r.db.WithContext(ctx).Model(&models.KnowledgeDocumentScheduleModel{}), cond)
	result := query.Updates(updates)
	if result.Error != nil {
		return 0, fmt.Errorf("update knowledge document schedule with conditions: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *KnowledgeDocumentScheduleRepository) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeDocumentScheduleModel{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete knowledge document schedule: %w", err)
	}
	return nil
}

func (r *KnowledgeDocumentScheduleRepository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	if err := r.db.WithContext(ctx).Delete(&models.KnowledgeDocumentScheduleModel{}, "doc_id = ?", documentID).Error; err != nil {
		return fmt.Errorf("delete knowledge document schedule by doc id: %w", err)
	}
	return nil
}

func (r *KnowledgeDocumentScheduleRepository) GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error) {
	return r.getOne(ctx, "id = ?", id)
}

func (r *KnowledgeDocumentScheduleRepository) GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error) {
	return r.getOne(ctx, "doc_id = ?", documentID)
}

func (r *KnowledgeDocumentScheduleRepository) ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error) {
	query := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentScheduleModel{}).
		Where("enabled = 1").
		Where("next_run_time IS NOT NULL AND next_run_time <= ?", before).
		Order("next_run_time asc")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var items []models.KnowledgeDocumentScheduleModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list due knowledge document schedules: %w", err)
	}

	result := make([]domain.KnowledgeDocumentSchedule, 0, len(items))
	for _, item := range items {
		result = append(result, toKnowledgeDocumentScheduleDomain(item))
	}
	return result, nil
}

func (r *KnowledgeDocumentScheduleRepository) TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentScheduleModel{}).
		Where("id = ?", lease.ScheduleID).
		Where("(lock_until IS NULL OR lock_until < ?)", now).
		Updates(buildKnowledgeDocumentScheduleUpdates(port.KnowledgeDocumentSchedulePatch{
			LockOwner: port.ValueOf(stringPointer(lease.LockToken)),
			LockUntil: port.ValueOf(&lockUntil),
		}))
	if result.Error != nil {
		return false, fmt.Errorf("acquire knowledge document schedule lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *KnowledgeDocumentScheduleRepository) RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentScheduleModel{}).
		Where("id = ?", lease.ScheduleID).
		Where("lock_owner = ?", lease.LockToken).
		Updates(buildKnowledgeDocumentScheduleUpdates(port.KnowledgeDocumentSchedulePatch{
			LockUntil: port.ValueOf(&lockUntil),
		}))
	if result.Error != nil {
		return false, fmt.Errorf("renew knowledge document schedule lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *KnowledgeDocumentScheduleRepository) ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.KnowledgeDocumentScheduleModel{}).
		Where("id = ?", lease.ScheduleID).
		Where("lock_owner = ?", lease.LockToken).
		Updates(buildKnowledgeDocumentScheduleUpdates(port.KnowledgeDocumentSchedulePatch{
			LockOwner: port.ValueOf((*string)(nil)),
			LockUntil: port.ValueOf((*time.Time)(nil)),
		}))
	if result.Error != nil {
		return false, fmt.Errorf("release knowledge document schedule lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *KnowledgeDocumentScheduleRepository) getOne(ctx context.Context, query string, args ...any) (domain.KnowledgeDocumentSchedule, error) {
	var model models.KnowledgeDocumentScheduleModel
	err := r.db.WithContext(ctx).Where(query, args...).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.KnowledgeDocumentSchedule{}, nil
	}
	if err != nil {
		return domain.KnowledgeDocumentSchedule{}, fmt.Errorf("get knowledge document schedule: %w", err)
	}
	return toKnowledgeDocumentScheduleDomain(model), nil
}

func (r *KnowledgeDocumentScheduleRepository) applyKnowledgeDocumentScheduleConditions(query *gorm.DB, cond port.KnowledgeDocumentScheduleConditions) *gorm.DB {
	if cond.ID != "" {
		query = query.Where("id = ?", cond.ID)
	}
	if cond.DocumentID != "" {
		query = query.Where("doc_id = ?", cond.DocumentID)
	}
	if cond.Enabled != nil {
		query = query.Where("enabled = ?", boolToFlag(*cond.Enabled))
	}
	if cond.LastStatusEQ != "" {
		query = query.Where("last_status = ?", cond.LastStatusEQ)
	}
	if cond.LockOwnerEQ != "" {
		query = query.Where("lock_owner = ?", cond.LockOwnerEQ)
	}
	return query
}

func buildKnowledgeDocumentScheduleUpdates(patch port.KnowledgeDocumentSchedulePatch) map[string]any {
	updates := map[string]any{}
	if patch.KnowledgeBaseID.Set {
		updates["kb_id"] = patch.KnowledgeBaseID.Value
	}
	if patch.CronExpr.Set {
		updates["cron_expr"] = patch.CronExpr.Value
	}
	if patch.Enabled.Set {
		updates["enabled"] = boolToFlag(patch.Enabled.Value)
	}
	if patch.NextRunTime.Set {
		updates["next_run_time"] = patch.NextRunTime.Value
	}
	if patch.LastRunTime.Set {
		updates["last_run_time"] = patch.LastRunTime.Value
	}
	if patch.LastSuccessTime.Set {
		updates["last_success_time"] = patch.LastSuccessTime.Value
	}
	if patch.LastStatus.Set {
		updates["last_status"] = patch.LastStatus.Value
	}
	if patch.LastError.Set {
		updates["last_error"] = patch.LastError.Value
	}
	if patch.LastETag.Set {
		updates["last_etag"] = patch.LastETag.Value
	}
	if patch.LastModified.Set {
		updates["last_modified"] = patch.LastModified.Value
	}
	if patch.LastContentHash.Set {
		updates["last_content_hash"] = patch.LastContentHash.Value
	}
	if patch.LockOwner.Set {
		updates["lock_owner"] = patch.LockOwner.Value
	}
	if patch.LockUntil.Set {
		updates["lock_until"] = patch.LockUntil.Value
	}
	if patch.UpdatedAt.Set {
		updates["update_time"] = patch.UpdatedAt.Value
	}
	return updates
}
