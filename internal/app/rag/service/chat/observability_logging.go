package chat

import (
	"context"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	"local/rag-project/internal/framework/log"
)

func enrichRagChatLogContext(ctx context.Context, traceID, conversationID, userID, taskID string) context.Context {
	fields := make([]interface{}, 0, 8)
	if value := strings.TrimSpace(traceID); value != "" {
		fields = append(fields, "trace_id", value)
	}
	if value := strings.TrimSpace(conversationID); value != "" {
		fields = append(fields, "conversation_id", value)
	}
	if value := strings.TrimSpace(userID); value != "" {
		fields = append(fields, "user_id", value)
	}
	if value := strings.TrimSpace(taskID); value != "" {
		fields = append(fields, "task_id", value)
	}
	if len(fields) == 0 {
		return ctx
	}
	return log.NewContext(ctx, fields...)
}

func logRagChatStart(ctx context.Context, input RagChatInput, agentRuntimeMode string, chatPath string) {
	log.FromContext(ctx).Infow(
		"rag chat start",
		"chat_path", strings.TrimSpace(chatPath),
		"use_agent_runtime", input.UseAgentRuntime,
		"require_approval", input.RequireApproval,
		"deep_thinking", input.DeepThinking,
		"knowledge_bases", len(input.KnowledgeBaseIDs),
		"agent_mode", strings.TrimSpace(agentRuntimeMode),
	)
}

func logRagChatToolStageResult(ctx context.Context, result ragChatToolStageResult) {
	if strings.TrimSpace(result.fallbackFrom) != "" {
		agentErrorCode := ""
		agentErrorKind := ""
		if result.agentError != nil {
			agentErrorCode = strings.TrimSpace(result.agentError.Code)
			agentErrorKind = strings.TrimSpace(result.agentError.Kind)
		}
		log.FromContext(ctx).Warnw(
			"rag chat tool stage fallback",
			"from", strings.TrimSpace(result.fallbackFrom),
			"to", firstNonEmptyString(result.backend, "tool_workflow"),
			"reason", strings.TrimSpace(result.fallbackReason),
			"agent_error_code", agentErrorCode,
			"agent_error_kind", agentErrorKind,
		)
	}
	log.FromContext(ctx).Infow(
		"rag chat tool stage completed",
		"backend", firstNonEmptyString(result.backend, "tool_workflow"),
		"tool_backend", resolveToolBackend(result),
		"used", result.result.Used,
		"degraded", result.result.Degraded,
		"awaiting_approval", result.agentRun != nil && result.agentRun.Outcome.Status == agentapp.RunStatusAwaitingApproval,
		"tool_calls", len(result.result.Calls),
		"rounds", len(result.result.Rounds),
	)
}

func logRagChatAgentRuntimeResult(ctx context.Context, result agentapp.RunResponse) {
	response := result.Response
	outcome := result.Outcome
	message := "rag chat agent runtime completed"
	switch strings.TrimSpace(outcome.Status) {
	case agentapp.RunStatusAwaitingApproval:
		message = "rag chat agent runtime awaiting approval"
	case agentapp.RunStatusDegraded:
		message = "rag chat agent runtime degraded"
	}
	log.FromContext(ctx).Infow(
		message,
		"status", strings.TrimSpace(outcome.Status),
		"checkpoint_id", strings.TrimSpace(outcome.CheckpointID),
		"interrupted", outcome.Interrupted,
		"degraded", response.Degraded,
		"provider", strings.TrimSpace(response.Provider),
		"results", len(response.Results),
		"pages", len(response.Pages),
	)
}

func logRagChatTerminalError(ctx context.Context, stage string, err error) {
	if err == nil {
		return
	}
	log.FromContext(ctx).Warnw(
		"rag chat failed",
		"stage", strings.TrimSpace(stage),
		"error", err,
	)
}

func logRagChatCompletion(ctx context.Context, result ragChatTaskResult) {
	log.FromContext(ctx).Infow(
		"rag chat stream completed",
		"cancelled", result.cancelled,
		"content_chars", len(strings.TrimSpace(result.content)),
		"thinking_chars", len(strings.TrimSpace(result.thinking)),
		"has_error", result.err != nil,
	)
}
