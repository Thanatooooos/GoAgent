package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type MemoryItemModel struct {
	ID              string                `gorm:"column:id;type:varchar(20);primaryKey"`
	UserID          string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_memory_item_user_scope_status_time,priority:1;index:idx_memory_item_user_scope_id_status,priority:1"`
	ScopeType       string                `gorm:"column:scope_type;type:varchar(16);not null;index:idx_memory_item_user_scope_status_time,priority:2;index:idx_memory_item_user_scope_id_status,priority:2"`
	ScopeID         string                `gorm:"column:scope_id;type:varchar(20);index:idx_memory_item_user_scope_id_status,priority:3"`
	MemoryType      string                `gorm:"column:memory_type;type:varchar(16);not null"`
	SourceMessageID string                `gorm:"column:source_message_id;type:varchar(20);index:idx_memory_item_source_message"`
	Content         string                `gorm:"column:content;type:text;not null"`
	Summary         string                `gorm:"column:summary;type:text"`
	Confidence      float64               `gorm:"column:confidence;not null;default:1"`
	Status          string                `gorm:"column:status;type:varchar(16);not null;index:idx_memory_item_user_scope_status_time,priority:3;index:idx_memory_item_user_scope_id_status,priority:4"`
	LastConfirmedAt *time.Time            `gorm:"column:last_confirmed_at"`
	ExpiresAt       *time.Time            `gorm:"column:expires_at"`
	CreatedBy       string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy       string                `gorm:"column:updated_by;type:varchar(20);not null"`
	CreateTime      time.Time             `gorm:"column:create_time;not null"`
	UpdateTime      time.Time             `gorm:"column:update_time;not null;index:idx_memory_item_user_scope_status_time,priority:4"`
	Deleted         soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (MemoryItemModel) TableName() string {
	return "t_memory_item"
}
