package service

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	"local/rag-project/internal/framework/log"
)

func logRagChatStart(input RagChatInput, agentRuntimeMode string) {
	log.Infof(
		"rag chat start: conversationID=%s userID=%s useAgentRuntime=%t requireApproval=%t deepThinking=%t knowledgeBases=%d agentMode=%s",
		strings.TrimSpace(input.ConversationID),
		strings.TrimSpace(input.UserID),
		input.UseAgentRuntime,
		input.RequireApproval,
		input.DeepThinking,
		len(input.KnowledgeBaseIDs),
		strings.TrimSpace(agentRuntimeMode),
	)
}

func logRagChatToolStageResult(traceID string, conversationID string, result ragChatToolStageResult) {
	if strings.TrimSpace(result.fallbackFrom) != "" {
		agentErrorCode := ""
		agentErrorKind := ""
		if result.agentError != nil {
			agentErrorCode = strings.TrimSpace(result.agentError.Code)
			agentErrorKind = strings.TrimSpace(result.agentError.Kind)
		}
		log.Warnf(
			"rag chat tool stage fallback: traceID=%s conversationID=%s from=%s to=%s reason=%s agentErrorCode=%s agentErrorKind=%s",
			strings.TrimSpace(traceID),
			strings.TrimSpace(conversationID),
			strings.TrimSpace(result.fallbackFrom),
			firstNonEmptyString(result.backend, "tool_workflow"),
			strings.TrimSpace(result.fallbackReason),
			agentErrorCode,
			agentErrorKind,
		)
	}
	log.Infof(
		"rag chat tool stage completed: traceID=%s conversationID=%s backend=%s used=%t degraded=%t awaitingApproval=%t toolCalls=%d rounds=%d",
		strings.TrimSpace(traceID),
		strings.TrimSpace(conversationID),
		firstNonEmptyString(result.backend, "tool_workflow"),
		result.result.Used,
		result.result.Degraded,
		result.agentRun != nil && result.agentRun.Outcome.Status == agentapp.RunStatusAwaitingApproval,
		len(result.result.Calls),
		len(result.result.Rounds),
	)
}

func logRagChatAgentRuntimeResult(traceID string, conversationID string, result agentapp.RunResponse) {
	response := result.Response
	outcome := result.Outcome
	message := "rag chat agent runtime completed"
	switch strings.TrimSpace(outcome.Status) {
	case agentapp.RunStatusAwaitingApproval:
		message = "rag chat agent runtime awaiting approval"
	case agentapp.RunStatusDegraded:
		message = "rag chat agent runtime degraded"
	}
	log.Infof(
		"%s: traceID=%s conversationID=%s status=%s checkpointID=%s interrupted=%t degraded=%t provider=%s results=%d pages=%d",
		message,
		strings.TrimSpace(traceID),
		strings.TrimSpace(conversationID),
		strings.TrimSpace(outcome.Status),
		strings.TrimSpace(outcome.CheckpointID),
		outcome.Interrupted,
		response.Degraded,
		strings.TrimSpace(response.Provider),
		len(response.Results),
		len(response.Pages),
	)
}

func logRagChatTerminalError(traceID string, stage string, err error) {
	if err == nil {
		return
	}
	log.Warnf(
		"rag chat failed: traceID=%s stage=%s err=%v",
		strings.TrimSpace(traceID),
		strings.TrimSpace(stage),
		err,
	)
}

func logRagChatCompletion(traceID string, conversationID string, result ragChatTaskResult) {
	log.Infof(
		"rag chat stream completed: traceID=%s conversationID=%s cancelled=%t contentChars=%d thinkingChars=%d hasError=%t",
		strings.TrimSpace(traceID),
		strings.TrimSpace(conversationID),
		result.cancelled,
		len(strings.TrimSpace(result.content)),
		len(strings.TrimSpace(result.thinking)),
		result.err != nil,
	)
}
