package rag

import (
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
)

type memoryBundle struct {
	explicitMemoryService *longtermmemory.MemoryService
	memoryCacheMetrics    *ragcachemetrics.Service
	memoryCacheClient     *goredis.Client
	recallCache           port.MemoryRecallCache
}

func buildMemoryServices(buildCtx *buildContext, repos repositoriesBundle) memoryBundle {
	cfg := buildCtx.cfg
	aiRuntime := buildCtx.aiRuntime
	db := buildCtx.db

	explicitMemoryService := longtermmemory.NewMemoryService(repos.memoryItemRepo, longtermmemory.MemoryServiceOptions{
		MaxRecallItems:        cfg.Rag.Memory.ExplicitRecall.MaxItems,
		MaxRecallChars:        cfg.Rag.Memory.ExplicitRecall.MaxContextChars,
		MaxCandidatesPerScope: cfg.Rag.Memory.ExplicitRecall.MaxCandidatesPerScope,
	})
	explicitMemoryService.SetMutationTransaction(postgresrag.NewMemoryItemTransaction(db))
	explicitMemoryService.SetEmbeddingSupport(aiRuntime.Embedding, repos.memoryItemEmbeddingRepo)
	memoryCacheMetrics := buildMemoryCacheMetrics(cfg)
	explicitMemoryService.SetCacheMetrics(memoryCacheMetrics)
	memoryCacheClient, recallCache := buildMemoryRecallCache(cfg)
	explicitMemoryService.SetRecallCache(recallCache, longtermmemory.RecallCacheOptions{
		Enabled:             cfg.Rag.Memory.Cache.Enabled,
		RequestScopeEnabled: readRequestScopeCacheEnabled(cfg),
		EmbeddingTTL:        time.Duration(cfg.Rag.Memory.Cache.EmbeddingTTLSeconds) * time.Second,
		RuleTTL:             time.Duration(cfg.Rag.Memory.Cache.RuleTTLSeconds) * time.Second,
		FactTTL:             time.Duration(cfg.Rag.Memory.Cache.FactTTLSeconds) * time.Second,
		EmptyFactTTL:        time.Duration(cfg.Rag.Memory.Cache.EmptyFactTTLSeconds) * time.Second,
		EmbeddingModel:      strings.TrimSpace(cfg.AI.Embedding.DefaultModel),
		RankVersion:         defaultMemoryFactRankVersion,
	})

	return memoryBundle{
		explicitMemoryService: explicitMemoryService,
		memoryCacheMetrics:    memoryCacheMetrics,
		memoryCacheClient:     memoryCacheClient,
		recallCache:           recallCache,
	}
}
