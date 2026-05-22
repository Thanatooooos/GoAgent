package domain

import "time"

const (
	MemoryScopeGlobal = "global"
	MemoryScopeKB     = "kb"

	MemoryTypePreference = "preference"
	MemoryTypeKnowledge  = "knowledge"
	MemoryTypeFeedback   = "feedback"

	MemoryStatusPending  = "pending"
	MemoryStatusActive   = "active"
	MemoryStatusRejected = "rejected"
	MemoryStatusExpired  = "expired"
)

type MemoryItem struct {
	ID              string
	UserID          string
	ScopeType       string
	ScopeID         string
	MemoryType      string
	SourceMessageID string
	Content         string
	Summary         string
	Confidence      float64
	Status          string
	LastConfirmedAt *time.Time
	ExpiresAt       *time.Time
	CreatedBy       string
	UpdatedBy       string
	CreateTime      time.Time
	UpdateTime      time.Time
}
