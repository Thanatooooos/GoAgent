package chat

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	agentstate "local/rag-project/internal/app/agent/state"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/exception"
)

func (s *RagChatService) ResumeAfterApproval(ctx context.Context, input RagChatApprovalResumeInput, sink RagChatEventSink) error {
	if sink == nil {
		return exception.NewServiceException("rag chat event sink is required", nil)
	}
	if err := s.validateAgentRuntimeDependencies(); err != nil {
		return err
	}

	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return exception.NewClientException("user id is required", nil)
	}
	if strings.TrimSpace(input.ConversationID) == "" {
		return exception.NewClientException("conversation id is required", nil)
	}
	if strings.TrimSpace(input.CheckpointID) == "" {
		return exception.NewClientException("checkpoint id is required", nil)
	}

	state, err := s.newAgentRuntimeState(ctx, strings.TrimSpace(input.ConversationID), userID)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}
	if err := sink.SendMeta(state.meta); err != nil {
		return err
	}

	result, err := s.runAgentRuntimeResumeStage(ctx, input, state)
	if err != nil {
		return s.handleAgentRuntimeError(ctx, state, sink, err)
	}
	return s.handleAgentRuntimeResult(ctx, state, RagChatInput{
		ConversationID: strings.TrimSpace(input.ConversationID),
		UserID:         userID,
		Question:       strings.TrimSpace(input.Question),
	}, result, sink)
}

func (s *RagChatService) runAgentChat(ctx context.Context, input RagChatInput, sink RagChatEventSink) error {
	if err := s.validateAgentRuntimeDependencies(); err != nil {
		return err
	}

	conversationStage, err := s.runConversationStage(ctx, input)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}
	userMessageStage, err := s.runUserMessageStage(ctx, input, conversationStage.conversationID)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}
	runtimeStage, err := s.runRuntimeStage(ctx, input, conversationStage, userMessageStage)
	if err != nil {
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if err := sink.SendMeta(runtimeStage.state.meta); err != nil {
		return err
	}
	ctx = enrichRagChatLogContext(
		ctx,
		runtimeStage.state.traceID,
		runtimeStage.state.meta.ConversationID,
		input.UserID,
		runtimeStage.state.meta.TaskID,
	)
	s.tracer.appendTraceRunExtra(ctx, runtimeStage.state.traceID, buildRuntimePathTraceExtra(chatPathAgentRuntimeTopLevel, toolBackendAgentRuntime, input, s.agentRuntimeMode))
	if strings.TrimSpace(runtimeStage.state.title) != "" {
		_ = sink.SendTitle(runtimeStage.state.title)
	}

	result, err := s.runAgentRuntimeStage(ctx, input, runtimeStage.state)
	if err != nil {
		return s.handleAgentRuntimeError(ctx, runtimeStage.state, sink, err)
	}
	return s.handleAgentRuntimeResult(ctx, runtimeStage.state, input, result, sink)
}

func (s *RagChatService) runAgentRuntimeStage(ctx context.Context, input RagChatInput, state ragChatRuntimeState) (agentapp.RunResponse, error) {
	return runRagChatStage(ctx, s.tracer, state.traceID, ragChatStage[agentapp.RunResponse]{
		node: ragChatTraceNode{
			NodeID:   "agent_runtime",
			NodeType: "agent",
			NodeName: "agent_runtime_run",
		},
		run: func(ctx context.Context) (agentapp.RunResponse, error) {
			return s.agentRuntime.RunDetailed(ctx, agentapp.Request{
				Question:  strings.TrimSpace(input.Question),
				UserID:    strings.TrimSpace(input.UserID),
				TraceID:   strings.TrimSpace(state.traceID),
				ToolStage: topLevelAgentToolStageContext(input),
				Options: agentapp.RequestOptions{
					RequireApproval: input.RequireApproval,
					OutputMode:      agentstate.OutputModeFinalAnswer,
				},
			})
		},
		buildExtra: func(result agentapp.RunResponse) map[string]any {
			return buildAgentRuntimeTraceExtra(result)
		},
		buildErrorExtra: buildAgentRuntimeServiceErrorTraceExtra,
	})
}

func (s *RagChatService) runAgentRuntimeResumeStage(ctx context.Context, input RagChatApprovalResumeInput, state ragChatRuntimeState) (agentapp.RunResponse, error) {
	return runRagChatStage(ctx, s.tracer, state.traceID, ragChatStage[agentapp.RunResponse]{
		node: ragChatTraceNode{
			NodeID:   "agent_runtime_resume",
			NodeType: "agent",
			NodeName: "agent_runtime_resume",
		},
		run: func(ctx context.Context) (agentapp.RunResponse, error) {
			return s.agentRuntime.ResumeAfterApproval(ctx, agentapp.ResumeApprovalRequest{
				CheckpointID: strings.TrimSpace(input.CheckpointID),
				Decision:     strings.TrimSpace(input.Decision),
				DecisionNote: strings.TrimSpace(input.DecisionNote),
			})
		},
		buildExtra: func(result agentapp.RunResponse) map[string]any {
			return buildAgentRuntimeTraceExtra(result)
		},
		buildErrorExtra: buildAgentRuntimeServiceErrorTraceExtra,
	})
}

