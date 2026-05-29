package rag

import "time"

type conversationVO struct {
	ConversationID string     `json:"conversationId"`
	Title          string     `json:"title"`
	LastTime       *time.Time `json:"lastTime,omitempty"`
}

type messageVO struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversationId"`
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	RawContent       string     `json:"rawContent,omitempty"`
	ContentSummary   string     `json:"contentSummary,omitempty"`
	IsSummarized     bool       `json:"isSummarized,omitempty"`
	ThinkingContent  string     `json:"thinkingContent,omitempty"`
	ThinkingDuration *int       `json:"thinkingDuration,omitempty"`
	Vote             *int       `json:"vote"`
	CreateTime       *time.Time `json:"createTime,omitempty"`
}

type memoryItemVO struct {
	ID               string     `json:"id"`
	UserID           string     `json:"userId"`
	ScopeType        string     `json:"scopeType"`
	ScopeID          string     `json:"scopeId,omitempty"`
	Namespace        string     `json:"namespace,omitempty"`
	MemoryType       string     `json:"memoryType"`
	Category         string     `json:"category,omitempty"`
	CanonicalKey     string     `json:"canonicalKey,omitempty"`
	ValueType        string     `json:"valueType,omitempty"`
	ValueJSON        string     `json:"valueJson,omitempty"`
	DisplayValue     string     `json:"displayValue,omitempty"`
	SourceMessageID  string     `json:"sourceMessageId,omitempty"`
	Content          string     `json:"content"`
	Summary          string     `json:"summary,omitempty"`
	Confidence       float64    `json:"confidence"`
	Importance       int        `json:"importance,omitempty"`
	Status           string     `json:"status"`
	LastConfirmedAt  *time.Time `json:"lastConfirmedAt,omitempty"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	SupersedesID     string     `json:"supersedesId,omitempty"`
	ExtractionMethod string     `json:"extractionMethod,omitempty"`
	CreateTime       *time.Time `json:"createTime,omitempty"`
	UpdateTime       *time.Time `json:"updateTime,omitempty"`
}
