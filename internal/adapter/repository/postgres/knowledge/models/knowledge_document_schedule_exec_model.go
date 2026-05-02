package models

import "time"

type KnowledgeDocumentScheduleExecModel struct {
	ID              string     `gorm:"column:id;type:varchar(20);primaryKey"`
	ScheduleID      string     `gorm:"column:schedule_id;type:varchar(20);not null;index:idx_schedule_time"`
	DocumentID      string     `gorm:"column:doc_id;type:varchar(20);not null;index:idx_doc_id_exec"`
	KnowledgeBaseID string     `gorm:"column:kb_id;type:varchar(20);not null"`
	Status          string     `gorm:"column:status;type:varchar(16);not null"`
	Message         string     `gorm:"column:message;type:varchar(512)"`
	StartTime       *time.Time `gorm:"column:start_time;index:idx_schedule_time"`
	EndTime         *time.Time `gorm:"column:end_time"`
	FileName        string     `gorm:"column:file_name;type:varchar(512)"`
	FileSize        *int64     `gorm:"column:file_size"`
	ContentHash     string     `gorm:"column:content_hash;type:varchar(128)"`
	ETag            string     `gorm:"column:etag;type:varchar(256)"`
	LastModified    string     `gorm:"column:last_modified;type:varchar(256)"`
	CreateTime      time.Time  `gorm:"column:create_time;not null"`
	UpdateTime      time.Time  `gorm:"column:update_time;not null"`
}

func (KnowledgeDocumentScheduleExecModel) TableName() string {
	return "t_knowledge_document_schedule_exec"
}
