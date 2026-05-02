package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

type conversationDeleteTransactionStub struct {
	runFn func(
		ctx context.Context,
		fn func(
			ctx context.Context,
			conversationRepo port.ConversationRepository,
			messageRepo port.ConversationMessageRepository,
			summaryRepo port.ConversationSummaryRepository,
		) error,
	) error
}

func (s conversationDeleteTransactionStub) Run(
	ctx context.Context,
	fn func(
		ctx context.Context,
		conversationRepo port.ConversationRepository,
		messageRepo port.ConversationMessageRepository,
		summaryRepo port.ConversationSummaryRepository,
	) error,
) error {
	return s.runFn(ctx, fn)
}

func (s conversationDeleteTransactionStub) asFunc() ConversationDeleteTransaction {
	return func(
		ctx context.Context,
		fn func(
			ctx context.Context,
			conversationRepo port.ConversationRepository,
			messageRepo port.ConversationMessageRepository,
			summaryRepo port.ConversationSummaryRepository,
		) error,
	) error {
		return s.runFn(ctx, fn)
	}
}

type conversationRepoStub struct {
	createFn                   func(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error)
	updateFn                   func(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error)
	deleteFn                   func(ctx context.Context, id string) error
	getByConversationIDAndUser func(ctx context.Context, conversationID string, userID string) (domain.Conversation, error)
	listByUserIDFn             func(ctx context.Context, userID string) ([]domain.Conversation, error)
}

func (s conversationRepoStub) Create(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	return s.createFn(ctx, conversation)
}

func (s conversationRepoStub) Update(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	return s.updateFn(ctx, conversation)
}

func (s conversationRepoStub) UpdateWhere(context.Context, port.ConversationConditions, port.ConversationPatch) (int64, error) {
	return 0, nil
}

func (s conversationRepoStub) Delete(ctx context.Context, id string) error {
	return s.deleteFn(ctx, id)
}

func (s conversationRepoStub) GetByID(context.Context, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationRepoStub) GetByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.Conversation, error) {
	return s.getByConversationIDAndUser(ctx, conversationID, userID)
}

func (s conversationRepoStub) ListByUserID(ctx context.Context, userID string) ([]domain.Conversation, error) {
	return s.listByUserIDFn(ctx, userID)
}

type conversationMessageRepoStub struct {
	deleteFn func(ctx context.Context, conversationID string, userID string) error
}

func (s conversationMessageRepoStub) Create(context.Context, domain.ConversationMessage) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (s conversationMessageRepoStub) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (s conversationMessageRepoStub) List(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	return nil, nil
}

func (s conversationMessageRepoStub) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (s conversationMessageRepoStub) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (s conversationMessageRepoStub) DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error {
	return s.deleteFn(ctx, conversationID, userID)
}

type conversationSummaryRepoStub struct {
	deleteFn func(ctx context.Context, conversationID string, userID string) error
}

func (s conversationSummaryRepoStub) Create(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
	return domain.ConversationSummary{}, nil
}

func (s conversationSummaryRepoStub) FindLatestByConversationIDAndUserID(context.Context, string, string) (domain.ConversationSummary, error) {
	return domain.ConversationSummary{}, nil
}

func (s conversationSummaryRepoStub) DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error {
	return s.deleteFn(ctx, conversationID, userID)
}

func TestConversationServiceCreateOrUpdateCreatesWhenMissing(t *testing.T) {
	var created domain.Conversation
	service := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				created = conversation
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			deleteFn: func(context.Context, string) error { return nil },
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
		},
		conversationMessageRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		conversationSummaryRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		nil,
		nil,
		12,
		conversationDeleteTransactionStub{
			runFn: func(
				ctx context.Context,
				fn func(context.Context, port.ConversationRepository, port.ConversationMessageRepository, port.ConversationSummaryRepository) error,
			) error {
				return fn(ctx, conversationRepoStub{}, conversationMessageRepoStub{}, conversationSummaryRepoStub{})
			},
		}.asFunc(),
	)

	result, err := service.CreateOrUpdate(context.Background(), CreateOrUpdateConversationInput{
		ConversationID: "c1",
		UserID:         "u1",
		Question:       "这是一个很长的问题标题用来测试截断能力",
	})
	if err != nil {
		t.Fatalf("CreateOrUpdate returned error: %v", err)
	}
	if result.ConversationID != "c1" || result.UserID != "u1" {
		t.Fatalf("unexpected conversation: %#v", result)
	}
	if created.ID == "" {
		t.Fatal("expected generated id")
	}
	if created.Title == "" {
		t.Fatal("expected generated title")
	}
}

