package chat

import (
	"context"
	"strings"

	agentstate "local/rag-project/internal/app/agent/state"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/core/tokenbudget"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
)

func (s *RagChatService) runToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	retrievalUsed bool,
	traceID string,
	sink RagChatEventSink,
) (ragChatToolStageResult, error) {
	if s == nil || !shouldRunToolWorkflow(input, rewriteResult, retrievalUsed) {
		return ragChatToolStageResult{}, nil
	}
	if s.shouldUseAgentRuntimeForToolStage(input, rewriteResult, retrievalUsed) {
		agentResult, err := s.runAgentToolWorkflowStage(ctx, input, history, memoryContext, sessionContext, rewriteResult, retrieveResult, traceID, sink)
		if err != nil {
			return ragChatToolStageResult{}, err
		}
		if shouldFallbackFromAgentToolStage(agentResult) && s.toolWorkflow != nil {
			legacyResult, legacyErr := s.runLegacyToolWorkflowStage(ctx, input, history, rewriteResult, retrieveResult, traceID, sink, true)
			if legacyErr == nil {
				legacyResult.fallbackFrom = "agent_runtime"
				legacyResult.fallbackReason = firstNonEmptyString(agentResult.result.DegradeReason, agentResult.agentError.Message)
				legacyResult.agentError = agentResult.agentError
				return legacyResult, nil
			}
			return ragChatToolStageResult{}, legacyErr
		}
		if agentResult.agentError != nil && sink != nil {
			_ = sink.SendAgentServiceError(*agentResult.agentError)
		}
		return agentResult, nil
	}
	return s.runLegacyToolWorkflowStage(ctx, input, history, rewriteResult, retrieveResult, traceID, sink, false)
}

func (s *RagChatService) runLegacyToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	traceID string,
	sink RagChatEventSink,
	fallback bool,
) (ragChatToolStageResult, error) {
	if s == nil || s.toolWorkflow == nil {
		return ragChatToolStageResult{}, nil
	}
	nodeID := "tool_workflow"
	nodeName := "tool_workflow"
	if fallback {
		nodeID = "tool_workflow_fallback"
		nodeName = "tool_workflow_fallback"
	}
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatToolStageResult]{
		node: ragChatTraceNode{
			NodeID:   nodeID,
			NodeType: "tool",
			NodeName: nodeName,
		},
		run: func(ctx context.Context) (ragChatToolStageResult, error) {
			result, err := s.toolWorkflow.Run(ctx, ragtool.WorkflowInput{
				Question:           strings.TrimSpace(input.Question),
				UserID:             strings.TrimSpace(input.UserID),
				ConversationID:     strings.TrimSpace(input.ConversationID),
				TraceID:            strings.TrimSpace(traceID),
				Control:            defaultWorkflowControl(),
				KnowledgeBaseIDs:   append([]string(nil), input.KnowledgeBaseIDs...),
				History:            append([]convention.ChatMessage(nil), history...),
				RewriteResult:      rewriteResult,
				RetrieveResult:     retrieveResult,
				ContextTokenBudget: s.chatContextBudget.ToolTokens,
				ContextEstimator:   s.chatContextBudget.Estimator,
				EventSink:          ragChatWorkflowEventSink{sink: sink},
			})
			if err != nil {
				return ragChatToolStageResult{}, err
			}
			backend := toolBackendToolWorkflow
			if fallback {
				backend = toolBackendToolWorkflowFallback
			}
			return ragChatToolStageResult{result: result, backend: backend}, nil
		},
		buildExtra: func(result ragChatToolStageResult) map[string]any {
			return buildToolWorkflowStageTraceExtra(result)
		},
	})
}

