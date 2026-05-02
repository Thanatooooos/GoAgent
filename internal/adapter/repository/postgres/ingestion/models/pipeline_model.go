package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// PipelineModel 对应 ingestion pipeline 持久化模型。
type PipelineModel struct {
	ID          string                `gorm:"column:id;type:varchar(20);primaryKey"`
	Name        string                `gorm:"column:name;type:varchar(128);not null;index:idx_ingestion_pipeline_name"`
	Description string                `gorm:"column:description;type:varchar(1024)"`
	NodesJSON   []byte                `gorm:"column:nodes_json;type:jsonb;not null"`
	CreatedBy   string                `gorm:"column:created_by;type:varchar(20);not null;index:idx_ingestion_pipeline_created_by"`
	UpdatedBy   string                `gorm:"column:updated_by;type:varchar(20)"`
	CreateTime  time.Time             `gorm:"column:create_time;not null"`
	UpdateTime  time.Time             `gorm:"column:update_time;not null"`
	Deleted     soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (PipelineModel) TableName() string {
	return "t_ingestion_pipeline"
}
