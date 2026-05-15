package tool

import (
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type AgentLoop = ragruntime.AgentLoop
type Executor = ragruntime.Executor
type LLMObserver = ragruntime.LLMObserver
type RuleObserver = ragruntime.RuleObserver

func NewAgentLoop(executor *Executor) *AgentLoop {
	return ragruntime.NewAgentLoop(executor)
}

func NewExecutor(registry *Registry) *Executor {
	return ragruntime.NewExecutor(registry)
}

func NewLLMObserver(chatService aichat.LLMService) *LLMObserver {
	return ragruntime.NewLLMObserver(chatService)
}

func NewRuleObserver() *RuleObserver {
	return ragruntime.NewRuleObserver()
}
