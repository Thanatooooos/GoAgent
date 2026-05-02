package models

import "time"

type KnowledgeDocumentChunkLogModel struct {
	ID              string     `gorm:"column:id;type:varchar(20);primaryKey"`
	DocumentID      string     `gorm:"column:doc_id;type:varchar(20);not null;index"`
	Status          string     `gorm:"column:status;type:varchar(16);not null"`
	ProcessMode     string     `gorm:"column:process_mode;type:varchar(16)"`
	ChunkStrategy   string     `gorm:"column:chunk_strategy;type:varchar(16)"`
	PipelineID      string     `gorm:"column:pipeline_id;type:varchar(20)"`
	ExtractDuration int64      `gorm:"column:extract_duration"`
	ChunkDuration   int64      `gorm:"column:chunk_duration"`
	EmbedDuration   int64      `gorm:"column:embed_duration"`
	PersistDuration int64      `gorm:"column:persist_duration"`
	TotalDuration   int64      `gorm:"column:total_duration"`
	ChunkCount      int        `gorm:"column:chunk_count"`
	ErrorMessage    string     `gorm:"column:error_message;type:text"`
	StartTime       *time.Time `gorm:"column:start_time"`
	EndTime         *time.Time `gorm:"column:end_time"`
	CreateTime      time.Time  `gorm:"column:create_time"`
	UpdateTime      time.Time  `gorm:"column:update_time"`
}

func (KnowledgeDocumentChunkLogModel) TableName() string {
	return "t_knowledge_document_chunk_log"
}
