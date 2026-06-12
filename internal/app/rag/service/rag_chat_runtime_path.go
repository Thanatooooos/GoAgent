package service

import "strings"

const (
	chatPathAgentRuntimeTopLevel = "agent_runtime_top_level"
	chatPathRagChatLegacyMain    = "rag_chat_legacy_main"

	toolBackendAgentRuntime         = "agent_runtime"
	toolBackendToolWorkflow         = "tool_workflow"
	toolBackendToolWorkflowFallback = "tool_workflow_fallback"
)

func resolveChatPath(input RagChatInput) string {
	if input.UseAgentRuntime {
		return chatPathAgentRuntimeTopLevel
	}
	return chatPathRagChatLegacyMain
}

func resolveToolBackend(result ragChatToolStageResult) string {
	if backend := strings.TrimSpace(result.backend); backend != "" {
		return backend
	}
	if strings.TrimSpace(result.fallbackFrom) != "" {
		return toolBackendToolWorkflowFallback
	}
	return toolBackendToolWorkflow
}

func buildRuntimePathTraceExtra(chatPath string, toolBackend string, input RagChatInput, agentMode string) map[string]any {
	payload := map[string]any{
		"chatPath":        strings.TrimSpace(chatPath),
		"useAgentRuntime": input.UseAgentRuntime,
		"agentMode":       strings.TrimSpace(agentMode),
	}
	if toolBackend = strings.TrimSpace(toolBackend); toolBackend != "" {
		payload["toolBackend"] = toolBackend
	}
	return map[string]any{
		"runtimePath": payload,
	}
}
