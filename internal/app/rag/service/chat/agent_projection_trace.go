package chat

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
)

func buildAgentRuntimeTraceExtra(result agentapp.RunResponse) map[string]any {
	payload := map[string]any{
		"status":          strings.TrimSpace(result.Outcome.Status),
		"interrupted":     result.Outcome.Interrupted,
		"interruptReason": strings.TrimSpace(result.Outcome.InterruptReason),
		"checkpointId":    strings.TrimSpace(result.Outcome.CheckpointID),
		"degraded":        result.Response.Degraded,
		"degradeReason":   strings.TrimSpace(result.Response.DegradeReason),
		"provider":        strings.TrimSpace(result.Response.Provider),
	}
	if approval := newRagChatApprovalPendingPayload(result.Outcome.Approval); approval != nil {
		payload["approval"] = approvalTraceProjection(*approval)
	}
	return payload
}

func buildAgentRuntimeServiceErrorTraceExtra(err error) map[string]any {
	payload := newRagChatAgentServiceErrorPayload(err)
	if payload == (RagChatAgentServiceErrorPayload{}) {
		return nil
	}
	return map[string]any{
		"status":       "failed",
		"serviceError": agentServiceErrorTraceProjection(payload),
	}
}

func buildAgentRuntimeToolStageTraceExtra(result ragChatToolStageResult) map[string]any {
	if result.agentRun != nil {
		extra := buildAgentRuntimeTraceExtra(*result.agentRun)
		extra["backend"] = firstNonEmptyString(result.backend, "agent_runtime")
		extra["hasToolContext"] = strings.TrimSpace(result.result.Context) != ""
		extra["hasAnswerGuidance"] = strings.TrimSpace(result.result.AnswerGuidance) != ""
		return extra
	}

	extra := map[string]any{
		"backend":       firstNonEmptyString(result.backend, "agent_runtime"),
		"degraded":      result.result.Degraded,
		"degradeReason": strings.TrimSpace(result.result.DegradeReason),
	}
	if result.agentError != nil {
		extra["status"] = "failed"
		extra["serviceError"] = agentServiceErrorTraceProjection(*result.agentError)
	}
	return extra
}

func buildToolWorkflowStageTraceExtra(result ragChatToolStageResult) map[string]any {
	names := make([]string, 0, len(result.result.Calls))
	for _, call := range result.result.Calls {
		names = append(names, strings.TrimSpace(call.Name))
	}
	extra := map[string]any{
		"backend":             firstNonEmptyString(result.backend, "tool_workflow"),
		"used":                result.result.Used,
		"toolCallCount":       len(result.result.Calls),
		"roundCount":          len(result.result.Rounds),
		"toolNames":           names,
		"degraded":            result.result.Degraded,
		"degradeReason":       strings.TrimSpace(result.result.DegradeReason),
		"capability":          strings.TrimSpace(result.result.TraceMeta.Capability),
		"executionMode":       strings.TrimSpace(result.result.TraceMeta.ExecutionMode),
		"riskLevel":           strings.TrimSpace(result.result.TraceMeta.RiskLevel),
		"approvalRequirement": strings.TrimSpace(result.result.TraceMeta.ApprovalRequirement),
		"evidenceSources":     append([]string(nil), result.result.TraceMeta.EvidenceSources...),
	}
	if strings.TrimSpace(result.fallbackFrom) != "" {
		extra["fallback"] = map[string]any{
			"from":   strings.TrimSpace(result.fallbackFrom),
			"reason": strings.TrimSpace(result.fallbackReason),
		}
		if result.agentError != nil {
			extra["fallbackAgentError"] = agentServiceErrorTraceProjection(*result.agentError)
		}
	}
	return extra
}

func approvalTraceProjection(payload RagChatApprovalPendingPayload) map[string]any {
	return map[string]any{
		"status":           payload.Status,
		"reasonCode":       payload.ReasonCode,
		"reasonMessage":    payload.ReasonMessage,
		"trigger":          payload.Trigger,
		"checkpointId":     payload.CheckpointID,
		"sessionId":        payload.SessionID,
		"capability":       payload.CapabilityName,
		"capabilityKind":   payload.CapabilityKind,
		"capabilityFamily": payload.CapabilityFamily,
		"riskLevel":        payload.RiskLevel,
		"supportsResume":   payload.SupportsResume,
		"idempotency":      payload.Idempotency,
		"currentStepId":    payload.CurrentStepID,
		"currentStepTitle": payload.CurrentStepTitle,
		"candidateUrls":    append([]string(nil), payload.CandidateURLs...),
		"canApprove":       payload.CanApprove,
		"canReject":        payload.CanReject,
		"rejectOutcome":    payload.RejectOutcome,
	}
}

func agentServiceErrorTraceProjection(payload RagChatAgentServiceErrorPayload) map[string]any {
	return map[string]any{
		"code":      strings.TrimSpace(payload.Code),
		"message":   strings.TrimSpace(payload.Message),
		"kind":      strings.TrimSpace(payload.Kind),
		"retryable": payload.Retryable,
	}
}