func TestConversationServiceCreateOrUpdateUpdatesExisting(t *testing.T) {
	now := time.Now()
	var updated domain.Conversation
	service := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				updated = conversation
				return conversation, nil
			},
			deleteFn: func(context.Context, string) error { return nil },
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{
					ID:             "1",
					ConversationID: "c1",
					UserID:         "u1",
					Title:          "old",
				}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
		},
		conversationMessageRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		conversationSummaryRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		nil,
		nil,
		12,
		conversationDeleteTransactionStub{
			runFn: func(
				ctx context.Context,
				fn func(context.Context, port.ConversationRepository, port.ConversationMessageRepository, port.ConversationSummaryRepository) error,
			) error {
				return fn(ctx, conversationRepoStub{}, conversationMessageRepoStub{}, conversationSummaryRepoStub{})
			},
		}.asFunc(),
	)

	_, err := service.CreateOrUpdate(context.Background(), CreateOrUpdateConversationInput{
		ConversationID: "c1",
		UserID:         "u1",
		LastTime:       &now,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdate returned error: %v", err)
	}
	if updated.ID != "1" {
		t.Fatalf("expected existing conversation to be updated, got %#v", updated)
	}
	if updated.LastTime == nil || !updated.LastTime.Equal(now) {
		t.Fatalf("expected updated last time, got %#v", updated.LastTime)
	}
}

func TestConversationServiceRenameValidatesAndUpdates(t *testing.T) {
	var updated domain.Conversation
	service := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				updated = conversation
				return conversation, nil
			},
			deleteFn: func(context.Context, string) error { return nil },
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{ID: "1", ConversationID: "c1", UserID: "u1", Title: "old"}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
		},
		conversationMessageRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		conversationSummaryRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		nil,
		nil,
		8,
		conversationDeleteTransactionStub{
			runFn: func(
				ctx context.Context,
				fn func(context.Context, port.ConversationRepository, port.ConversationMessageRepository, port.ConversationSummaryRepository) error,
			) error {
				return fn(ctx, conversationRepoStub{}, conversationMessageRepoStub{}, conversationSummaryRepoStub{})
			},
		}.asFunc(),
	)

	if err := service.Rename(context.Background(), RenameConversationInput{
		ConversationID: "c1",
		UserID:         "u1",
		Title:          "新标题",
	}); err != nil {
		t.Fatalf("Rename returned error: %v", err)
	}
	if updated.Title != "新标题" {
		t.Fatalf("expected renamed title, got %#v", updated)
	}
}

func TestConversationServiceDeleteDeletesAllResources(t *testing.T) {
	deletedConversation := false
	deletedMessages := false
	deletedSummaries := false
	service := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			deleteFn: func(_ context.Context, id string) error {
				deletedConversation = id == "1"
				return nil
			},
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{ID: "1", ConversationID: "c1", UserID: "u1"}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
		},
		conversationMessageRepoStub{
			deleteFn: func(_ context.Context, conversationID string, userID string) error {
				deletedMessages = conversationID == "c1" && userID == "u1"
				return nil
			},
		},
		conversationSummaryRepoStub{
			deleteFn: func(_ context.Context, conversationID string, userID string) error {
				deletedSummaries = conversationID == "c1" && userID == "u1"
				return nil
			},
		},
		nil,
		nil,
		12,
		conversationDeleteTransactionStub{
			runFn: func(
				ctx context.Context,
				fn func(context.Context, port.ConversationRepository, port.ConversationMessageRepository, port.ConversationSummaryRepository) error,
			) error {
				return fn(
					ctx,
					conversationRepoStub{
						createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
							return conversation, nil
						},
						updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
							return conversation, nil
						},
						deleteFn: func(_ context.Context, id string) error {
							deletedConversation = id == "1"
							return nil
						},
						getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
							return domain.Conversation{ID: "1", ConversationID: "c1", UserID: "u1"}, nil
						},
						listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
					},
					conversationMessageRepoStub{
						deleteFn: func(_ context.Context, conversationID string, userID string) error {
							deletedMessages = conversationID == "c1" && userID == "u1"
							return nil
						},
					},
					conversationSummaryRepoStub{
						deleteFn: func(_ context.Context, conversationID string, userID string) error {
							deletedSummaries = conversationID == "c1" && userID == "u1"
							return nil
						},
					},
				)
			},
		}.asFunc(),
	)

	if err := service.Delete(context.Background(), DeleteConversationInput{
		ConversationID: "c1",
		UserID:         "u1",
	}); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if !deletedConversation || !deletedMessages || !deletedSummaries {
		t.Fatalf("expected all deletes to run: conversation=%v messages=%v summaries=%v", deletedConversation, deletedMessages, deletedSummaries)
	}
}

func TestConversationServiceListWrapsRepoError(t *testing.T) {
	service := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				return conversation, nil
			},
			deleteFn: func(context.Context, string) error { return nil },
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) {
				return nil, errors.New("db down")
			},
		},
		conversationMessageRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		conversationSummaryRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		nil,
		nil,
		12,
		conversationDeleteTransactionStub{
			runFn: func(
				ctx context.Context,
				fn func(context.Context, port.ConversationRepository, port.ConversationMessageRepository, port.ConversationSummaryRepository) error,
			) error {
				return fn(ctx, conversationRepoStub{}, conversationMessageRepoStub{}, conversationSummaryRepoStub{})
			},
		}.asFunc(),
	)

	if _, err := service.List(context.Background(), ListConversationsInput{UserID: "u1"}); err == nil {
		t.Fatal("expected error")
	}
}
