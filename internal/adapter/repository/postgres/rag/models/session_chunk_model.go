package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type SessionChunkModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	ConversationID string                `gorm:"column:conversation_id;type:varchar(20);not null;index:idx_session_chunk_conversation_time,priority:1"`
	MessageID      string                `gorm:"column:message_id;type:varchar(20);not null;uniqueIndex:uk_session_chunk_message_index,priority:1;index:idx_session_chunk_message"`
	UserID         string                `gorm:"column:user_id;type:varchar(20);not null;index:idx_session_chunk_conversation_time,priority:2"`
	ChunkIndex     int                   `gorm:"column:chunk_index;not null;uniqueIndex:uk_session_chunk_message_index,priority:2"`
	Content        string                `gorm:"column:content;type:text;not null"`
	ContentSummary string                `gorm:"column:content_summary;type:text"`
	TokenEstimate  int                   `gorm:"column:token_estimate;not null;default:0"`
	CreateTime     time.Time             `gorm:"column:create_time;not null;index:idx_session_chunk_conversation_time,priority:3"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (SessionChunkModel) TableName() string {
	return "t_session_chunk"
}