func (s *RagChatService) applyRetrieveContextBudget(ctx context.Context, traceID string, result ragretrieve.Result) ragretrieve.Result {
	if s == nil || s.chatContextBudget.RetrieveTokens <= 0 || len(result.Chunks) == 0 {
		return result
	}
	contextText, stats := ragretrieve.BuildKnowledgeContextWithinBudget(
		result.Chunks,
		s.chatContextBudget.RetrieveTokens,
		s.chatContextBudget.Estimator,
	)
	result.KnowledgeContext = contextText
	if s.tracer != nil {
		s.tracer.appendTraceRunExtra(ctx, traceID, map[string]any{
			"retrieveContextBudget": stats,
		})
	}
	return result
}

func (s *RagChatService) applyToolContextBudget(ctx context.Context, traceID string, result ragChatToolStageResult) ragChatToolStageResult {
	if s == nil || s.chatContextBudget.ToolTokens <= 0 || strings.TrimSpace(result.result.Context) == "" {
		return result
	}
	before := s.chatContextBudget.Estimator.EstimateTokens(result.result.Context)
	trimmed, changed := tokenbudget.TruncateText(
		result.result.Context,
		s.chatContextBudget.ToolTokens,
		s.chatContextBudget.Estimator,
	)
	result.result.Context = trimmed
	stats := result.result.ContextBudget
	if stats.TokensBefore == 0 {
		stats.TokensBefore = before
	}
	stats.TokensAfter = s.chatContextBudget.Estimator.EstimateTokens(trimmed)
	stats.Truncated = stats.Truncated || changed
	if stats.RetainedSections == 0 && strings.TrimSpace(trimmed) != "" {
		stats.RetainedSections = 1
	}
	if changed && strings.TrimSpace(trimmed) == "" && stats.DroppedSections == 0 {
		stats.DroppedSections = 1
	}
	result.result.ContextBudget = stats
	if s.tracer != nil {
		s.tracer.appendTraceRunExtra(ctx, traceID, map[string]any{
			"toolContextBudget": stats,
		})
	}
	return result
}

func (s *RagChatService) runAgentToolWorkflowStage(
	ctx context.Context,
	input RagChatInput,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
	traceID string,
	sink RagChatEventSink,
) (ragChatToolStageResult, error) {
	if s == nil || s.agentRuntime == nil {
		return ragChatToolStageResult{}, nil
	}

	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatToolStageResult]{
		node: ragChatTraceNode{
			NodeID:   "agent_tool_workflow",
			NodeType: "agent",
			NodeName: "agent_tool_workflow",
		},
		run: func(ctx context.Context) (ragChatToolStageResult, error) {
			req := buildAgentToolStageRequest(input, traceID, history, memoryContext, sessionContext, rewriteResult, retrieveResult)
			req.Options.OutputMode = agentstate.OutputModeFinalAnswer
			run, err := s.agentRuntime.RunDetailed(ctx, req)
			if err != nil {
				payload := newRagChatAgentServiceErrorPayload(err)
				return ragChatToolStageResult{
					backend:    "agent_runtime",
					agentError: &payload,
					result: ragtool.WorkflowResult{
						Used:          true,
						Degraded:      true,
						DegradeReason: err.Error(),
						Control:       defaultAgentWorkflowControl(),
						TraceMeta:     defaultAgentWorkflowTraceMeta(),
					},
				}, nil
			}
			emitProjectedAgentToolEvents(sink, run)
			return ragChatToolStageResult{
				result:   workflowResultFromAgentRun(run),
				backend:  "agent_runtime",
				agentRun: &run,
			}, nil
		},
		buildExtra: func(result ragChatToolStageResult) map[string]any {
			return buildAgentRuntimeToolStageTraceExtra(result)
		},
	})
}

func shouldFallbackFromAgentToolStage(result ragChatToolStageResult) bool {
	return result.agentRun == nil && result.agentError != nil
}

func buildFallbackPrompt(question string) string {
	return "Knowledge retrieval confidence is low for question: " + question + ". Respond in Chinese, clearly state no matching knowledge was found, and note the answer may rely on general model knowledge."
}

