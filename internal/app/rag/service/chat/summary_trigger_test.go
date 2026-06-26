package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	ragconversation "local/rag-project/internal/app/rag/service/conversation"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestPersistAssistantMessageEnqueuesSummaryAfterPersistence(t *testing.T) {
	trigger := &summaryTriggerRecorder{}
	service := chatServiceForSummaryTriggerTest(trigger)

	payload, err := service.persistAssistantMessage(
		context.Background(),
		ragChatRuntimeState{meta: RagChatMeta{ConversationID: "c1"}},
		RagChatInput{UserID: "u1"},
		"assistant answer",
		"",
	)
	if err != nil {
		t.Fatalf("persistAssistantMessage() error = %v", err)
	}
	if trigger.input.ConversationID != "c1" || trigger.input.UserID != "u1" {
		t.Fatalf("summary trigger input = %+v", trigger.input)
	}
	if trigger.input.TargetMessageID != payload.MessageID || payload.MessageID != "assistant-1" {
		t.Fatalf("target message = %q, payload message = %q", trigger.input.TargetMessageID, payload.MessageID)
	}
}

func TestPersistAssistantMessageFailsOpenWhenSummaryEnqueueFails(t *testing.T) {
	trigger := &summaryTriggerRecorder{err: errors.New("queue full")}
	service := chatServiceForSummaryTriggerTest(trigger)

	payload, err := service.persistAssistantMessage(
		context.Background(),
		ragChatRuntimeState{meta: RagChatMeta{ConversationID: "c1"}},
		RagChatInput{UserID: "u1"},
		"assistant answer",
		"",
	)
	if err != nil {
		t.Fatalf("persistAssistantMessage() error = %v, want fail-open", err)
	}
	if payload.MessageID != "assistant-1" {
		t.Fatalf("payload message id = %q", payload.MessageID)
	}
}

func TestPersistAssistantMessageDrivesAsyncSummaryToAssistantBoundary(t *testing.T) {
	var messageMu sync.Mutex
	messages := make([]domain.ConversationMessage, 0, 1)
	messageRepo := conversationMessageRepoServiceStub{
		createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
			messageMu.Lock()
			defer messageMu.Unlock()
			messages = append(messages, message)
			return message, nil
		},
		listFn: func(_ context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
			messageMu.Lock()
			defer messageMu.Unlock()
			result := make([]domain.ConversationMessage, 0, len(messages))
			for _, message := range messages {
				if filter.AfterID != "" && message.ID <= filter.AfterID {
					continue
				}
				if filter.ThroughID != "" && message.ID > filter.ThroughID {
					continue
				}
				result = append(result, message)
			}
			return result, nil
		},
	}
	summaryRepo := &asyncSummaryRepo{}
	compressible := raghistory.NewCompressibleSummaryService(summaryRepo, raghistory.SummaryCompressionOptions{
		MessageRepo:           messageRepo,
		ChatService:           asyncSummaryChatStub{},
		TriggerTokens:         1,
		Estimator:             tokenbudget.FixedEstimator(10),
		SafetyFactor:          1,
		MessageOverheadTokens: 4,
		MaxChars:              200,
	})
	worker := compressible.EnableAsyncSummaryJobs(8)
	defer worker.Stop()

	messageService := ragconversation.NewMessageService(nil, messageRepo, summaryRepo, nil)
	service := &RagChatService{
		conversationService: &ConversationService{},
		messageService:      messageService,
		tracer:              NewChatTracer(nil, nil),
		summaryTrigger:      compressible,
	}
	payload, err := service.persistAssistantMessage(
		context.Background(),
		ragChatRuntimeState{meta: RagChatMeta{ConversationID: "c1"}},
		RagChatInput{UserID: "u1"},
		"assistant answered",
		"",
	)
	if err != nil {
		t.Fatalf("persistAssistantMessage() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latest, count := summaryRepo.snapshot()
		if count > 0 {
			if latest.CoveredToMessageID != payload.MessageID {
				t.Fatalf("CoveredToMessageID = %q, want %q", latest.CoveredToMessageID, payload.MessageID)
			}
			if err := compressible.EnqueueSummaryCheck(context.Background(), raghistory.SummaryJobInput{
				ConversationID:  "c1",
				UserID:          "u1",
				TargetMessageID: payload.MessageID,
			}); err != nil {
				t.Fatalf("duplicate enqueue error = %v", err)
			}
			time.Sleep(50 * time.Millisecond)
			_, afterDuplicate := summaryRepo.snapshot()
			if afterDuplicate != 1 {
				t.Fatalf("summary count after duplicate = %d, want 1", afterDuplicate)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for asynchronous summary")
}

func chatServiceForSummaryTriggerTest(trigger raghistory.SummaryTrigger) *RagChatService {
	messageService := ragconversation.NewMessageService(
		nil,
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				message.ID = "assistant-1"
				return message, nil
			},
		},
		nil,
		nil,
	)
	return &RagChatService{
		conversationService: &ConversationService{},
		messageService:      messageService,
		tracer:              NewChatTracer(nil, nil),
		summaryTrigger:      trigger,
	}
}

type summaryTriggerRecorder struct {
	input raghistory.SummaryJobInput
	err   error
}

func (r *summaryTriggerRecorder) EnqueueSummaryCheck(_ context.Context, input raghistory.SummaryJobInput) error {
	r.input = input
	return r.err
}

type asyncSummaryRepo struct {
	mu      sync.Mutex
	latest  domain.ConversationSummary
	created int
}

func (r *asyncSummaryRepo) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latest = summary
	r.created++
	return summary, nil
}

func (r *asyncSummaryRepo) CreateIfCoverageAdvances(_ context.Context, summary domain.ConversationSummary) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latest.CoveredToMessageID != "" && r.latest.CoveredToMessageID >= summary.CoveredToMessageID {
		return false, nil
	}
	r.latest = summary
	r.created++
	return true, nil
}

func (r *asyncSummaryRepo) FindLatestByConversationIDAndUserID(context.Context, string, string) (domain.ConversationSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest, nil
}

func (r *asyncSummaryRepo) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

func (r *asyncSummaryRepo) snapshot() (domain.ConversationSummary, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest, r.created
}

type asyncSummaryChatStub struct{}

func (asyncSummaryChatStub) Chat(string) (string, error) {
	return `{"schema_version":1,"goal":"answer the current question","recent_progress":["assistant answered"]}`, nil
}

func (s asyncSummaryChatStub) ChatWithRequest(convention.ChatRequest) (string, error) {
	return s.Chat("")
}

func (s asyncSummaryChatStub) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return s.Chat("")
}

func (asyncSummaryChatStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (asyncSummaryChatStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ port.ConversationSummaryRepository = (*asyncSummaryRepo)(nil)
var _ port.ConversationSummaryCoverageRepository = (*asyncSummaryRepo)(nil)
var _ aichat.LLMService = asyncSummaryChatStub{}
