package longtermmemory

import (
	"context"
	"time"
)

const defaultFactRankingVersion = "v1"

type ScopeVersions struct {
	GlobalVersion int64
	KBVersions    map[string]int64
}

type CachedMemoryItem struct {
	ID              string    `json:"id"`
	UserID          string    `json:"userId,omitempty"`
	ScopeType       string    `json:"scopeType"`
	ScopeID         string    `json:"scopeId,omitempty"`
	Namespace       string    `json:"namespace,omitempty"`
	MemoryType      string    `json:"memoryType"`
	Category        string    `json:"category,omitempty"`
	CanonicalKey    string    `json:"canonicalKey,omitempty"`
	ValueType       string    `json:"valueType,omitempty"`
	ValueJSON       string    `json:"valueJson,omitempty"`
	DisplayValue    string    `json:"displayValue,omitempty"`
	Content         string    `json:"content,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Status          string    `json:"status,omitempty"`
	Importance      int       `json:"importance,omitempty"`
	LastConfirmedAt time.Time `json:"lastConfirmedAt,omitempty"`
	UpdateTime      time.Time `json:"updateTime"`
}

type RuleMemoryCacheKey struct {
	UserID           string
	KnowledgeBaseIDs []string
	ScopeVersions    ScopeVersions
}

type RuleMemoryCacheValue struct {
	Items []CachedMemoryItem `json:"items"`
}

type QueryEmbeddingCacheKey struct {
	Query          string
	EmbeddingModel string
}

type CachedFactProjection struct {
	MemoryID       string    `json:"memoryId"`
	ScopeType      string    `json:"scopeType"`
	ScopeID        string    `json:"scopeId,omitempty"`
	Namespace      string    `json:"namespace,omitempty"`
	MemoryType     string    `json:"memoryType"`
	Category       string    `json:"category,omitempty"`
	CanonicalKey   string    `json:"canonicalKey,omitempty"`
	DisplayValue   string    `json:"displayValue,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	Detail         string    `json:"detail,omitempty"`
	KeywordMatched bool      `json:"keywordMatched"`
	VectorMatched  bool      `json:"vectorMatched"`
	KeywordScore   int       `json:"keywordScore"`
	VectorScore    float32   `json:"vectorScore"`
	FinalScore     int       `json:"finalScore"`
	UpdateTime     time.Time `json:"updateTime"`
}

type FactRankingCacheKey struct {
	UserID           string
	Query            string
	KnowledgeBaseIDs []string
	CandidateLimit   int
	EmbeddingModel   string
	RankVersion      string
	ScopeVersions    ScopeVersions
}

type FactRankingCacheValue struct {
	CandidateCount int                    `json:"candidateCount"`
	Items          []CachedFactProjection `json:"items"`
}

type RecallCache interface {
	GetRuleMemories(ctx context.Context, key RuleMemoryCacheKey) (RuleMemoryCacheValue, bool, error)
	SetRuleMemories(ctx context.Context, key RuleMemoryCacheKey, value RuleMemoryCacheValue, ttl time.Duration) error

	GetFactRankings(ctx context.Context, key FactRankingCacheKey) (FactRankingCacheValue, bool, error)
	SetFactRankings(ctx context.Context, key FactRankingCacheKey, value FactRankingCacheValue, ttl time.Duration) error

	GetQueryEmbedding(ctx context.Context, key QueryEmbeddingCacheKey) ([]float32, bool, error)
	SetQueryEmbedding(ctx context.Context, key QueryEmbeddingCacheKey, value []float32, ttl time.Duration) error

	IncrGlobalVersion(ctx context.Context, userID string) error
	IncrKBVersion(ctx context.Context, userID string, kbID string) error
	GetScopeVersions(ctx context.Context, userID string, kbIDs []string) (ScopeVersions, error)
}

type RecallCacheOptions struct {
	Enabled             bool
	RequestScopeEnabled bool
	EmbeddingTTL        time.Duration
	RuleTTL             time.Duration
	FactTTL             time.Duration
	EmptyFactTTL        time.Duration
	EmbeddingModel      string
	RankVersion         string
}

func normalizeRecallCacheOptions(options RecallCacheOptions) RecallCacheOptions {
	if options.EmbeddingTTL <= 0 {
		options.EmbeddingTTL = 30 * time.Minute
	}
	if options.RuleTTL <= 0 {
		options.RuleTTL = 10 * time.Minute
	}
	if options.FactTTL <= 0 {
		options.FactTTL = 3 * time.Minute
	}
	if options.EmptyFactTTL <= 0 {
		options.EmptyFactTTL = 30 * time.Second
	}
	if options.RankVersion == "" {
		options.RankVersion = defaultFactRankingVersion
	}
	return options
}
