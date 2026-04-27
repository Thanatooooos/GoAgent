package models

import "time"

type KnowledgeDocumentScheduleModel struct {
	ID              string     `gorm:"column:id;type:varchar(20);primaryKey"`
	DocumentID      string     `gorm:"column:doc_id;type:varchar(20);not null;uniqueIndex:uk_doc_id"`
	KnowledgeBaseID string     `gorm:"column:kb_id;type:varchar(20);not null"`
	CronExpr        string     `gorm:"column:cron_expr;type:varchar(64)"`
	Enabled         int16      `gorm:"column:enabled;default:0"`
	NextRunTime     *time.Time `gorm:"column:next_run_time"`
	LastRunTime     *time.Time `gorm:"column:last_run_time"`
	LastSuccessTime *time.Time `gorm:"column:last_success_time"`
	LastStatus      string     `gorm:"column:last_status;type:varchar(16)"`
	LastError       string     `gorm:"column:last_error;type:varchar(512)"`
	LastETag        string     `gorm:"column:last_etag;type:varchar(256)"`
	LastModified    string     `gorm:"column:last_modified;type:varchar(256)"`
	LastContentHash string     `gorm:"column:last_content_hash;type:varchar(128)"`
	LockOwner       string     `gorm:"column:lock_owner;type:varchar(128);index"`
	LockUntil       *time.Time `gorm:"column:lock_until;index"`
	CreateTime      time.Time  `gorm:"column:create_time;not null"`
	UpdateTime      time.Time  `gorm:"column:update_time;not null"`
}

func (KnowledgeDocumentScheduleModel) TableName() string {
	return "t_knowledge_document_schedule"
}
