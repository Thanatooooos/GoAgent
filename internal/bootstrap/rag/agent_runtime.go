package rag

import (
	"fmt"
	agentapp "local/rag-project/internal/app/agent"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
	ragservice "local/rag-project/internal/app/rag/service"
	longtermmemory "local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/config"
	aichat "local/rag-project/internal/infra-ai/chat"
	inframcp "local/rag-project/internal/infra-mcp"
	"strings"
)

func buildAgentRuntimeService(
	cfg *config.Config,
	mcpManager *inframcp.Manager,
	llmService aichat.LLMService,
	memoryService *longtermmemory.MemoryService,
) (ragservice.AgentRuntimeService, error) {
	options := agentapp.ServiceOptions{
		Config:        cfg,
		MCPManager:    mcpManager,
		LLMService:    llmService,
		MaxIterations: cfg.Rag.Agent.MaxIterations,
		OutputMode:    agentstate.OutputModeFinalAnswer,
		Pattern:       agentapp.PatternPlanExecute,
	}
	if memoryService != nil {
		options.MemoryRecaller = memoryService
	}
	if cfg != nil && cfg.Rag.Agent.RuntimePersistence.Enabled {
		persistenceDir := strings.TrimSpace(cfg.Rag.Agent.RuntimePersistence.Dir)
		if persistenceDir == "" {
			persistenceDir = ".agent-runtime"
		}
		sessionStore, err := agentruntime.NewFileSessionStore(persistenceDir)
		if err != nil {
			return nil, fmt.Errorf("create runtime session store: %w", err)
		}
		pendingStore, err := agentruntime.NewFilePendingApprovalStore(persistenceDir)
		if err != nil {
			return nil, fmt.Errorf("create runtime pending approval store: %w", err)
		}
		checkpointStore, err := agentkernel.NewFileCheckpointStore(persistenceDir)
		if err != nil {
			return nil, fmt.Errorf("create runtime checkpoint store: %w", err)
		}
		options.SessionStore = sessionStore
		options.PendingApprovalStore = pendingStore
		options.CheckpointStore = checkpointStore
	}
	service, err := agentapp.NewService(options)
	if err != nil {
		return nil, err
	}
	return service, nil
}
