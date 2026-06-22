package chat

import (
	"strings"

	agentapp "local/rag-project/internal/app/agent"
)

func topLevelAgentToolStageContext(input RagChatInput) *agentapp.ToolStageContext {
	conversationID := strings.TrimSpace(input.ConversationID)
	knowledgeBaseIDs := uniqueKnowledgeBaseIDs(input.KnowledgeBaseIDs)
	if conversationID == "" && len(knowledgeBaseIDs) == 0 {
		return nil
	}
	return &agentapp.ToolStageContext{
		ConversationID:   conversationID,
		KnowledgeBaseIDs: knowledgeBaseIDs,
	}
}

func uniqueKnowledgeBaseIDs(values []string) []string {
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