func (s *RagChatService) handleAgentRuntimeResult(
	ctx context.Context,
	state ragChatRuntimeState,
	input RagChatInput,
	result agentapp.RunResponse,
	sink RagChatEventSink,
) error {
	logRagChatAgentRuntimeResult(ctx, result)
	outcomePayload := newRagChatAgentOutcomePayload(result.Outcome)
	_ = sink.SendAgentOutcome(outcomePayload)

	if result.Outcome.Status == agentapp.RunStatusAwaitingApproval {
		approvalPayload := newRagChatApprovalPendingPayload(result.Outcome.Approval)
		if approvalPayload != nil {
			_ = sink.SendApprovalPending(*approvalPayload)
		}
		if s.tracer != nil {
			s.tracer.appendTraceRunExtra(ctx, state.traceID, map[string]any{
				"agentRuntime": map[string]any{
					"status":       outcomePayload.Status,
					"checkpointId": outcomePayload.CheckpointID,
				},
			})
			s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusRunning, nil)
		}
		_ = sink.SendDone()
		return nil
	}

	content := buildAgentAssistantContent(result)
	if strings.TrimSpace(content) != "" {
		_ = sink.SendMessage(content)
	}

	payload, err := s.persistAssistantMessage(ctx, state, input, content, "")
	if err != nil {
		if s.tracer != nil {
			s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
		}
		_ = sink.SendError(err)
		_ = sink.SendDone()
		return err
	}

	if s.tracer != nil {
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusSuccess, nil)
	}
	_ = sink.SendFinish(payload)
	_ = sink.SendDone()
	return nil
}

func (s *RagChatService) handleAgentRuntimeError(
	ctx context.Context,
	state ragChatRuntimeState,
	sink RagChatEventSink,
	err error,
) error {
	ctx = enrichRagChatLogContext(ctx, state.traceID, state.meta.ConversationID, "", state.meta.TaskID)
	logRagChatTerminalError(ctx, "agent_runtime", err)
	if s.tracer != nil {
		s.tracer.appendTraceRunExtra(ctx, state.traceID, map[string]any{
			"agentRuntime": buildAgentRuntimeServiceErrorTraceExtra(err),
		})
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusFailed, err)
	}
	_ = sink.SendAgentServiceError(newRagChatAgentServiceErrorPayload(err))
	_ = sink.SendError(err)
	_ = sink.SendDone()
	return err
}

func (s *RagChatService) newAgentRuntimeState(ctx context.Context, conversationID string, userID string) (ragChatRuntimeState, error) {
	traceID, err := nextRagTraceID()
	if err != nil {
		return ragChatRuntimeState{}, err
	}
	taskID, err := nextRagTaskID()
	if err != nil {
		return ragChatRuntimeState{}, err
	}
	state := ragChatRuntimeState{
		meta: RagChatMeta{
			ConversationID: strings.TrimSpace(conversationID),
			TaskID:         taskID,
		},
		traceID:   traceID,
		startTime: s.tracer.now(),
	}
	_ = s.tracer.startTraceRunAt(ctx, traceID, strings.TrimSpace(conversationID), taskID, strings.TrimSpace(userID), state.startTime)
	return state, nil
}

func (s *RagChatService) validateAgentRuntimeDependencies() error {
	if s == nil {
		return exception.NewServiceException("rag chat service is required", nil)
	}
	if s.conversationService == nil {
		return exception.NewServiceException("conversation service is required", nil)
	}
	if s.messageService == nil {
		return exception.NewServiceException("conversation message service is required", nil)
	}
	if s.agentRuntime == nil {
		return exception.NewServiceException("agent runtime service is required", nil)
	}
	if s.tracer == nil {
		return exception.NewServiceException("chat tracer is required", nil)
	}
	return nil
}

func buildAgentAssistantContent(result agentapp.RunResponse) string {
	switch {
	case strings.TrimSpace(result.Response.Summary) != "":
		return strings.TrimSpace(result.Response.Summary)
	case strings.TrimSpace(result.Response.CombinedText) != "":
		return strings.TrimSpace(result.Response.CombinedText)
	case strings.TrimSpace(result.Response.DegradeReason) != "":
		return strings.TrimSpace(result.Response.DegradeReason)
	default:
		return ""
	}
}

