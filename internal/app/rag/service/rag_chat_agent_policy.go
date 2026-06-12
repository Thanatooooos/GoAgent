package service

import (
	"strings"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

const (
	ragChatAgentModeOff        = "off"
	ragChatAgentModeDiagnostic = "diagnostic"
	ragChatAgentModeAlways     = "always"
)

func (s *RagChatService) shouldUseAgentRuntimeForToolStage(input RagChatInput, rewriteResult ragrewrite.Result, retrievalUsed bool) bool {
	if s == nil || s.agentRuntime == nil {
		return false
	}
	if input.UseAgentRuntime {
		return true
	}
	if !shouldRunToolWorkflow(input, rewriteResult, retrievalUsed) {
		return false
	}
	switch normalizeAgentRuntimeMode(s.agentRuntimeMode) {
	case ragChatAgentModeAlways:
		return true
	case ragChatAgentModeDiagnostic:
		return isAgentDiagnosticQuestion(input.Question)
	default:
		return false
	}
}

func normalizeAgentRuntimeMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case ragChatAgentModeAlways:
		return ragChatAgentModeAlways
	case ragChatAgentModeDiagnostic:
		return ragChatAgentModeDiagnostic
	default:
		return ragChatAgentModeOff
	}
}

func isAgentDiagnosticQuestion(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	if ragtool.FirstMatchedID(ragtool.DocumentIDPattern, question) != "" {
		return true
	}
	if ragtool.FirstMatchedID(ragtool.TaskIDPattern, question) != "" {
		return true
	}
	if ragtool.FirstMatchedID(ragtool.TraceIDPattern, question) != "" {
		return true
	}

	lower := strings.ToLower(question)
	diagnosticMarkers := []string{
		"why failed",
		"diagnose",
		"diagnosis",
		"root cause",
		"trace failed",
		"document failed",
		"task failed",
		"失败",
		"报错",
		"错误",
		"根因",
		"排查",
		"诊断",
		"追踪",
	}
	for _, marker := range diagnosticMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
