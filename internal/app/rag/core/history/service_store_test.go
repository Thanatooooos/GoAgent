package history

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
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

// TestCompressIfNeededBelowThreshold 验证消息数不足时不触发压缩。
func TestCompressIfNeededBelowThreshold(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{},
		ChatService: &mockChatServiceForCompress{},
		StartTurns:  10, // 阈值很高
		MaxChars:    200,
	})

	// CountByConversationIDAndUserIDAndRole 返回 0，小于阈值 20。
	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// summaryRepo 不应被调用。
	if summaryRepo.created {
		t.Fatal("expected no summary created below threshold")
	}
}

// TestCompressIfNeededNoChatService 验证无 chatService 时跳过压缩。
func TestCompressIfNeededNoChatService(t *testing.T) {
	svc := NewSummaryServiceAdapter(nil)
	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCompressIfNeededTriggersCompression 验证超过阈值且无重复摘要时触发 LLM 压缩。
func TestCompressIfNeededTriggersCompression(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{response: "用户询问了关于 Go 语言特性的问题，助手给出了详细解答。"}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复4"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题3"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复2"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题1"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2, // 阈值为 4，totalMessages=4 正好达到阈值。
		MaxChars:    200,
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 没有已有摘要阻断 → 应触发 LLM 压缩并写入。
	if !summaryRepo.created {
		t.Fatal("expected summary to be created when threshold met")
	}
	if summaryRepo.lastContent != "用户询问了关于 Go 语言特性的问题，助手给出了详细解答。" {
		t.Fatalf("unexpected summary content: %q", summaryRepo.lastContent)
	}
}

// TestCompressIfNeededAtBoundary 验证消息数恰为阈值整数倍时触发压缩。
func TestCompressIfNeededAtBoundary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{response: "压缩后的历史摘要"}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "8", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复8"},
				{ID: "7", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题7"},
				{ID: "6", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复6"},
				{ID: "5", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题5"},
			},
			userCount:      4,
			assistantCount: 4,
		},
		ChatService: chatSvc,
		StartTurns:  2, // 阈值为 4，totalMessages=8, 8%4==0 触发。
		MaxChars:    200,
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created at boundary")
	}
	if summaryRepo.lastContent != "压缩后的历史摘要" {
		t.Fatalf("unexpected summary content: %q", summaryRepo.lastContent)
	}
}

// TestCompressIfNeededAlreadyCompressed 验证最新消息已被覆盖时不重复压缩。
func TestCompressIfNeededAlreadyCompressed(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{
		latestSummary: domain.ConversationSummary{
			ID:            "s1",
			LastMessageID: "8", // 最新消息 ID 已是 8，说明覆盖过。
		},
	}
	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "8", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复8"},
				{ID: "7", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题7"},
				{ID: "6", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "回复6"},
				{ID: "5", ConversationID: "c1", UserID: "u1", Role: "user", Content: "问题5"},
			},
			userCount:      4,
			assistantCount: 4,
		},
		ChatService: &mockChatServiceForCompress{response: "should not be called"},
		StartTurns:  2,
		MaxChars:    200,
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.created {
		t.Fatal("expected no duplicate compression")
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
	messages       []domain.ConversationMessage
	userCount      int64
	assistantCount int64
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

func (s conversationMessageRepoStubForMemory) List(_ context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	if filter.Limit > 0 && filter.Limit < len(s.messages) {
		return s.messages[:filter.Limit], nil
	}
	return s.messages, nil
}

func (s conversationMessageRepoStubForMemory) CountByConversationIDAndUserIDAndRole(_ context.Context, _ string, _ string, role string) (int64, error) {
	if role == "user" {
		return s.userCount, nil
	}
	return s.assistantCount, nil
}

func (s conversationMessageRepoStubForMemory) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (s conversationMessageRepoStubForMemory) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

// mockChatServiceForCompress 用于压缩测试的 LLM 服务桩。
type mockChatServiceForCompress struct {
	response string
	err      error
}

func (m *mockChatServiceForCompress) Chat(prompt string) (string, error) {
	return m.response, m.err
}

func (m *mockChatServiceForCompress) ChatWithRequest(request convention.ChatRequest) (string, error) {
	return m.response, m.err
}

func (m *mockChatServiceForCompress) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	return m.response, m.err
}

func (m *mockChatServiceForCompress) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (m *mockChatServiceForCompress) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*mockChatServiceForCompress)(nil)

// mockSummaryRepoForCompress 用于压缩测试的摘要仓储桩。
type mockSummaryRepoForCompress struct {
	created       bool
	lastContent   string
	latestSummary domain.ConversationSummary
}

func (m *mockSummaryRepoForCompress) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	m.created = true
	m.lastContent = summary.Content
	summary.ID = "summary-1"
	return summary, nil
}

func (m *mockSummaryRepoForCompress) FindLatestByConversationIDAndUserID(_ context.Context, _ string, _ string) (domain.ConversationSummary, error) {
	return m.latestSummary, nil
}

func (m *mockSummaryRepoForCompress) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

var _ port.ConversationSummaryRepository = (*mockSummaryRepoForCompress)(nil)

// Ensure SummaryServiceAdapter implements the SummaryService interface.
var _ SummaryService = (*SummaryServiceAdapter)(nil)
