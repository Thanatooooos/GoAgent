package longtermmemory

import (
	"time"

	"local/rag-project/internal/app/rag/domain"
)

type MemoryCardinality string

const (
	MemoryCardinalitySingle MemoryCardinality = "single"
	MemoryCardinalityMulti  MemoryCardinality = "multi"
)

type GateDecisionAction string

const (
	GateDecisionCreate  GateDecisionAction = "create"
	GateDecisionUpdate  GateDecisionAction = "update"
	GateDecisionMerge   GateDecisionAction = "merge"
	GateDecisionIgnore  GateDecisionAction = "ignore"
	GateDecisionPending GateDecisionAction = "pending"
)

type MemoryKeySpec struct {
	CanonicalKey      string
	Category          string
	MemoryType        string
	ValueType         string
	Cardinality       MemoryCardinality
	DefaultImportance int
	AllowedScopeTypes []string
}

type normalizedSaveInput struct {
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
	Importance       int
	ExtractionMethod string
	ExpiresAt        *time.Time
}

type GateDecision struct {
	Action GateDecisionAction
	Input  normalizedSaveInput
	Spec   *MemoryKeySpec
}

type ConflictResolution struct {
	Action          GateDecisionAction
	Existing        *domain.MemoryItem
	UpdatedExisting *domain.MemoryItem
	CreateCandidate *domain.MemoryItem
}
