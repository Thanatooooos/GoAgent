package runtime

import (
	"context"
	"errors"
	"fmt"

	"local/rag-project/internal/app/runtime/persistence"
)

// ChatPublication publishes an already generated answer. Recovery never executes
// the answer model or tools again.
type ChatPublication interface {
	Publish(context.Context, string) (ConversationMessage, JournalEntry, error)
	Recover(context.Context, string, string) (JournalEntry, error)
	CancelPending(context.Context, string, string) error
}

func (s *ChatService) CancelPublication(ctx context.Context, user, task string) error {
	if s.publication == nil {
		return nil
	}
	return s.publication.CancelPending(ctx, user, task)
}

type PublicationPendingError struct{ Cause error }

func (e *PublicationPendingError) Error() string {
	return "答案已生成，正在恢复最终结果，请稍后重新连接。"
}
func (e *PublicationPendingError) Unwrap() error { return e.Cause }
func IsPublicationPending(err error) bool {
	var pending *PublicationPendingError
	return errors.As(err, &pending)
}

func (s *ChatService) RecoverPublication(ctx context.Context, user, task string) (JournalEntry, error) {
	if s.publication == nil {
		return JournalEntry{}, nil
	}
	return s.publication.Recover(ctx, user, task)
}

func (s *ChatService) SetPublication(publication ChatPublication) { s.publication = publication }

// ReadyAnswer atomically records the final answer, execution status and pending
// publication. A store lacking this protocol must fail before execution starts.
func (l *Lifecycle) ReadyAnswer(ctx context.Context, session persistence.Session, answer string) error {
	store, ok := l.store.(interface {
		ReadyAnswer(context.Context, JournalEntry) error
	})
	if !ok {
		return fmt.Errorf("runtime store does not support answer publication")
	}
	id, err := l.ids()
	if err != nil {
		return err
	}
	return store.ReadyAnswer(ctx, JournalEntry{ID: id, RuntimeSessionID: session.ID,
		ConversationID: session.ConversationID, UserMessageID: session.UserMessageID,
		TraceID: session.TraceID, EventType: EventAnswerFinal, Detail: answer, CreatedAt: l.now().UTC()})
}

// Older journals contain an execution-only completed event without a message ID.
// It must not terminate chat replay before the durable publication event.
type publicationReplaySink struct{ EventSink }

func (s publicationReplaySink) Append(ctx context.Context, entry JournalEntry) error {
	if entry.EventType == EventCompleted && entry.Detail == "" {
		return nil
	}
	if s.EventSink == nil {
		return nil
	}
	return s.EventSink.Append(ctx, entry)
}
