package domain

import "time"

type MemoryItemEmbedding struct {
	MemoryItemID string
	Embedding    []float32
	CreateTime   time.Time
	UpdateTime   time.Time
}

type MemoryItemSearchHit struct {
	MemoryItem
	Score float32
}
