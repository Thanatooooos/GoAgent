package profile

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

const (
	defaultObservationBatchSize = 20
	maxObservationSourceRunes   = 6000
)

// ObservationStateRepository owns durable scheduling state. The worker claims
// work before calling the LLM, so a second runtime will not process it too.
type ObservationStateRepository interface {
	ClaimDue(context.Context, time.Time, int) ([]domain.ConversationProfileState, error)
	SetProcessingBoundary(context.Context, string, string, time.Time) error
	Complete(context.Context, string, string, time.Time) error
	Fail(context.Context, string, time.Time) error
}

type Worker struct {
	state         ObservationStateRepository
	profiles      *Service
	observer      *Observer
	conversations port.ConversationRepository
	messages      port.ConversationMessageRepository
	summaries     port.ConversationSummaryRepository
	now           func() time.Time
}

func NewWorker(state ObservationStateRepository, profiles *Service, observer *Observer, conversations port.ConversationRepository, messages port.ConversationMessageRepository, summaries port.ConversationSummaryRepository) *Worker {
	return &Worker{state: state, profiles: profiles, observer: observer, conversations: conversations, messages: messages, summaries: summaries, now: time.Now}
}

// RunDue observes sessions which have been idle long enough. The message high
// water mark is fixed before material is read; messages added afterwards are
// deliberately left for the next run.
func (w *Worker) RunDue(ctx context.Context) error {
	if w == nil || w.state == nil || w.profiles == nil || w.observer == nil || w.conversations == nil || w.messages == nil {
		return nil
	}
	now := w.now()
	states, err := w.state.ClaimDue(ctx, now, defaultObservationBatchSize)
	if err != nil {
		return err
	}
	for _, state := range states {
		if err := w.runOne(ctx, state, now); err != nil {
			// A failed observation must not stop unrelated sessions in this batch.
			_ = w.state.Fail(ctx, state.ConversationID, now.Add(observationRetryDelay(state.Attempts)))
		}
	}
	return nil
}

func (w *Worker) runOne(ctx context.Context, state domain.ConversationProfileState, now time.Time) error {
	throughID, err := w.messages.FindMaxIDAtOrBefore(ctx, state.ConversationID, state.UserID, now)
	if err != nil {
		return fmt.Errorf("fix profile observation boundary: %w", err)
	}
	if throughID == "" {
		return w.state.Complete(ctx, state.ConversationID, "", now)
	}
	if err := w.state.SetProcessingBoundary(ctx, state.ConversationID, throughID, now); err != nil {
		return fmt.Errorf("save profile observation boundary: %w", err)
	}

	conversations, err := w.conversations.ListByUserID(ctx, state.UserID)
	if err != nil {
		return fmt.Errorf("list user conversations: %w", err)
	}
	if len(conversations) < 2 {
		return w.state.Complete(ctx, state.ConversationID, throughID, now)
	}

	material, err := w.sessionMaterial(ctx, state, throughID)
	if err != nil {
		return err
	}
	if material == "" {
		return w.state.Complete(ctx, state.ConversationID, throughID, now)
	}
	current, err := w.profiles.Get(ctx, state.UserID)
	if err != nil {
		return fmt.Errorf("load derived profile: %w", err)
	}
	if current.UserID == "" {
		current.UserID = state.UserID
	}
	result, err := w.observer.Observe(ctx, ObservationInput{Profile: current.ContentMarkdown, Conversation: material})
	if err != nil {
		return err
	}
	if _, err := w.profiles.ApplyObservation(ctx, current, result); err != nil {
		return fmt.Errorf("apply profile observation: %w", err)
	}
	return w.state.Complete(ctx, state.ConversationID, throughID, now)
}

func (w *Worker) sessionMaterial(ctx context.Context, state domain.ConversationProfileState, throughID string) (string, error) {
	if w.summaries != nil {
		summary, err := w.summaries.FindLatestByConversationIDAndUserID(ctx, state.ConversationID, state.UserID)
		if err != nil {
			return "", fmt.Errorf("load session summary: %w", err)
		}
		if summary.QualityStatus == domain.SummaryQualityAccepted && summary.CoveredToMessageID == throughID && strings.TrimSpace(summary.Content) != "" {
			return trimRunes(summary.Content, maxObservationSourceRunes), nil
		}
	}
	messages, err := w.messages.List(ctx, port.ConversationMessageListFilter{
		ConversationID: state.ConversationID,
		UserID:         state.UserID,
		Roles:          []string{"user"},
		ThroughID:      throughID,
		Order:          port.ConversationMessageOrderAsc,
		Limit:          500,
	})
	if err != nil {
		return "", fmt.Errorf("load session user messages: %w", err)
	}
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if content := strings.TrimSpace(message.Content); content != "" {
			parts = append(parts, content)
		}
	}
	return trimRunes(strings.Join(parts, "\n\n"), maxObservationSourceRunes), nil
}

func observationRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		return 5 * time.Minute
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Duration(1<<uint(attempts-1)) * 5 * time.Minute
}

func trimRunes(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if maximum <= 0 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	return string([]rune(value)[:maximum])
}
