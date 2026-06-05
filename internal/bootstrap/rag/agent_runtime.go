package rag

import (
	agentapp "local/rag-project/internal/app/agent"
	agentstate "local/rag-project/internal/app/agent/state"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/config"
	aichat "local/rag-project/internal/infra-ai/chat"
	inframcp "local/rag-project/internal/infra-mcp"
)

func buildAgentRuntimeService(cfg *config.Config, mcpManager *inframcp.Manager, llmService aichat.LLMService) (ragservice.AgentRuntimeService, error) {
	service, err := agentapp.NewService(agentapp.ServiceOptions{
		Config:        cfg,
		MCPManager:    mcpManager,
		LLMService:    llmService,
		MaxIterations: cfg.Rag.Agent.MaxIterations,
		OutputMode:    agentstate.OutputModeFinalAnswer,
		Pattern:       agentapp.PatternReactive,
	})
	if err != nil {
		return nil, err
	}
	return service, nil
}