func workflowResultFromAgentRun(result agentapp.RunResponse) ragtool.WorkflowResult {
	control := defaultAgentWorkflowControl()
	traceMeta := defaultAgentWorkflowTraceMeta()
	projected := projectAgentToolEvents(result)
	if approval := result.Outcome.Approval; approval != nil {
		switch strings.TrimSpace(strings.ToLower(approval.RiskLevel)) {
		case ragtool.RiskLevelMedium, ragtool.RiskLevelHigh:
			control.RiskLevel = strings.TrimSpace(strings.ToLower(approval.RiskLevel))
			traceMeta.RiskLevel = control.RiskLevel
		}
		if approval.Required || strings.TrimSpace(approval.Status) != "" {
			control.ApprovalRequirement = ragtool.ApprovalRequirementRequired
			traceMeta.ApprovalRequirement = ragtool.ApprovalRequirementRequired
		}
		if family := strings.TrimSpace(approval.CapabilityFamily); family != "" {
			traceMeta.Capability = family
		}
	}
	if traceMeta.Capability == ragtool.CapabilityGeneral && strings.TrimSpace(result.Response.Query) != "" {
		control.Capability = ragtool.CapabilityDiagnosis
		traceMeta.Capability = ragtool.CapabilityDiagnosis
	}
	return ragtool.WorkflowResult{
		Used:           true,
		Context:        buildAgentToolContext(result),
		AnswerGuidance: buildAgentToolAnswerGuidance(result),
		Control:        control.Normalize(),
		TraceMeta:      traceMeta.Normalize(),
		Calls:          projected.calls,
		Rounds:         projected.rounds,
		Degraded:       result.Outcome.Status == agentapp.RunStatusDegraded || result.Response.Degraded,
		DegradeReason:  firstNonEmptyString(result.Response.DegradeReason, result.Outcome.InterruptReason),
	}
}

func buildAgentToolContext(result agentapp.RunResponse) string {
	parts := make([]string, 0, 5)
	if query := strings.TrimSpace(result.Response.Query); query != "" {
		parts = append(parts, "Agent query: "+query)
	}
	if summary := strings.TrimSpace(result.Response.Summary); summary != "" {
		parts = append(parts, "Agent summary: "+summary)
	}
	if provider := strings.TrimSpace(result.Response.Provider); provider != "" {
		parts = append(parts, "Agent provider: "+provider)
	}
	if combined := strings.TrimSpace(result.Response.CombinedText); combined != "" {
		parts = append(parts, "Agent evidence:\n"+combined)
	}
	if sources := strings.TrimSpace(buildAgentToolSourceContext(result)); sources != "" {
		parts = append(parts, sources)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func buildAgentToolAnswerGuidance(result agentapp.RunResponse) string {
	sourceInstruction := buildAgentSourceAnswerInstruction(result)
	if summary := strings.TrimSpace(result.Response.Summary); summary != "" {
		return strings.TrimSpace(fmt.Sprintf(
			"Use the agent runtime summary and evidence to answer the user consistently.\nSummary: %s\n%s",
			summary,
			sourceInstruction,
		))
	}
	if reason := strings.TrimSpace(result.Response.DegradeReason); reason != "" {
		return fmt.Sprintf("Agent runtime degraded. Explain the limitation clearly.\nReason: %s", reason)
	}
	return strings.TrimSpace(sourceInstruction)
}

func buildAgentToolSourceContext(result agentapp.RunResponse) string {
	sources := collectAgentToolSources(result)
	if len(sources) == 0 {
		return ""
	}
	lines := make([]string, 0, len(sources)+1)
	lines = append(lines, "Agent sources:")
	for idx, source := range sources {
		lines = append(lines, fmt.Sprintf("[%d] %s", idx+1, source))
	}
	return strings.Join(lines, "\n")
}

func buildAgentSourceAnswerInstruction(result agentapp.RunResponse) string {
	sources := collectAgentToolSources(result)
	if len(sources) == 0 {
		return ""
	}
	return "When you rely on external evidence, explicitly include a `来源` section at the end of the answer and list the source titles or URLs you used. Do not invent sources that are not present in the tool context."
}

func collectAgentToolSources(result agentapp.RunResponse) []string {
	if len(result.Response.Results) == 0 && len(result.Response.Pages) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	sources := make([]string, 0, len(result.Response.Results)+len(result.Response.Pages))
	appendSource := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		sources = append(sources, value)
	}
	for _, item := range result.Response.Results {
		appendSource(renderAgentToolSource(item.Title, item.URL))
	}
	for _, page := range result.Response.Pages {
		appendSource(renderAgentToolSource("", page.URL))
	}
	return sources
}

func renderAgentToolSource(title string, rawURL string) string {
	title = strings.TrimSpace(title)
	rawURL = strings.TrimSpace(rawURL)
	if title == "" {
		return rawURL
	}
	if rawURL == "" {
		return title
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return fmt.Sprintf("%s - %s", title, rawURL)
	}
	return fmt.Sprintf("%s - %s", title, rawURL)
}

func (s *RagChatService) handleAgentToolStageApproval(ctx context.Context, state ragChatRuntimeState, sink RagChatEventSink, result agentapp.RunResponse) error {
	outcomePayload := newRagChatAgentOutcomePayload(result.Outcome)
	_ = sink.SendAgentOutcome(outcomePayload)
	if approvalPayload := newRagChatApprovalPendingPayload(result.Outcome.Approval); approvalPayload != nil {
		_ = sink.SendApprovalPending(*approvalPayload)
	}
	if s.tracer != nil {
		s.tracer.finishTraceRun(ctx, state.traceID, ragTraceStatusRunning, nil)
	}
	_ = sink.SendDone()
	return nil
}
