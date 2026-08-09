package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type WikiLinkModel struct {
	ID              string                `gorm:"column:id;type:varchar(20);primaryKey"`
	KnowledgeBaseID string                `gorm:"column:kb_id;type:varchar(20);not null;index"`
	FromPageID      string                `gorm:"column:from_page_id;type:varchar(256);not null;index"`
	ToPageID        string                `gorm:"column:to_page_id;type:varchar(256);index"`
	TargetType      string                `gorm:"column:target_type;type:varchar(16);not null;default:wiki"`
	Anchor          string                `gorm:"column:anchor;type:varchar(256);not null;default:''"`
	CreateTime      time.Time             `gorm:"column:create_time;not null"`
	Deleted         soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (WikiLinkModel) TableName() string {
	return "t_wiki_link"
}
