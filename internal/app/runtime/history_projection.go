package runtime

import (
	"encoding/json"
	"fmt"
)

// ProjectHistory reconstructs the model protocol from durable runtime facts.
// Thinking and live deltas are deliberately absent.
func ProjectHistory(question string, entries []JournalEntry) ([]ModelMessage, error) {
	return ProjectJournalHistory([]ModelMessage{{Role: ModelRoleUser, Content: question}}, entries)
}

// ProjectJournalHistory appends this session's durable assistant and tool
// facts to already-projected conversation history. The caller owns injecting
// the current user message before this function is called.
func ProjectJournalHistory(history []ModelMessage, entries []JournalEntry) ([]ModelMessage, error) {
	messages := append([]ModelMessage(nil), history...)
	for _, entry := range entries {
		switch entry.EventType {
		case EventAssistantTurn:
			var turn struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			}
			if err := json.Unmarshal([]byte(entry.Detail), &turn); err != nil {
				return nil, fmt.Errorf("decode assistant turn: %w", err)
			}
			messages = append(messages, ModelMessage{Role: ModelRoleAssistant, Content: turn.Content, ToolCalls: turn.ToolCalls})
		case EventToolSettled:
			if entry.ToolCallID == "" {
				continue
			}
			content := entry.Detail
			var result struct {
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(entry.Detail), &result) == nil && result.Content != "" {
				content = result.Content
			}
			messages = append(messages, ModelMessage{Role: ModelRoleTool, ToolCallID: entry.ToolCallID, Content: content})
		}
	}
	return messages, nil
}
