package history

import (
	"context"
	"strings"
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

// compressionTestOptions returns options that trigger compression for short test messages.
func compressionTestOptions(base SummaryCompressionOptions) SummaryCompressionOptions {
	if base.TriggerTokens <= 0 {
		base.TriggerTokens = 1
	}
	if base.Estimator == nil {
		base.Estimator = NewTokenEstimateAdapter()
	}
	return base
}

// TestCompressIfNeededBelowThreshold 验证消息数不足时不触发压缩。
func TestCompressIfNeededBelowThreshold(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	svc := NewCompressibleSummaryService(summaryRepo, compressionTestOptions(SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{},
		ChatService: &mockChatServiceForCompress{},
		StartTurns:  10,
		MaxChars:    200,
	}))

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

// TestCompressIfNeededTriggersCompression 验证超过阈值且无重复摘要时触发结构化压缩并持久化元数据。
func TestCompressIfNeededTriggersCompression(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"解答 Go 语言特性问题","established_facts":["用户询问了关于 Go 语言特性的问题"],"recent_progress":["助手给出了详细解答"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, compressionTestOptions(SummaryCompressionOptions{
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
		StartTurns:  2,
		MaxChars:    200,
	}))

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created when threshold met")
	}
	if !strings.Contains(summaryRepo.lastContent, "目标：解答 Go 语言特性问题") {
		t.Fatalf("expected rendered summary to include goal, got %q", summaryRepo.lastContent)
	}
	if !strings.Contains(summaryRepo.lastContent, "最近进展") || !strings.Contains(summaryRepo.lastContent, "助手给出了详细解答") {
		t.Fatalf("expected rendered summary to include recent progress, got %q", summaryRepo.lastContent)
	}
	if summaryRepo.lastSummary.StructuredSummaryJSON == "" {
		t.Fatal("expected structured summary json to be stored")
	}
	if summaryRepo.lastSummary.SummaryVersion != domain.SummaryVersionV1 {
		t.Fatalf("unexpected summary version: %d", summaryRepo.lastSummary.SummaryVersion)
	}
	if summaryRepo.lastSummary.CoveredToMessageID != "4" || summaryRepo.lastSummary.CoveredFromMessageID != "1" {
		t.Fatalf("unexpected covered message range: from=%q to=%q", summaryRepo.lastSummary.CoveredFromMessageID, summaryRepo.lastSummary.CoveredToMessageID)
	}
	if summaryRepo.lastSummary.SourceMessageCount != 4 {
		t.Fatalf("unexpected source message count: %d", summaryRepo.lastSummary.SourceMessageCount)
	}
	if summaryRepo.lastSummary.QualityStatus != domain.SummaryQualityAccepted {
		t.Fatalf("unexpected quality status: %q", summaryRepo.lastSummary.QualityStatus)
	}
	if summaryRepo.lastSummary.LastRebuildReason != "threshold_reached" {
		t.Fatalf("unexpected rebuild reason: %q", summaryRepo.lastSummary.LastRebuildReason)
	}
}

func TestCompressIfNeededRepairsBeforeValidationAndStoresRepairedSummary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"排查导入失败","established_facts":["接口方案还没确认"],"recent_progress":["doc_fail_01 已确认 vector store unavailable"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, compressionTestOptions(SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "doc_fail_01 已确认 vector store unavailable"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "请确认接口方案"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "继续排查"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "帮我分析导入失败"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
	}))

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created after repair")
	}
	if summaryRepo.lastSummary.QualityStatus != domain.SummaryQualityAccepted {
		t.Fatalf("expected repaired summary to be accepted, got %q", summaryRepo.lastSummary.QualityStatus)
	}
	if !strings.Contains(summaryRepo.lastSummary.StructuredSummaryJSON, `"open_questions":["接口方案还没确认"]`) {
		t.Fatalf("expected repaired structured summary JSON to store open questions, got %q", summaryRepo.lastSummary.StructuredSummaryJSON)
	}
	if !strings.Contains(summaryRepo.lastContent, "待确认问题") {
		t.Fatalf("expected stored rendered content to include repaired open questions section, got %q", summaryRepo.lastContent)
	}
	if !strings.Contains(summaryRepo.lastContent, "接口方案还没确认") {
		t.Fatalf("expected stored rendered content to keep the unresolved item, got %q", summaryRepo.lastContent)
	}
}

func TestCompressIfNeededAsyncEnqueuesJob(t *testing.T) {
	done := make(chan struct{})
	runner := &mockSummaryCompressionRunner{done: done}
	svc := NewCompressibleSummaryService(&mockSummaryRepoForCompress{}, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{},
		ChatService: &mockChatServiceForCompress{},
		StartTurns:  10,
		MaxChars:    200,
	})
	worker := NewInMemorySummaryJobWorker(runner, 8)
	worker.Start()
	defer worker.Stop()
	svc.jobEnqueuer = worker
	svc.asyncEnabled = true

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected async compression runner to be invoked")
	}
}

type mockSummaryCompressionRunner struct {
	done chan struct{}
}

func (m *mockSummaryCompressionRunner) runConversationSummaryCompression(_ context.Context, input SummaryJobInput) error {
	if input.ConversationID != "c1" || input.UserID != "u1" {
		return nil
	}
	if m.done != nil {
		close(m.done)
	}
	return nil
}

// TestCompressIfNeededAtBoundary 验证消息数恰为阈值整数倍时触发压缩。
func TestCompressIfNeededAtBoundary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"整理最近对话","established_facts":["消息数达到压缩阈值"],"recent_progress":["已触发压缩"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, compressionTestOptions(SummaryCompressionOptions{
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
		StartTurns:  2,
		MaxChars:    200,
	}))

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created at boundary")
	}
	if !strings.Contains(summaryRepo.lastContent, "目标：整理最近对话") {
		t.Fatalf("expected rendered summary to include goal, got %q", summaryRepo.lastContent)
	}
	if !strings.Contains(summaryRepo.lastContent, "已触发压缩") {
		t.Fatalf("expected rendered summary to include recent progress, got %q", summaryRepo.lastContent)
	}
}

// TestCompressIfNeededAlreadyCompressed 验证最新消息已被覆盖时不重复压缩。
func TestCompressIfNeededAlreadyCompressed(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{
		latestSummary: domain.ConversationSummary{
			ID:            "s1",
			LastMessageID: "8",
		},
	}
	svc := NewCompressibleSummaryService(summaryRepo, compressionTestOptions(SummaryCompressionOptions{
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
	}))

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
	items := make([]domain.ConversationMessage, 0, len(s.messages))
	for _, message := range s.messages {
		if filter.AfterID != "" && message.ID <= filter.AfterID {
			continue
		}
		if filter.ThroughID != "" && message.ID > filter.ThroughID {
			continue
		}
		items = append(items, message)
	}
	if filter.Order == port.ConversationMessageOrderAsc {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	if filter.Limit > 0 && filter.Limit < len(items) {
		return items[:filter.Limit], nil
	}
	return items, nil
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
	lastSummary   domain.ConversationSummary
	latestSummary domain.ConversationSummary
}

func (m *mockSummaryRepoForCompress) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	m.created = true
	m.lastContent = summary.Content
	m.lastSummary = summary
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
