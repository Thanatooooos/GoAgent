package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type WikiPageModel struct {
	ID                string                `gorm:"column:id;type:varchar(20);primaryKey"`
	KnowledgeBaseID   string                `gorm:"column:kb_id;type:varchar(20);not null;index"`
	Slug              string                `gorm:"column:slug;type:varchar(256);not null"`
	Title             string                `gorm:"column:title;type:varchar(256);not null"`
	PageType          string                `gorm:"column:page_type;type:varchar(16);not null;default:entity"`
	Status            string                `gorm:"column:status;type:varchar(16);not null;default:published"`
	Content           string                `gorm:"column:content;type:text;not null;default:''"`
	Summary           string                `gorm:"column:summary;type:text;not null;default:''"`
	SourceDocumentIDs []byte                `gorm:"column:source_document_ids;type:jsonb;not null"`
	SourceChunkIDs    []byte                `gorm:"column:source_chunk_ids;type:jsonb;not null"`
	CreatedBy         string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy         string                `gorm:"column:updated_by;type:varchar(20);not null"`
	InLinks           int                   `gorm:"column:in_links;not null;default:0"`
	OutLinks          int                   `gorm:"column:out_links;not null;default:0"`
	CreateTime        time.Time             `gorm:"column:create_time;not null"`
	UpdateTime        time.Time             `gorm:"column:update_time;not null"`
	Deleted           soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (WikiPageModel) TableName() string {
	return "t_wiki_page"
}
