package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type ConversationModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID string                `gorm:"column:conversation_id;type:varchar(20);not null;uniqueIndex:uk_conversation_user,priority:1"`
	UserID         string                `gorm:"column:user_id;type:varchar(20);not null;uniqueIndex:uk_conversation_user,priority:2;index:idx_user_time,priority:1"`
	Title          string                `gorm:"column:title;type:varchar(128);not null"`
	LastTime       *time.Time            `gorm:"column:last_time;index:idx_user_time,priority:2"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (ConversationModel) TableName() string {
	return "t_conversation"
}
