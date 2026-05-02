package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type RagTraceNodeModel struct {
	ID           string                `gorm:"column:id;type:varchar(20);primaryKey"`
	TraceID      string                `gorm:"column:trace_id;type:varchar(20);not null;uniqueIndex:uk_run_node,priority:1"`
	NodeID       string                `gorm:"column:node_id;type:varchar(20);not null;uniqueIndex:uk_run_node,priority:2"`
	ParentNodeID string                `gorm:"column:parent_node_id;type:varchar(20)"`
	Depth        int                   `gorm:"column:depth;default:0"`
	NodeType     string                `gorm:"column:node_type;type:varchar(16)"`
	NodeName     string                `gorm:"column:node_name;type:varchar(128)"`
	ClassName    string                `gorm:"column:class_name;type:varchar(256)"`
	MethodName   string                `gorm:"column:method_name;type:varchar(128)"`
	Status       string                `gorm:"column:status;type:varchar(16);not null;default:RUNNING"`
	ErrorMessage string                `gorm:"column:error_message;type:varchar(1000)"`
	StartTime    *time.Time            `gorm:"column:start_time"`
	EndTime      *time.Time            `gorm:"column:end_time"`
	DurationMs   *int64                `gorm:"column:duration_ms"`
	ExtraData    string                `gorm:"column:extra_data;type:text"`
	CreateTime   time.Time             `gorm:"column:create_time;not null"`
	UpdateTime   time.Time             `gorm:"column:update_time;not null"`
	Deleted      soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (RagTraceNodeModel) TableName() string {
	return "t_rag_trace_node"
}
