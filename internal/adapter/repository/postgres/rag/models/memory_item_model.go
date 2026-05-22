package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type MemoryItemModel struct {
	ID               string                `gorm:"column:id;type:varchar(20);primaryKey"`
	UserID           string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_memory_item_user_scope_status_time,priority:1;index:idx_memory_item_user_scope_id_status,priority:1;index:idx_memory_item_user_scope_key_status,priority:1;index:idx_memory_item_user_namespace_category_status,priority:1"`
	ScopeType        string                `gorm:"column:scope_type;type:varchar(16);not null;index:idx_memory_item_user_scope_status_time,priority:2;index:idx_memory_item_user_scope_id_status,priority:2;index:idx_memory_item_user_scope_key_status,priority:2"`
	ScopeID          string                `gorm:"column:scope_id;type:varchar(20);index:idx_memory_item_user_scope_id_status,priority:3;index:idx_memory_item_user_scope_key_status,priority:3"`
	Namespace        string                `gorm:"column:namespace;type:varchar(64);not null;default:'';index:idx_memory_item_user_namespace_category_status,priority:2"`
	MemoryType       string                `gorm:"column:memory_type;type:varchar(16);not null"`
	Category         string                `gorm:"column:category;type:varchar(32);not null;default:'';index:idx_memory_item_user_namespace_category_status,priority:3"`
	CanonicalKey     string                `gorm:"column:canonical_key;type:varchar(64);not null;default:'';index:idx_memory_item_user_scope_key_status,priority:4"`
	ValueType        string                `gorm:"column:value_type;type:varchar(16);not null;default:'text'"`
	ValueJSON        string                `gorm:"column:value_json;type:text"`
	DisplayValue     string                `gorm:"column:display_value;type:text"`
	SourceMessageID  string                `gorm:"column:source_message_id;type:varchar(20);index:idx_memory_item_source_message"`
	Content          string                `gorm:"column:content;type:text;not null"`
	Summary          string                `gorm:"column:summary;type:text"`
	Confidence       float64               `gorm:"column:confidence;not null;default:1"`
	Importance       int                   `gorm:"column:importance;not null;default:0"`
	Status           string                `gorm:"column:status;type:varchar(16);not null;index:idx_memory_item_user_scope_status_time,priority:3;index:idx_memory_item_user_scope_id_status,priority:4;index:idx_memory_item_user_scope_key_status,priority:5;index:idx_memory_item_user_namespace_category_status,priority:4"`
	LastConfirmedAt  *time.Time            `gorm:"column:last_confirmed_at"`
	LastUsedAt       *time.Time            `gorm:"column:last_used_at"`
	ExpiresAt        *time.Time            `gorm:"column:expires_at"`
	SupersedesID     string                `gorm:"column:supersedes_id;type:varchar(20);index:idx_memory_item_supersedes_id"`
	ExtractionMethod string                `gorm:"column:extraction_method;type:varchar(32);not null;default:'manual'"`
	CreatedBy        string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy        string                `gorm:"column:updated_by;type:varchar(20);not null"`
	CreateTime       time.Time             `gorm:"column:create_time;not null"`
	UpdateTime       time.Time             `gorm:"column:update_time;not null;index:idx_memory_item_user_scope_status_time,priority:4"`
	Deleted          soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (MemoryItemModel) TableName() string {
	return "t_memory_item"
}
