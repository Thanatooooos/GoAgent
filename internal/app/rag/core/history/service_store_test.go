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

	message := convention.SystemMessage("history already discussed permissions")
	decorated := adapter.DecorateIfNeeded(&message)
	if decorated == nil || !strings.HasPrefix(decorated.Content, "\u5bf9\u8bdd\u6458\u8981\uff1a") {
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
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "绗笁鏉?"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "绗簩鏉?"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "绗竴鏉?"},
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
	if history[0].Content != "绗竴鏉?" || history[2].Content != "绗笁鏉?" {
		t.Fatalf("unexpected history order: %#v", history)
	}
}

func TestMessageServiceStoreAppend(t *testing.T) {
	store := NewMessageServiceStore(
		conversationMessageConversationRepoStubForMemory{},
		conversationMessageRepoStubForMemory{},
	)

	messageID, err := store.Append(context.Background(), "c1", "u1", convention.UserMessage("浣犲ソ"))
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if messageID == "" {
		t.Fatal("expected message id")
	}
}

func TestCompressIfNeededBelowThreshold(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{},
		ChatService: &mockChatServiceForCompress{},
		StartTurns:  10,
		MaxChars:    200,
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.created {
		t.Fatal("expected no summary created below threshold")
	}
}

func TestCompressIfNeededNoChatService(t *testing.T) {
	svc := NewSummaryServiceAdapter(nil)
	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadLatestSummaryStillUsesRenderedContent(t *testing.T) {
	repo := &mockSummaryRepoForCompress{
		latestSummary: domain.ConversationSummary{
			ID:                    "s1",
			Content:               "鐩爣锛氬疄鐜扮粨鏋勫寲鎽樿",
			StructuredSummaryJSON: `{"schema_version":1,"goal":"涓嶈鐩存帴璇诲彇 JSON"}`,
			QualityStatus:         domain.SummaryQualityAccepted,
		},
	}

	adapter := NewSummaryServiceAdapter(repo)
	msg, err := adapter.LoadLatestSummary(context.Background(), "c1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil || msg.Content != "鐩爣锛氬疄鐜扮粨鏋勫寲鎽樿" {
		t.Fatalf("expected rendered content path to remain unchanged, got %#v", msg)
	}
}

func TestCompressIfNeededTriggersCompression(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"??? Go ???????","established_facts":["?????? Go ??????????"],"recent_progress":["??????????????"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "???4"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "???3"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "???2"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "???1"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
		Budget:      defaultSummaryBudgetOptions(),
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created when threshold met")
	}
	if !strings.Contains(summaryRepo.lastContent, "\u76ee\u6807\uff1a") {
		t.Fatalf("expected rendered content to contain goal section, got %q", summaryRepo.lastContent)
	}
	if !strings.Contains(summaryRepo.lastContent, "\u5df2\u786e\u8ba4\u4e8b\u5b9e\uff1a") {
		t.Fatalf("expected rendered content to contain confirmed facts section, got %q", summaryRepo.lastContent)
	}
	if !strings.Contains(summaryRepo.lastContent, "\u6700\u8fd1\u8fdb\u5c55\uff1a") {
		t.Fatalf("expected rendered content to contain recent progress section, got %q", summaryRepo.lastContent)
	}
	if chatSvc.lastRequest.JSONMode == nil || !*chatSvc.lastRequest.JSONMode {
		t.Fatalf("expected JSON mode request, got %+v", chatSvc.lastRequest)
	}
}

func TestCompressIfNeededStoresStructuredSummaryAndAcceptedQuality(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"鎺掓煡瀵煎叆澶辫触","constraints":["淇濇寔鐜版湁璇婚摼璺吋瀹?"],"established_facts":["doc_fail_01 鍦?indexer 鑺傜偣澶辫触","閿欒鏄?vector store unavailable"],"recent_progress":["宸插喅瀹氱粨鏋勫寲鎽樿浣滀负鐪熸簮"],"open_questions":["鏄惁闇€瑕佹寜瀵嗗害鍒嗘。"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "indexer failed: vector store unavailable"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "doc_fail_01 涓轰粈涔堝け璐?"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "璁╂垜缁х画鎺掓煡"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "璇峰府鎴戞帓鏌ュ鍏ュけ璐?"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
		Budget:      defaultSummaryBudgetOptions(),
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.lastSummary.StructuredSummaryJSON == "" {
		t.Fatal("expected structured summary json to be stored")
	}
	if summaryRepo.lastSummary.QualityStatus != domain.SummaryQualityAccepted {
		t.Fatalf("expected accepted quality status, got %q", summaryRepo.lastSummary.QualityStatus)
	}
	if !strings.Contains(summaryRepo.lastContent, "\u76ee\u6807\uff1a") {
		t.Fatalf("expected rendered content to contain goal section, got %q", summaryRepo.lastContent)
	}
	if chatSvc.lastRequest.JSONMode == nil || !*chatSvc.lastRequest.JSONMode {
		t.Fatalf("expected JSON mode request, got %+v", chatSvc.lastRequest)
	}
}


func TestCompressIfNeededRepairsBeforeValidationAndStoresRepairedSummary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"排查导入失败","established_facts":["接口方案还没确认"],"recent_progress":["doc_fail_01 已确认 vector store unavailable"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
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
		Budget:      defaultSummaryBudgetOptions(),
	})

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
func TestCompressIfNeededSkipsRejectedStructuredSummary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"鎺掓煡瀵煎叆澶辫触","established_facts":["瀵煎叆澶辫触"],"recent_progress":["缁х画澶勭悊"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "4", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "indexer failed: vector store unavailable"},
				{ID: "3", ConversationID: "c1", UserID: "u1", Role: "user", Content: "doc_fail_01 涓轰粈涔堝け璐?"},
				{ID: "2", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "璁╂垜缁х画鎺掓煡"},
				{ID: "1", ConversationID: "c1", UserID: "u1", Role: "user", Content: "璇峰府鎴戞帓鏌ュ鍏ュけ璐?"},
			},
			userCount:      2,
			assistantCount: 2,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
		Budget:      defaultSummaryBudgetOptions(),
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryRepo.created {
		t.Fatal("expected rejected structured summary to be skipped")
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

func TestCompressIfNeededAtBoundary(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{}
	chatSvc := &mockChatServiceForCompress{
		response: `{"schema_version":1,"goal":"?????????","recent_progress":["????????????????"]}`,
	}

	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "8", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "???8"},
				{ID: "7", ConversationID: "c1", UserID: "u1", Role: "user", Content: "???7"},
				{ID: "6", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "???6"},
				{ID: "5", ConversationID: "c1", UserID: "u1", Role: "user", Content: "???5"},
			},
			userCount:      4,
			assistantCount: 4,
		},
		ChatService: chatSvc,
		StartTurns:  2,
		MaxChars:    200,
		Budget:      defaultSummaryBudgetOptions(),
	})

	err := svc.CompressIfNeeded(context.Background(), "c1", "u1", convention.UserMessage("new msg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !summaryRepo.created {
		t.Fatal("expected summary to be created at boundary")
	}
	if !strings.Contains(summaryRepo.lastContent, "\u76ee\u6807\uff1a") {
		t.Fatalf("unexpected summary content: %q", summaryRepo.lastContent)
	}
}

