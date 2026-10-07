package models

import "time"

type SubscriptionModel struct {
	UserID            string     `gorm:"column:user_id;type:varchar(20);primaryKey"`
	Enabled           int16      `gorm:"column:enabled;index:idx_daily_brief_subscription_enabled"`
	Timezone          string     `gorm:"column:timezone;type:varchar(64);not null"`
	DeliveryTimeLocal string     `gorm:"column:delivery_time_local;type:varchar(8);not null"`
	TopicsJSON        []string   `gorm:"column:topics_json;type:jsonb;serializer:json;not null"`
	SourcesJSON       []string   `gorm:"column:sources_json;type:jsonb;serializer:json;not null"`
	LockOwner         *string    `gorm:"column:lock_owner;type:varchar(128)"`
	LockUntil         *time.Time `gorm:"column:lock_until;index:idx_daily_brief_subscription_lock_until"`
	CreateTime        time.Time  `gorm:"column:create_time;not null"`
	UpdateTime        time.Time  `gorm:"column:update_time;not null"`
}

func (SubscriptionModel) TableName() string {
	return "t_daily_brief_subscription"
}
