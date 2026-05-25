package types

import (
	"context"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type SaveExplicitMemoryInput struct {
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

type ListMemoriesInput struct {
	UserID       string
	ScopeType    string
	ScopeID      string
	Namespace    string
	MemoryType   string
	Category     string
	CanonicalKey string
	Status       string
	Page         int
	PageSize     int
}

type RecallMemoriesInput struct {
	UserID           string
	Query            string
	KnowledgeBaseIDs []string
}

type RecallMemoriesResult struct {
	Used                bool
	Context             string
	Items               []domain.MemoryItem
	SelectedEntries     []RecallMemoryEntry
	CandidateCount      int
	SelectedCount       int
	RuleCount           int
	FactCandidateCount  int
	FactSelectedCount   int
	Truncated           bool
	ScopeCounts         map[string]int
	SourceCounts        map[string]int
	ContributionCounts  map[string]int
	TypeCounts          map[string]int
	SelectedMemoryIDs   []string
	RuleMemoryIDs       []string
	FactMemoryIDs       []string
	CacheEnabled        bool
	RuleCacheLayer      string
	FactCacheLayer      string
	EmbeddingCacheLayer string
	ScopeVersions       port.ScopeVersions
	RecomputeReason     string
}

type RecallMemoryEntry struct {
	ID           string
	ScopeType    string
	ScopeID      string
	MemoryType   string
	Summary      string
	Detail       string
	HitSources   []string
	KeywordScore int
	VectorScore  float32
	FinalScore   int
}

type MemoryServiceOptions struct {
	MaxRecallItems        int
	MaxRecallChars        int
	MaxCandidatesPerScope int
	DefaultListStatus     string
}

// RecallService serves long-term memory recall for chat preparation.
type RecallService interface {
	RecallMemories(ctx context.Context, input RecallMemoriesInput) (RecallMemoriesResult, error)
}
