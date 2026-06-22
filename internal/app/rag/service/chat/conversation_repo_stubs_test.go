package chat

import (
	"context"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
)

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
	if s.deleteFn != nil {
		return s.deleteFn(ctx, conversationID, userID)
	}
	return nil
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
	if s.deleteFn != nil {
		return s.deleteFn(ctx, conversationID, userID)
	}
	return nil
}

type conversationMessageConversationRepoStub struct {
	getByConversationIDAndUserFn func(ctx context.Context, conversationID string, userID string) (domain.Conversation, error)
}

func (s conversationMessageConversationRepoStub) Create(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) Update(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) UpdateWhere(context.Context, port.ConversationConditions, port.ConversationPatch) (int64, error) {
	return 0, nil
}

func (s conversationMessageConversationRepoStub) Delete(context.Context, string) error {
	return nil
}

func (s conversationMessageConversationRepoStub) GetByID(context.Context, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStub) GetByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.Conversation, error) {
	return s.getByConversationIDAndUserFn(ctx, conversationID, userID)
}

func (s conversationMessageConversationRepoStub) ListByUserID(context.Context, string) ([]domain.Conversation, error) {
	return nil, nil
}

type conversationMessageRepoServiceStub struct {
	createFn func(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error)
	listFn   func(ctx context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error)
}

func (s conversationMessageRepoServiceStub) Create(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
	return s.createFn(ctx, message)
}

func (s conversationMessageRepoServiceStub) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (s conversationMessageRepoServiceStub) List(ctx context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	return s.listFn(ctx, filter)
}

func (s conversationMessageRepoServiceStub) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (s conversationMessageRepoServiceStub) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (s conversationMessageRepoServiceStub) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

type conversationSummaryRepoServiceStub struct {
	createFn func(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error)
	latestFn func(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error)
}

func (s conversationSummaryRepoServiceStub) Create(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	return s.createFn(ctx, summary)
}

func (s conversationSummaryRepoServiceStub) FindLatestByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error) {
	if s.latestFn != nil {
		return s.latestFn(ctx, conversationID, userID)
	}
	return domain.ConversationSummary{}, nil
}

func (s conversationSummaryRepoServiceStub) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}
