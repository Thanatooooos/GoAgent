package dailybrief

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

type SubscriptionRepository struct {
	db                *gorm.DB
	SyncScheduledTask func(*gorm.DB, domain.Subscription) error
}

func NewSubscriptionRepository(db *gorm.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) Upsert(ctx context.Context, subscription domain.Subscription) (domain.Subscription, error) {
	if r.SyncScheduledTask != nil {
		var saved domain.Subscription
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Task mutations use task -> subscription lock order too.
			if err := tx.Exec(`SELECT t.id FROM t_scheduled_task t JOIN t_daily_brief_task_binding b ON b.task_id=t.id WHERE b.user_id=? FOR UPDATE OF t`, subscription.UserID).Error; err != nil {
				return err
			}
			// Serialize against handoff before checking the binding or writing.
			if err := tx.Exec(`SELECT user_id FROM t_daily_brief_subscription WHERE user_id=? FOR UPDATE`, subscription.UserID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`SELECT set_config('app.daily_brief_sync','on',true)`).Error; err != nil {
				return err
			}
			var err error
			saved, err = NewSubscriptionRepository(tx).Upsert(ctx, subscription)
			if err != nil {
				return err
			}
			return r.SyncScheduledTask(tx, saved)
		})
		return saved, err
	}
	model := toSubscriptionModel(subscription)
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"enabled",
				"timezone",
				"delivery_time_local",
				"topics_json",
				"sources_json",
				"update_time",
			}),
		}).
		Create(&model).Error
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("upsert daily brief subscription: %w", err)
	}
	return toSubscriptionDomain(model), nil
}

func (r *SubscriptionRepository) GetByUserID(ctx context.Context, userID string) (domain.Subscription, error) {
	var model models.SubscriptionModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Subscription{}, nil
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("get daily brief subscription by user id: %w", err)
	}
	return toSubscriptionDomain(model), nil
}

func (r *SubscriptionRepository) List(ctx context.Context, filter port.SubscriptionListFilter) ([]domain.Subscription, error) {
	query := r.db.WithContext(ctx).Model(&models.SubscriptionModel{}).Order("update_time desc")
	if filter.LegacyOnly {
		query = query.Where("NOT EXISTS (SELECT 1 FROM t_daily_brief_task_binding b WHERE b.user_id = t_daily_brief_subscription.user_id)")
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", boolToFlag(*filter.Enabled))
	}
	if filter.Timezone != "" {
		query = query.Where("timezone = ?", filter.Timezone)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []models.SubscriptionModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list daily brief subscriptions: %w", err)
	}
	result := make([]domain.Subscription, 0, len(items))
	for _, item := range items {
		result = append(result, toSubscriptionDomain(item))
	}
	return result, nil
}

func (r *SubscriptionRepository) TryAcquireLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.SubscriptionModel{}).
		Where("user_id = ?", lease.UserID).
		Where("NOT EXISTS (SELECT 1 FROM t_daily_brief_task_binding WHERE user_id = ?)", lease.UserID).
		Where("(lock_until IS NULL OR lock_until < ?)", now).
		Updates(map[string]any{
			"lock_owner":  lease.LockOwner,
			"lock_until":  lockUntil,
			"update_time": now,
		})
	if result.Error != nil {
		return false, fmt.Errorf("acquire daily brief subscription lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *SubscriptionRepository) RenewLock(ctx context.Context, lease domain.SubscriptionLockLease, lockUntil time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.SubscriptionModel{}).
		Where("user_id = ?", lease.UserID).
		Where("lock_owner = ?", lease.LockOwner).
		Updates(map[string]any{
			"lock_until":  lockUntil,
			"update_time": time.Now(),
		})
	if result.Error != nil {
		return false, fmt.Errorf("renew daily brief subscription lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *SubscriptionRepository) ReleaseLock(ctx context.Context, lease domain.SubscriptionLockLease) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&models.SubscriptionModel{}).
		Where("user_id = ?", lease.UserID).
		Where("lock_owner = ?", lease.LockOwner).
		Updates(map[string]any{
			"lock_owner":  nil,
			"lock_until":  nil,
			"update_time": time.Now(),
		})
	if result.Error != nil {
		return false, fmt.Errorf("release daily brief subscription lock: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

var _ port.SubscriptionRepository = (*SubscriptionRepository)(nil)
