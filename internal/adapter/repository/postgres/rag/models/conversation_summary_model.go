package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type ConversationSummaryModel struct {
	ID                    string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID        string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_conv_user,priority:1"`
	UserID                string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_conv_user,priority:2"`
	LastMessageID         string                `gorm:"column:last_message_id;type:varchar(20);not null"`
	Content               string                `gorm:"column:content;type:text;not null"`
	StructuredSummaryJSON string                `gorm:"column:structured_summary_json;type:text"`
	SummaryVersion        int                   `gorm:"column:summary_version;not null;default:1"`
	CoveredFromMessageID  string                `gorm:"column:covered_from_message_id;type:varchar(20)"`
	CoveredToMessageID    string                `gorm:"column:covered_to_message_id;type:varchar(20)"`
	SourceMessageCount    int                   `gorm:"column:source_message_count;not null;default:0"`
	QualityStatus         string                `gorm:"column:quality_status;type:varchar(32);not null;default:unchecked"`
	LastRebuildReason     string                `gorm:"column:last_rebuild_reason;type:text"`
	CreateTime            time.Time             `gorm:"column:create_time;not null"`
	UpdateTime            time.Time             `gorm:"column:update_time;not null"`
	Deleted               soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (ConversationSummaryModel) TableName() string {
	return "t_conversation_summary"
}
