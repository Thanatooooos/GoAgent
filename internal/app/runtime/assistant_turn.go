package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"local/rag-project/internal/app/runtime/persistence"
)

func (l *Lifecycle) RecordAssistantTurn(ctx context.Context, session persistence.Session, turn Turn) (JournalEntry, error) {
	payload, err := json.Marshal(struct {
		Content   string     `json:"content"`
		ToolCalls []ToolCall `json:"tool_calls"`
	}{turn.Content, turn.ToolCalls})
	if err != nil {
		return JournalEntry{}, fmt.Errorf("encode assistant turn: %w", err)
	}
	return l.appendJournal(ctx, session, JournalEntry{EventType: EventAssistantTurn, Detail: string(payload)})
}

func (l *Lifecycle) RecordEvent(ctx context.Context, session persistence.Session, eventType, detail string) (JournalEntry, error) {
	return l.appendJournal(ctx, session, JournalEntry{EventType: eventType, Detail: detail})
}

func (l *Lifecycle) appendJournal(ctx context.Context, session persistence.Session, entry JournalEntry) (JournalEntry, error) {
	if l == nil || l.store == nil || l.ids == nil {
		return JournalEntry{}, fmt.Errorf("runtime lifecycle is not configured")
	}
	id, err := l.ids()
	if err != nil {
		return JournalEntry{}, fmt.Errorf("generate runtime journal id: %w", err)
	}
	entry.ID, entry.RuntimeSessionID, entry.ConversationID, entry.UserMessageID, entry.TraceID, entry.CreatedAt = id, session.ID, session.ConversationID, session.UserMessageID, session.TraceID, l.now().UTC()
	result, err := l.store.Append(ctx, entry)
	if err != nil {
		return JournalEntry{}, fmt.Errorf("append runtime journal: %w", err)
	}
	return result, nil
}
