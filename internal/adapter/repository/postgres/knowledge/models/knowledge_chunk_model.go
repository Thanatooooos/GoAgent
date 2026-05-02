package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type KnowledgeChunkModel struct {
	ID              string                `gorm:"column:id;type:varchar(20);primaryKey"`
	KnowledgeBaseID string                `gorm:"column:kb_id;type:varchar(20);not null;index"`
	DocumentID      string                `gorm:"column:doc_id;type:varchar(20);not null;index"`
	ChunkIndex      int                   `gorm:"column:chunk_index;not null"`
	Content         string                `gorm:"column:content;type:text;not null"`
	ContentHash     string                `gorm:"column:content_hash;type:varchar(64)"`
	CharCount       int                   `gorm:"column:char_count"`
	TokenCount      int                   `gorm:"column:token_count"`
	Enabled         int16                 `gorm:"column:enabled;not null;default:1"`
	CreatedBy       string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy       string                `gorm:"column:updated_by;type:varchar(20)"`
	CreateTime      time.Time             `gorm:"column:create_time;not null"`
	UpdateTime      time.Time             `gorm:"column:update_time;not null"`
	Deleted         soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (KnowledgeChunkModel) TableName() string {
	return "t_knowledge_chunk"
}