func effectiveFallbackPrompt(fallbackPrompt string, toolUsed bool, question string) string {
	if fallbackPrompt == "" {
		return ""
	}
	if toolUsed {
		return ""
	}
	return fallbackPrompt
}

func (s *RagChatService) runPromptStage(
	ctx context.Context,
	question string,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	promptCtx ragretrieve.Result,
	toolContext string,
	workflowPolicy string,
	answerGuidance string,
	systemPromptOverride string,
	traceID string,
) (ragChatPromptStageResult, error) {
	return runRagChatStage(ctx, s.tracer, traceID, ragChatStage[ragChatPromptStageResult]{
		node: ragChatTraceNode{
			NodeID:   "prompt",
			NodeType: "prompt",
			NodeName: "build_messages",
		},
		run: func(context.Context) (ragChatPromptStageResult, error) {
			promptContext := ragprompt.Context{
				Question:         question,
				MemoryContext:    memoryContext,
				SessionContext:   sessionContext,
				KnowledgeContext: promptCtx.KnowledgeContext,
				ToolContext:      toolContext,
				WorkflowPolicy:   workflowPolicy,
				AnswerGuidance:   answerGuidance,
				History:          history,
				SystemPrompt:     systemPromptOverride,
			}
			budgetResult, err := applyChatContextBudget(s.chatContextBudget, s.promptService, promptContext)
			if err != nil {
				return ragChatPromptStageResult{}, err
			}
			promptContext.History = budgetResult.History
			promptContext.MemoryContext = budgetResult.MemoryContext
			promptContext.SessionContext = budgetResult.SessionContext
			promptContext.KnowledgeContext = budgetResult.KnowledgeContext
			promptContext.ToolContext = budgetResult.ToolContext
			emitPreferenceOverrideObservability(ctx, question, promptContext.MemoryContext)
			override, hasOverride := detectPreferenceOverride(question, promptContext.MemoryContext)
			messages, err := s.promptService.BuildMessages(promptContext)
			if err != nil {
				return ragChatPromptStageResult{}, err
			}
			normalizedBudget := s.chatContextBudget.normalized()
			budgetResult.EstimatedPromptTokens = estimateChatMessagesTokensWithOverhead(
				messages,
				normalizedBudget.Estimator,
				normalizedBudget.MessageOverheadTokens,
			)
			budgetResult.StageTokens = estimateChatContextStageTokens(
				s.promptService,
				promptContext,
				normalizedBudget.Estimator,
				normalizedBudget.MessageOverheadTokens,
				budgetResult.EstimatedPromptTokens,
			)
			result := ragChatPromptStageResult{messages: messages, budget: budgetResult}
			if hasOverride {
				overrideCopy := override
				result.override = &overrideCopy
			}
			return result, nil
		},
		buildExtra: func(result ragChatPromptStageResult) map[string]any {
			extra := map[string]any{
				"messageCount": len(result.messages),
			}
			for key, value := range chatContextBudgetTraceExtra(result.budget) {
				extra[key] = value
			}
			if result.override != nil {
				extra["preferenceOverride"] = map[string]any{
					"detected":        true,
					"canonicalKey":    result.override.CanonicalKey,
					"historicalValue": result.override.HistoricalValue,
					"currentValue":    result.override.CurrentValue,
				}
			}
			return extra
		},
	})
}

func defaultWorkflowControl() ragtool.WorkflowControl {
	return ragtool.WorkflowControl{
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}

func defaultAgentWorkflowControl() ragtool.WorkflowControl {
	return ragtool.WorkflowControl{
		Capability:          ragtool.CapabilityGeneral,
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}

func defaultAgentWorkflowTraceMeta() ragtool.WorkflowTraceMeta {
	return ragtool.WorkflowTraceMeta{
		Capability:          ragtool.CapabilityGeneral,
		ExecutionMode:       ragtool.ExecutionModeReadOnly,
		RiskLevel:           ragtool.RiskLevelLow,
		ApprovalRequirement: ragtool.ApprovalRequirementNone,
	}
}
