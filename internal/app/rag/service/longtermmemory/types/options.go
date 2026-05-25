package types

import "time"

const (
	DefaultMemoryListPageSize   = 20
	MaxMemoryListPageSize       = 100
	DefaultMemoryRecallItems    = 6
	DefaultMemoryRecallMaxChars = 1600
	DefaultMemorySummaryRunes   = 120
	DefaultMemoryDetailRunes    = 220
	DefaultFactRankingVersion   = "v1"
)

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

func NormalizeRecallCacheOptions(options RecallCacheOptions) RecallCacheOptions {
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
		options.RankVersion = DefaultFactRankingVersion
	}
	return options
}
