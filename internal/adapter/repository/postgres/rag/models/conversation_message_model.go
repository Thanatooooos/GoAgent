package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type ConversationMessageModel struct {
	ID               string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID   string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_conversation_user_time,priority:1;index:idx_conversation_summary,priority:1"`
	UserID           string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_conversation_user_time,priority:2;index:idx_conversation_summary,priority:2"`
	Role             string                `gorm:"column:role;type:varchar(16);not null"`
	Content          string                `gorm:"column:content;type:text;not null"`
	RawContent       string                `gorm:"column:raw_content;type:text"`
	ContentSummary   string                `gorm:"column:content_summary;type:text"`
	IsSummarized     bool                  `gorm:"column:is_summarized;not null;default:false"`
	ThinkingContent  string                `gorm:"column:thinking_content;type:text"`
	ThinkingDuration *int                  `gorm:"column:thinking_duration"`
	CreateTime       time.Time             `gorm:"column:create_time;not null;index:idx_conversation_user_time,priority:3;index:idx_conversation_summary,priority:3"`
	UpdateTime       time.Time             `gorm:"column:update_time;not null"`
	Deleted          soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (ConversationMessageModel) TableName() string {
	return "t_message"
}
