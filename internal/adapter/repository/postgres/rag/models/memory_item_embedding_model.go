package models

import "time"

type MemoryItemEmbeddingModel struct {
	MemoryItemID string    `gorm:"column:memory_item_id;type:varchar(20);primaryKey"`
	Embedding    string    `gorm:"column:embedding;type:vector;not null"`
	CreateTime   time.Time `gorm:"column:create_time;not null"`
	UpdateTime   time.Time `gorm:"column:update_time;not null"`
}

func (MemoryItemEmbeddingModel) TableName() string {
	return "t_memory_item_embedding"
}
