package domain

import "time"

type KnowledgeBase struct {
	ID             string
	Name           string
	EmbeddingModel string
	CollectionName string
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewKnowledgeBase(id, name, embeddingModel, collectionName, createdBy string) KnowledgeBase {
	now := time.Now()
	return KnowledgeBase{
		ID:             id,
		Name:           name,
		EmbeddingModel: embeddingModel,
		CollectionName: collectionName,
		CreatedBy:      createdBy,
		UpdatedBy:      createdBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
