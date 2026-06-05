package agent

import (
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	"local/rag-project/internal/framework/log"
)

func logAgentServiceInitialized(pattern string, runtimeName string, maxIterations int, outputMode string) {
	log.Infof(
		"agent service initialized: pattern=%s runtime=%s maxIterations=%d outputMode=%s",
		strings.TrimSpace(pattern),
		strings.TrimSpace(runtimeName),
		maxIterations,
		strings.TrimSpace(outputMode),
	)
}

func logAgentRunStart(req Request, pattern string, runtimeName string, maxIterations int) {
	log.Infof(
		"agent run start: traceID=%s userID=%s pattern=%s runtime=%s requireApproval=%t outputMode=%s maxIterations=%d toolStage=%t",
		strings.TrimSpace(req.TraceID),
		strings.TrimSpace(req.UserID),
		strings.TrimSpace(pattern),
		strings.TrimSpace(runtimeName),
		req.Options.RequireApproval,
		strings.TrimSpace(req.Options.OutputMode),
		firstPositive(req.Options.MaxIterations, maxIterations),
		req.ToolStage != nil,
	)
}

func logAgentToolStageSeed(req Request, session *agentruntime.RuntimeSession) {
	if req.ToolStage == nil || session == nil {
		return
	}
	log.Infof(
		"agent tool-stage context seeded: traceID=%s conversationID=%s knowledgeBases=%d subQuestions=%d searchChannels=%d notes=%d needRetrieval=%t rewrittenQuestion=%t",
		strings.TrimSpace(req.TraceID),
		strings.TrimSpace(session.Request.ConversationID),
		len(session.Snapshot.Request.KnowledgeBaseIDs),
		len(req.ToolStage.SubQuestions),
		len(req.ToolStage.SearchChannels),
		len(session.Snapshot.Context.Notes),
		req.ToolStage.NeedRetrieval,
		strings.TrimSpace(req.ToolStage.RewrittenQuestion) != "",
	)
}

func logAgentRunCompleted(session *agentruntime.RuntimeSession, outcome RunOutcome) {
	if session == nil {
		return
	}
	response := responseFromSession(session)
	log.Infof(
		"agent run completed: traceID=%s sessionID=%s status=%s checkpointID=%s interrupted=%t degraded=%t provider=%s results=%d pages=%d notes=%d",
		strings.TrimSpace(session.Request.TraceID),
		strings.TrimSpace(session.SessionID),
		strings.TrimSpace(outcome.Status),
		strings.TrimSpace(outcome.CheckpointID),
		outcome.Interrupted,
		response.Degraded,
		strings.TrimSpace(response.Provider),
		len(response.Results),
		len(response.Pages),
		len(session.Snapshot.Context.Notes),
	)
}

func logAgentResumeStart(req ResumeApprovalRequest) {
	log.Infof(
		"agent resume start: checkpointID=%s decision=%s approved=%t hasDecisionNote=%t",
		strings.TrimSpace(req.CheckpointID),
		strings.TrimSpace(req.Decision),
		req.Approved,
		strings.TrimSpace(req.DecisionNote) != "",
	)
}

func logAgentExecutionError(operation string, traceID string, checkpointID string, err error) {
	if err == nil {
		return
	}
	desc := DescribeServiceError(err)
	log.Warnf(
		"agent execution failed: operation=%s traceID=%s checkpointID=%s code=%s kind=%s retryable=%t err=%v",
		strings.TrimSpace(operation),
		strings.TrimSpace(traceID),
		strings.TrimSpace(checkpointID),
		strings.TrimSpace(desc.Code),
		strings.TrimSpace(desc.Kind),
		desc.Retryable,
		err,
	)
}
