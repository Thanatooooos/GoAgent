package profile

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

func TestWorkerRunDueUsesFixedBoundaryAndCompletes(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	state := &workerStateStub{due: []domain.ConversationProfileState{{ConversationID: "c1", UserID: "u1", Attempts: 1}}}
	profiles := &workerProfileRepoStub{}
	worker := NewWorker(state, NewService(profiles), NewObserver(nil), workerConversationRepoStub{items: []domain.Conversation{{ConversationID: "c1"}, {ConversationID: "c2"}}}, workerMessageRepoStub{
		maxID: "m2",
		items: []domain.ConversationMessage{{ID: "m1", Content: "durable project fact"}, {ID: "m2", Content: "another fact"}},
	}, nil)
	worker.now = func() time.Time { return now }

	if err := worker.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if state.boundary != "m2" {
		t.Fatalf("boundary = %q, want m2", state.boundary)
	}
	if state.completed != "m2" {
		t.Fatalf("completed = %q, want m2", state.completed)
	}
	if state.failed != 0 {
		t.Fatalf("unexpected failed observations: %d", state.failed)
	}
}

func TestWorkerRunDueSkipsSingleSessionUser(t *testing.T) {
	state := &workerStateStub{due: []domain.ConversationProfileState{{ConversationID: "c1", UserID: "u1"}}}
	worker := NewWorker(state, NewService(&workerProfileRepoStub{}), NewObserver(nil), workerConversationRepoStub{items: []domain.Conversation{{ConversationID: "c1"}}}, workerMessageRepoStub{maxID: "m1"}, nil)
	if err := worker.RunDue(context.Background()); err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if state.completed != "m1" {
		t.Fatalf("completed = %q, want m1", state.completed)
	}
}

func TestSanitizeProfileMarkdownRemovesGuessedNameHeading(t *testing.T) {
	got := sanitizeProfileMarkdown("## User Profile: Dionysis\n\n- Uses Go")
	if got != "## Overview\n\n- Uses Go" {
		t.Fatalf("sanitizeProfileMarkdown() = %q", got)
	}
}

func TestApplyObservationSanitizesExistingProfileOnKeep(t *testing.T) {
	repo := &workerProfileRepoStub{}
	service := NewService(repo)
	updated, err := service.ApplyObservation(context.Background(), domain.UserMemoryProfile{UserID: "u1", ContentMarkdown: "## User Profile: Dionysis", Version: 1}, ObservationResult{Action: "keep"})
	if err != nil {
		t.Fatalf("ApplyObservation() error = %v", err)
	}
	if updated.ContentMarkdown != "## Overview" || updated.Version != 2 {
		t.Fatalf("updated profile = %+v", updated)
	}
}

type workerStateStub struct {
	due       []domain.ConversationProfileState
	boundary  string
	completed string
	failed    int
}

func (s *workerStateStub) ClaimDue(context.Context, time.Time, int) ([]domain.ConversationProfileState, error) {
	return s.due, nil
}
func (s *workerStateStub) SetProcessingBoundary(_ context.Context, _ string, messageID string, _ time.Time) error {
	s.boundary = messageID
	return nil
}
func (s *workerStateStub) Complete(_ context.Context, _ string, messageID string, _ time.Time) error {
	s.completed = messageID
	return nil
}
func (s *workerStateStub) Fail(context.Context, string, time.Time) error { s.failed++; return nil }

type workerProfileRepoStub struct{ item domain.UserMemoryProfile }

func (s *workerProfileRepoStub) Get(context.Context, string) (domain.UserMemoryProfile, error) {
	return s.item, nil
}
func (s *workerProfileRepoStub) Save(_ context.Context, item domain.UserMemoryProfile) (domain.UserMemoryProfile, error) {
	s.item = item
	return item, nil
}

type workerConversationRepoStub struct{ items []domain.Conversation }

func (s workerConversationRepoStub) Create(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}
func (s workerConversationRepoStub) Update(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}
func (s workerConversationRepoStub) UpdateWhere(context.Context, port.ConversationConditions, port.ConversationPatch) (int64, error) {
	return 0, nil
}
func (s workerConversationRepoStub) Delete(context.Context, string) error { return nil }
func (s workerConversationRepoStub) GetByID(context.Context, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}
func (s workerConversationRepoStub) GetByConversationIDAndUserID(context.Context, string, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}
func (s workerConversationRepoStub) ListByUserID(context.Context, string) ([]domain.Conversation, error) {
	return s.items, nil
}

type workerMessageRepoStub struct {
	maxID string
	items []domain.ConversationMessage
}

func (s workerMessageRepoStub) Create(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}
func (s workerMessageRepoStub) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}
func (s workerMessageRepoStub) List(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	return s.items, nil
}
func (s workerMessageRepoStub) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (s workerMessageRepoStub) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return s.maxID, nil
}
func (s workerMessageRepoStub) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}
