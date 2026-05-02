package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type RagTraceRunModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	TraceID        string                `gorm:"column:trace_id;type:varchar(64);not null;uniqueIndex:uk_run_id"`
	TraceName      string                `gorm:"column:trace_name;type:varchar(128)"`
	EntryMethod    string                `gorm:"column:entry_method;type:varchar(256)"`
	ConversationID string                `gorm:"column:conversation_id;type:varchar(20)"`
	TaskID         string                `gorm:"column:task_id;type:varchar(20);index:idx_task_id"`
	UserID         string                `gorm:"column:user_id;type:varchar(20);index:idx_user_id_trace"`
	Status         string                `gorm:"column:status;type:varchar(16);not null;default:RUNNING"`
	ErrorMessage   string                `gorm:"column:error_message;type:varchar(1000)"`
	StartTime      *time.Time            `gorm:"column:start_time"`
	EndTime        *time.Time            `gorm:"column:end_time"`
	DurationMs     *int64                `gorm:"column:duration_ms"`
	ExtraData      string                `gorm:"column:extra_data;type:text"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (RagTraceRunModel) TableName() string {
	return "t_rag_trace_run"
}
