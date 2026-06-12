package rag

import (
	"strings"
	"time"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/port"
	ragservice "local/rag-project/internal/app/rag/service"
)

type retrieveBundle struct {
	rewriteService       ragrewrite.Service
	promptService        *ragprompt.Service
	retrieveService      *ragretrieve.Engine
	traceService         *ragservice.TraceService
	tracer               *ragservice.ChatTracer
	sessionRecallService ragservice.SessionRecallService
}

func buildRetrieveServices(
	buildCtx *buildContext,
	repos repositoriesBundle,
	memory memoryBundle,
) retrieveBundle {
	cfg := buildCtx.cfg
	aiRuntime := buildCtx.aiRuntime

	var rewriteService ragrewrite.Service
	if cfg.Rag.QueryRewrite.Enabled {
		rewriteService = ragrewrite.NewLLMService(aiRuntime.Chat)
		rewriteService = ragrewrite.NewTermNormalizingService(
			rewriteService,
			ragrewrite.TermNormalizationOptions{
				Enabled: cfg.Rag.QueryRewrite.TermNormalization.Enabled,
				Rules:   buildTermNormalizationRules(cfg.Rag.QueryRewrite.TermNormalization.Rules),
			},
		)
	}
	promptService := ragprompt.NewService(nil)
	retrieveService := ragretrieve.NewEngine(buildCtx.searcher, aiRuntime.Embedding, aiRuntime.Rerank)
	retrieveService.SetFactMemoryRetriever(memory.explicitMemoryService.FactRetriever())
	traceService := ragservice.NewTraceService(repos.traceRunRepo, repos.traceNodeRepo, repos.userRepo)
	tracer := ragservice.NewChatTracer(repos.traceRunRepo, repos.traceNodeRepo)
	sessionRecallService := buildSessionRecallService(buildCtx, repos, memory)

	return retrieveBundle{
		rewriteService:       rewriteService,
		promptService:        promptService,
		retrieveService:      retrieveService,
		traceService:         traceService,
		tracer:               tracer,
		sessionRecallService: sessionRecallService,
	}
}

func buildSessionRecallService(
	buildCtx *buildContext,
	repos repositoriesBundle,
	memory memoryBundle,
) ragservice.SessionRecallService {
	cfg := buildCtx.cfg
	aiRuntime := buildCtx.aiRuntime

	sessionRecallService := ragservice.NewSessionRecallService(repos.sessionChunkRepo, aiRuntime.Embedding, ragservice.SessionRecallOptions{
		Enabled:              cfg.Rag.Memory.SessionRecall.Enabled,
		MaxExcerpts:          cfg.Rag.Memory.SessionRecall.MaxExcerpts,
		MaxChunksPerMessage:  cfg.Rag.Memory.SessionRecall.MaxChunksPerMessage,
		ExcerptTargetTokens:  cfg.Rag.Memory.SessionRecall.ExcerptTargetTokens,
		ExcerptOverlapTokens: cfg.Rag.Memory.SessionRecall.ExcerptOverlapTokens,
		MaxPromptTokens:      cfg.Rag.Memory.SessionRecall.MaxPromptTokens,
		Estimator:            ragservice.RoughTokenEstimator{},
	})
	if cacheAware, ok := sessionRecallService.(interface {
		SetCacheSupport(cache port.MemoryRecallCache, options ragservice.SessionRecallCacheOptions)
	}); ok {
		cacheAware.SetCacheSupport(memory.recallCache, ragservice.SessionRecallCacheOptions{
			Enabled:                  cfg.Rag.Memory.Cache.Enabled && readSessionRecallCacheEnabled(cfg),
			RequestScopeEnabled:      readRequestScopeCacheEnabled(cfg),
			ConversationScopeEnabled: readConversationScopeCacheEnabled(cfg),
			ConversationMaxEntries:   cfg.Rag.Memory.Cache.ConversationMaxEntries,
			ConversationTTL:          time.Duration(cfg.Rag.Memory.Cache.ConversationTTLSeconds) * time.Second,
			EmptyResultTTL:           time.Duration(cfg.Rag.Memory.Cache.EmptySessionTTLSeconds) * time.Second,
			EmbeddingTTL:             time.Duration(cfg.Rag.Memory.Cache.EmbeddingTTLSeconds) * time.Second,
			EmbeddingModel:           strings.TrimSpace(cfg.AI.Embedding.DefaultModel),
		})
	}
	if metricAware, ok := sessionRecallService.(interface {
		SetCacheMetrics(metrics *ragcachemetrics.Service)
	}); ok {
		metricAware.SetCacheMetrics(memory.memoryCacheMetrics)
	}
	return sessionRecallService
}
