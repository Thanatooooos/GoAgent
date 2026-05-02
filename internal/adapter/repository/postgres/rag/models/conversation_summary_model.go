package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type ConversationSummaryModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_conv_user,priority:1"`
	UserID         string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_conv_user,priority:2"`
	LastMessageID  string                `gorm:"column:last_message_id;type:varchar(20);not null"`
	Content        string                `gorm:"column:content;type:text;not null"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (ConversationSummaryModel) TableName() string {
	return "t_conversation_summary"
}
