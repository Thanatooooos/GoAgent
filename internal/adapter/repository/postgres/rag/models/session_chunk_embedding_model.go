package models

import "time"

type SessionChunkEmbeddingModel struct {
	ChunkID    string    `gorm:"column:chunk_id;type:varchar(20);primaryKey"`
	Embedding  string    `gorm:"column:embedding;type:vector;not null"`
	CreateTime time.Time `gorm:"column:create_time;not null"`
	UpdateTime time.Time `gorm:"column:update_time;not null"`
}

func (SessionChunkEmbeddingModel) TableName() string {
	return "t_session_chunk_embedding"
}
