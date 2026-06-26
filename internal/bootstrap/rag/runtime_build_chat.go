package rag

import (
	"fmt"

	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/app/rag/service/longtermmemory/extraction"
	ltmwriteback "local/rag-project/internal/app/rag/service/longtermmemory/writeback"
	ragassembly "local/rag-project/internal/app/rag/tool/assembly"
	"local/rag-project/internal/framework/config"
	inframcp "local/rag-project/internal/infra-mcp"
)

type chatBundle struct {
	chatService *ragservice.RagChatService
	mcpManager  *inframcp.Manager
}

func buildChatService(
	buildCtx *buildContext,
	repos repositoriesBundle,
	conversation conversationBundle,
	memory memoryBundle,
	retrieve retrieveBundle,
) (chatBundle, error) {
	cfg := buildCtx.cfg
	aiRuntime := buildCtx.aiRuntime

	mcpManager := buildMCPManager(cfg)
	toolWorkflow := ragassembly.BuildLocalWorkflow(buildCtx.db, repos.traceRunRepo, repos.traceNodeRepo, cfg, mcpManager, aiRuntime.Chat)
	agentRuntimeService, err := buildAgentRuntimeService(cfg, mcpManager, aiRuntime.Chat, memory.explicitMemoryService)
	if err != nil {
		return chatBundle{}, fmt.Errorf("build agent runtime service: %w", err)
	}

	confidenceThreshold := 0.0
	if cfg != nil {
		confidenceThreshold = cfg.Rag.Search.Channels.VectorGlobal.ConfidenceThreshold
	}
	chatService, err := ragservice.NewRagChatServiceWithDeps(
		ragservice.RagChatDeps{
			ConversationService: conversation.conversationService,
			MessageService:      conversation.messageService,
			HistoryService:      conversation.historyService,
			RewriteService:      retrieve.rewriteService,
			RetrieveService:     retrieve.retrieveService,
			PromptService:       retrieve.promptService,
			ChatService:         aiRuntime.Chat,
			Tracer:              retrieve.tracer,
			AgentRuntime:        agentRuntimeService,
			SummaryTrigger:      conversation.summaryTrigger,
		},
		ragservice.RagChatOptions{
			ConfidenceThreshold:     confidenceThreshold,
			ParallelSubquestions:    cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
			SubquestionConcurrency:  cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
			RequestCacheMaxEntries:  readRequestCacheMaxEntries(cfg),
			AgentRuntimeMode:        cfg.Rag.Agent.Chat.Mode,
			SessionRecall:           retrieve.sessionRecallService,
			LongTermMemoryRecall:    memory.explicitMemoryService.RecallService(),
			LongTermMemoryWriteback: adaptLongTermMemoryWriteback(buildLongTermMemoryWriteback(buildCtx, memory)),
			ToolWorkflow:            toolWorkflow,
			ChatContextBudget:       buildChatContextBudgetOptions(cfg),
		},
	)
	if err != nil {
		return chatBundle{}, fmt.Errorf("build rag chat service: %w", err)
	}

	return chatBundle{
		chatService: chatService,
		mcpManager:  mcpManager,
	}, nil
}

func buildLongTermMemoryWriteback(buildCtx *buildContext, memory memoryBundle) *ltmwriteback.Service {
	if buildCtx == nil || buildCtx.aiRuntime == nil || buildCtx.aiRuntime.Chat == nil {
		return nil
	}
	if memory.explicitMemoryService == nil {
		return nil
	}

	return ltmwriteback.NewService(
		extraction.NewObservedLLMPreferenceExtractor(buildCtx.aiRuntime.Chat, memory.memoryCacheMetrics),
		longtermmemory.NewPreferenceCandidateLifecycleService(memory.explicitMemoryService),
		memory.memoryCacheMetrics,
	)
}

func buildChatContextBudgetOptions(cfg *config.Config) ragservice.ChatContextBudgetOptions {
	if cfg == nil {
		return ragservice.ChatContextBudgetOptions{}
	}
	return ragservice.ChatContextBudgetOptions{
		Enabled:               cfg.Rag.Memory.ChatContext.Enabled,
		MaxPromptTokens:       cfg.Rag.Memory.ChatContext.MaxPromptTokens,
		FixedReserveTokens:    cfg.Rag.Memory.ChatContext.FixedReserveTokens,
		SafetyReserveTokens:   cfg.Rag.Memory.ChatContext.SafetyReserveTokens,
		MemoryTokens:          cfg.Rag.Memory.ChatContext.StageBudget.MemoryTokens,
		SessionRecallTokens:   cfg.Rag.Memory.ChatContext.StageBudget.SessionRecallTokens,
		RetrieveTokens:        cfg.Rag.Memory.ChatContext.StageBudget.RetrieveTokens,
		ToolTokens:            cfg.Rag.Memory.ChatContext.StageBudget.ToolTokens,
		MessageOverheadTokens: cfg.Rag.Memory.SummaryToken.MessageOverheadTokens,
		Estimator:             ragservice.RoughTokenEstimator{},
	}
}