func TestCompressIfNeededAlreadyCompressed(t *testing.T) {
	summaryRepo := &mockSummaryRepoForCompress{
		latestSummary: domain.ConversationSummary{
			ID:            "s1",
			LastMessageID: "8",
		},
	}
	svc := NewCompressibleSummaryService(summaryRepo, SummaryCompressionOptions{
		MessageRepo: &conversationMessageRepoStubForMemory{
			messages: []domain.ConversationMessage{
				{ID: "8", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "鍥炲8"},
				{ID: "7", ConversationID: "c1", UserID: "u1", Role: "user", Content: "闂7"},
				{ID: "6", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "鍥炲6"},
				{ID: "5", ConversationID: "c1", UserID: "u1", Role: "user", Content: "闂5"},
			},
			userCount:      4,
			assistantCount: 4,
		},
		ChatService: &mockChatServiceForCompress{response: `{"schema_version":1,"goal":"should not be called","recent_progress":["x"]}`},
		StartTurns:  2,
		MaxChars:    200,
		Budget:      defaultSummaryBudgetOptions(),
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

type mockChatServiceForCompress struct {
	response    string
	err         error
	lastRequest convention.ChatRequest
}

func (m *mockChatServiceForCompress) Chat(prompt string) (string, error) {
	return m.response, m.err
}

func (m *mockChatServiceForCompress) ChatWithRequest(request convention.ChatRequest) (string, error) {
	m.lastRequest = request
	return m.response, m.err
}

func (m *mockChatServiceForCompress) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	m.lastRequest = request
	return m.response, m.err
}

func (m *mockChatServiceForCompress) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (m *mockChatServiceForCompress) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	m.lastRequest = request
	return nil, nil
}

var _ aichat.LLMService = (*mockChatServiceForCompress)(nil)

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

var _ SummaryService = (*SummaryServiceAdapter)(nil)

func defaultSummaryBudgetOptions() SummaryBudgetOptions {
	return SummaryBudgetOptions{
		SmallMaxChars:         400,
		MediumMaxChars:        600,
		LargeMaxChars:         800,
		MediumMessageCountMin: 6,
		LargeMessageCountMin:  10,
	}
}







