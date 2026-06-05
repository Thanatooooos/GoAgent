package agent

import (
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
)

func seedRuntimeSessionFromToolStage(session *agentruntime.RuntimeSession, context *ToolStageContext) {
	if session == nil || context == nil {
		return
	}

	if conversationID := strings.TrimSpace(context.ConversationID); conversationID != "" {
		session.Request.ConversationID = conversationID
		session.Snapshot.Request.ConversationID = conversationID
	}
	if knowledgeBaseIDs := uniqueTrimmedStrings(context.KnowledgeBaseIDs); len(knowledgeBaseIDs) > 0 {
		session.Snapshot.Request.KnowledgeBaseIDs = knowledgeBaseIDs
	}

	rewrittenQuestion := strings.TrimSpace(context.RewrittenQuestion)
	if rewrittenQuestion != "" {
		session.Snapshot.Context.RewrittenQuery = rewrittenQuestion
	}
	session.Snapshot.Context.SearchQuery = firstNonEmpty(
		rewrittenQuestion,
		strings.TrimSpace(session.Request.Question),
		strings.TrimSpace(session.Snapshot.Request.Question),
	)

	notes := buildToolStageNotes(context)
	if len(notes) > 0 {
		session.Snapshot.Context.Notes = append([]string(nil), notes...)
	}
}

func buildToolStageNotes(context *ToolStageContext) []string {
	if context == nil {
		return nil
	}

	notes := make([]string, 0, 8)
	if len(context.SubQuestions) > 0 {
		notes = append(notes, "tool-stage sub-questions: "+strings.Join(uniqueTrimmedStrings(context.SubQuestions), " | "))
	}
	if context.NeedRetrieval {
		notes = append(notes, "tool-stage retrieval was requested before agent handoff")
	}
	if summary := summarizeToolStageText("tool-stage history summary", context.HistorySummary, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage session context", context.SessionContext, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage memory context", context.MemoryContext, 320); summary != "" {
		notes = append(notes, summary)
	}
	if summary := summarizeToolStageText("tool-stage knowledge context", context.KnowledgeContext, 400); summary != "" {
		notes = append(notes, summary)
	}
	if len(context.SearchChannels) > 0 {
		notes = append(notes, "tool-stage search channels: "+strings.Join(uniqueTrimmedStrings(context.SearchChannels), ", "))
	}
	return uniqueTrimmedStrings(notes)
}

func summarizeToolStageText(label string, value string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if trimmed == "" {
		return ""
	}
	if limit > 0 && len(trimmed) > limit {
		trimmed = strings.TrimSpace(trimmed[:limit-3]) + "..."
	}
	return label + ": " + trimmed
}

func uniqueTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
