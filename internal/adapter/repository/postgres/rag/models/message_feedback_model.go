package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type MessageFeedbackModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	MessageID      string                `gorm:"column:message_id;type:varchar(20);not null;uniqueIndex:uk_msg_user,priority:1"`
	ConversationID string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_conversation_id"`
	UserID         string                `gorm:"column:user_id;type:varchar(20);not null;uniqueIndex:uk_msg_user,priority:2;index:idx_user_id"`
	Vote           int16                 `gorm:"column:vote;not null"`
	Reason         string                `gorm:"column:reason;type:varchar(255)"`
	Comment        string                `gorm:"column:comment;type:varchar(1024)"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (MessageFeedbackModel) TableName() string {
	return "t_message_feedback"
}
