package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// TaskModel 对应 ingestion task 持久化模型。
type TaskModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	PipelineID     string                `gorm:"column:pipeline_id;type:varchar(20);not null;index:idx_ingestion_task_pipeline_id"`
	SourceType     string                `gorm:"column:source_type;type:varchar(16);not null"`
	SourceLocation string                `gorm:"column:source_location;type:varchar(1024)"`
	SourceFileName string                `gorm:"column:source_file_name;type:varchar(256)"`
	Status         string                `gorm:"column:status;type:varchar(16);not null;default:pending;index:idx_ingestion_task_status"`
	ChunkCount     int                   `gorm:"column:chunk_count;not null;default:0"`
	ErrorMessage   string                `gorm:"column:error_message;type:varchar(1000)"`
	Metadata       []byte                `gorm:"column:metadata;type:jsonb"`
	StartedAt      *time.Time            `gorm:"column:started_at"`
	CompletedAt    *time.Time            `gorm:"column:completed_at"`
	CreatedBy      string                `gorm:"column:created_by;type:varchar(20);not null;index:idx_ingestion_task_created_by"`
	UpdatedBy      string                `gorm:"column:updated_by;type:varchar(20)"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (TaskModel) TableName() string {
	return "t_ingestion_task"
}
