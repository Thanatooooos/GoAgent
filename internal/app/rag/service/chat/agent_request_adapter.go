package chat

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

func buildAgentToolStageRequest(
	input RagChatInput,
	traceID string,
	history []convention.ChatMessage,
	memoryContext string,
	sessionContext string,
	rewriteResult ragrewrite.Result,
	retrieveResult ragretrieve.Result,
) agentapp.Request {
	return agentapp.Request{
		Question: strings.TrimSpace(input.Question),
		UserID:   strings.TrimSpace(input.UserID),
		TraceID:  strings.TrimSpace(traceID),
		Options: agentapp.RequestOptions{
			RequireApproval: input.RequireApproval,
		},
		ToolStage: &agentapp.ToolStageContext{
			ConversationID:    strings.TrimSpace(input.ConversationID),
			KnowledgeBaseIDs:  append([]string(nil), input.KnowledgeBaseIDs...),
			RewrittenQuestion: strings.TrimSpace(rewriteResult.RewrittenQuestion),
			SubQuestions:      append([]string(nil), rewriteResult.SubQuestions...),
			NeedRetrieval:     rewriteResult.NeedRetrieval,
			KnowledgeContext:  strings.TrimSpace(retrieveResult.KnowledgeContext),
			SearchChannels:    append([]string(nil), retrieveResult.SearchChannels...),
			HistorySummary:    summarizeChatHistory(history, 4, 320),
			SessionContext:    strings.TrimSpace(sessionContext),
			MemoryContext:     strings.TrimSpace(memoryContext),
		},
	}
}

func summarizeChatHistory(history []convention.ChatMessage, limit int, maxChars int) string {
	if len(history) == 0 || limit <= 0 {
		return ""
	}
	if len(history) > limit {
		history = history[len(history)-limit:]
	}

	parts := make([]string, 0, len(history))
	for _, message := range history {
		content := strings.Join(strings.Fields(strings.TrimSpace(message.Content)), " ")
		if content == "" {
			continue
		}
		prefix := "context"
		switch message.Role {
		case convention.UserRole:
			prefix = "user"
		case convention.AssistantRole:
			prefix = "assistant"
		}
		parts = append(parts, prefix+": "+content)
	}
	if len(parts) == 0 {
		return ""
	}

	summary := strings.Join(parts, " || ")
	if maxChars > 0 && len(summary) > maxChars {
		summary = strings.TrimSpace(summary[:maxChars-3]) + "..."
	}
	return summary
}
