package memory

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
)

func TestSummaryServiceAdapterDecorateIfNeeded(t *testing.T) {
	adapter := NewSummaryServiceAdapter(nil)

	message := convention.SystemMessage("历史已经讨论了权限问题")
	decorated := adapter.DecorateIfNeeded(&message)
	if decorated == nil || decorated.Content != "对话摘要：历史已经讨论了权限问题" {
		t.Fatalf("unexpected decorated summary: %#v", decorated)
	}
}

func TestMessageServiceStoreLoadHistory(t *testing.T) {
	store := NewMessageServiceStore(
		conversationMessageConversationRepoStubForMemory{
			conversation: domain.Conversation{ID: "1", ConversationID: "c1", UserID: "u1"},
		},
		conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "第三条"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "第二条"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "第一条"},
			},
		},
	)

	history, err := store.LoadHistory(context.Background(), "c1", "u1", 3)
	if err != nil {
		t.Fatalf("LoadHistory returned error: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("expected 3 history items, got %d", len(history))
	}
	if history[0].Content != "第一条" || history[2].Content != "第三条" {
		t.Fatalf("unexpected history order: %#v", history)
	}
}

func TestMessageServiceStoreAppend(t *testing.T) {
	store := NewMessageServiceStore(
		conversationMessageConversationRepoStubForMemory{},
		conversationMessageRepoStubForMemory{},
	)

	messageID, err := store.Append(context.Background(), "c1", "u1", convention.UserMessage("你好"))
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected message id")
	}
}

type conversationMessageConversationRepoStubForMemory struct {
	conversation domain.Conversation
}

func (s conversationMessageConversationRepoStubForMemory) Create(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStubForMemory) Update(context.Context, domain.Conversation) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStubForMemory) UpdateWhere(context.Context, port.ConversationConditions, port.ConversationPatch) (int64, error) {
	return 0, nil
}

func (s conversationMessageConversationRepoStubForMemory) Delete(context.Context, string) error {
	return nil
}

func (s conversationMessageConversationRepoStubForMemory) GetByID(context.Context, string) (domain.Conversation, error) {
	return domain.Conversation{}, nil
}

func (s conversationMessageConversationRepoStubForMemory) GetByConversationIDAndUserID(context.Context, string, string) (domain.Conversation, error) {
	return s.conversation, nil
}

func (s conversationMessageConversationRepoStubForMemory) ListByUserID(context.Context, string) ([]domain.Conversation, error) {
	return nil, nil
}

type conversationMessageRepoStubForMemory struct {
	messages []domain.ConversationMessage
}

func (s conversationMessageRepoStubForMemory) Create(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
	if message.ID == "" {
		message.ID = "m1"
	}
	if message.CreateTime.IsZero() {
		message.CreateTime = time.Now()
	}
	return message, nil
}

func (s conversationMessageRepoStubForMemory) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (s conversationMessageRepoStubForMemory) List(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	return s.messages, nil
}

func (s conversationMessageRepoStubForMemory) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (s conversationMessageRepoStubForMemory) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (s conversationMessageRepoStubForMemory) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}
