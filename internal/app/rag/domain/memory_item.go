package domain

import "time"

const (
	MemoryScopeGlobal = "global"
	MemoryScopeKB     = "kb"

	MemoryTypePreference = "preference"
	MemoryTypeKnowledge  = "knowledge"
	MemoryTypeFeedback   = "feedback"

	MemoryStatusPending          = "pending"
	MemoryStatusActive           = "active"
	MemoryStatusRejected         = "rejected"
	MemoryStatusExpired          = "expired"
	MemoryStatusSuperseded       = "superseded"
	MemoryValueTypeText          = "text"
	MemoryValueTypeEnum          = "enum"
	MemoryValueTypeBoolean       = "boolean"
	MemoryValueTypeJSON          = "json"
	MemoryCategoryResponse       = "response"
	MemoryCategoryWorkflow       = "workflow"
	MemoryCategoryBehavior       = "behavior"
	MemoryCategoryProject        = "project"
	MemoryCategoryGeneral        = "general"
	MemoryCategoryFeedback       = "feedback"
	MemoryExtractionMethodManual = "manual"
	MemoryExtractionMethodRule   = "explicit_rule"
	MemoryExtractionMethodLLM    = "explicit_llm"
)

type MemoryItem struct {
	ID               string
	UserID           string
	ScopeType        string
	ScopeID          string
	Namespace        string
	MemoryType       string
	Category         string
	CanonicalKey     string
	ValueType        string
	ValueJSON        string
	DisplayValue     string
	SourceMessageID  string
	Content          string
	Summary          string
	Confidence       float64
	Importance       int
	Status           string
	LastConfirmedAt  *time.Time
	LastUsedAt       *time.Time
	ExpiresAt        *time.Time
	SupersedesID     string
	ExtractionMethod string
	CreatedBy        string
	UpdatedBy        string
	CreateTime       time.Time
	UpdateTime       time.Time
}
