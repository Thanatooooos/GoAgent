package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// TaskNodeModel 对应 ingestion task node 持久化模型。
type TaskNodeModel struct {
	ID           string                `gorm:"column:id;type:varchar(20);primaryKey"`
	TaskID       string                `gorm:"column:task_id;type:varchar(20);not null;uniqueIndex:uk_ingestion_task_node;index:idx_ingestion_task_node_task_id,priority:1;index:idx_ingestion_task_node_task_order,priority:1"`
	PipelineID   string                `gorm:"column:pipeline_id;type:varchar(20);not null;index:idx_ingestion_task_node_pipeline_id"`
	NodeID       string                `gorm:"column:node_id;type:varchar(64);not null;uniqueIndex:uk_ingestion_task_node"`
	NodeType     string                `gorm:"column:node_type;type:varchar(16);not null"`
	NodeOrder    int                   `gorm:"column:node_order;default:0;index:idx_ingestion_task_node_task_order,priority:2"`
	Status       string                `gorm:"column:status;type:varchar(16);not null;default:pending"`
	DurationMs   int64                 `gorm:"column:duration_ms"`
	Message      string                `gorm:"column:message;type:varchar(1000)"`
	ErrorMessage string                `gorm:"column:error_message;type:varchar(1000)"`
	Output       []byte                `gorm:"column:output;type:jsonb"`
	CreateTime   time.Time             `gorm:"column:create_time;not null"`
	UpdateTime   time.Time             `gorm:"column:update_time;not null"`
	Deleted      soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (TaskNodeModel) TableName() string {
	return "t_ingestion_task_node"
}
