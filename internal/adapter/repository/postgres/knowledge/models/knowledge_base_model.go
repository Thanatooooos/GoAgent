package models

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

type KnowledgeBaseModel struct {
	ID             string                `gorm:"column:id;type:varchar(20);primaryKey"`
	Name           string                `gorm:"column:name;type:varchar(128);not null"`
	EmbeddingModel string                `gorm:"column:embedding_model;type:varchar(64);not null"`
	CollectionName string                `gorm:"column:collection_name;type:varchar(64);not null"`
	CreatedBy      string                `gorm:"column:created_by;type:varchar(20);not null"`
	UpdatedBy      string                `gorm:"column:updated_by;type:varchar(20)"`
	CreateTime     time.Time             `gorm:"column:create_time;not null"`
	UpdateTime     time.Time             `gorm:"column:update_time;not null"`
	Deleted        soft_delete.DeletedAt `gorm:"column:deleted;softDelete:flag"`
}

func (KnowledgeBaseModel) TableName() string {
	return "t_knowledge_base"
}
