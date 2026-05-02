package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type KnowledgeDocumentModel struct {
	ID              string                `gorm:"column:id;type:varchar(20);primaryKey"`
	KnowledgeBaseID string                `gorm:"column:kb_id;type:varchar(20);not null;index"`
	DocName         string                `gorm:"column:doc_name;type:varchar(256);not null"`
	Enabled         int16                 `gorm:"column:enabled;not null;default:1"`
	ChunkCount      int                   `gorm:"column:chunk_count;default:0"`
	FileURL         string                `gorm:"column:file_url;type:varchar(1024);not null"`
	FileType        string                `gorm:"column:file_type;type:varchar(16);not null"`
	FileSize        int64                 `gorm:"column:file_size"`
	ProcessMode     string                `gorm:"column:process_mode;type:varchar(16);default:chunk"`
	Status          string                `gorm:"column:status;type:varchar(16);not null;default:pending;index"`
	SourceType      string                `gorm:"column:source_type;type:varchar(16)"`
	SourceLocation  string                `gorm:"column:source_location;type:varchar(1024)"`
	ScheduleEnabled *int16                `gorm:"column:schedule_enabled"`
	ScheduleCron    string                `gorm:"column:schedule_cron;type:varchar(64)"`
	ChunkStrategy   string                `gorm:"column:chunk_strategy;type:varchar(32)"`
	ChunkConfig     []byte                `gorm:"column:chunk_config;type:jsonb"`
	PipelineID      string                `gorm:"column:pipeline_id;type:varchar(20)"`
	CreatedBy       string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy       string                `gorm:"column:updated_by;type:varchar(20)"`
	CreateTime      time.Time             `gorm:"column:create_time;not null"`
	UpdateTime      time.Time             `gorm:"column:update_time;not null"`
	Deleted         soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (KnowledgeDocumentModel) TableName() string {
	return "t_knowledge_document"
}
