package chat

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/cachemetrics"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/app/rag/service/longtermmemory/extraction"
	ltmwriteback "local/rag-project/internal/app/rag/service/longtermmemory/writeback"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type e2ePreferenceExtractionLLMStub struct {
	mu        sync.Mutex
	responses map[string]string
	requests  []convention.ChatRequest
}

func (s *e2ePreferenceExtractionLLMStub) Chat(string) (string, error) {
	return "", nil
}

func (s *e2ePreferenceExtractionLLMStub) ChatWithRequest(request convention.ChatRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, request)
	if len(request.Messages) == 0 {
		return "", nil
	}
	message := strings.TrimSpace(request.Messages[len(request.Messages)-1].Content)
	return s.responses[message], nil
}

func (s *e2ePreferenceExtractionLLMStub) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	return s.ChatWithRequest(request)
}

func (s *e2ePreferenceExtractionLLMStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *e2ePreferenceExtractionLLMStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *e2ePreferenceExtractionLLMStub) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

type signalingLongTermMemoryWriteback struct {
	delegate *ltmwriteback.Service
	done     chan struct{}
	once     sync.Once
}

func (s *signalingLongTermMemoryWriteback) CapturePreferenceCandidate(ctx context.Context, input LongTermMemoryWritebackInput) {
	if s.delegate != nil {
		s.delegate.CapturePreferenceCandidate(ctx, ltmwriteback.Input{
			UserID:          input.UserID,
			Message:         input.Message,
			SourceMessageID: input.SourceMessageID,
		})
	}
	if s.done != nil {
		s.once.Do(func() {
			close(s.done)
		})
	}
}

type inMemoryLongTermMemoryRepo struct {
	mu    sync.Mutex
	items map[string]domain.MemoryItem
	order []string
}

func newInMemoryLongTermMemoryRepo() *inMemoryLongTermMemoryRepo {
	return &inMemoryLongTermMemoryRepo{
		items: map[string]domain.MemoryItem{},
	}
}

func (r *inMemoryLongTermMemoryRepo) Create(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[item.ID] = item
	r.order = append(r.order, item.ID)
	return item, nil
}

func (r *inMemoryLongTermMemoryRepo) Update(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[item.ID]; !ok {
		r.order = append(r.order, item.ID)
	}
	r.items[item.ID] = item
	return item, nil
}

func (r *inMemoryLongTermMemoryRepo) GetByID(_ context.Context, id string) (domain.MemoryItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.items[strings.TrimSpace(id)], nil
}

func (r *inMemoryLongTermMemoryRepo) List(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]domain.MemoryItem, 0, len(r.items))
	for _, id := range r.order {
		item := r.items[id]
		if !matchesMemoryItemFilter(item, filter) {
			continue
		}
		items = append(items, item)
	}

	offset := filter.Offset
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]

	if filter.Limit > 0 && filter.Limit < len(items) {
		items = items[:filter.Limit]
	}
	return append([]domain.MemoryItem(nil), items...), nil
}

func (r *inMemoryLongTermMemoryRepo) Count(ctx context.Context, filter port.MemoryItemListFilter) (int64, error) {
	filter.Offset = 0
	filter.Limit = 0
	items, err := r.List(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}

func (r *inMemoryLongTermMemoryRepo) ListActiveByCanonicalKey(_ context.Context, userID string, scopeType string, scopeID string, canonicalKey string) ([]domain.MemoryItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var items []domain.MemoryItem
	for _, id := range r.order {
		item := r.items[id]
		if strings.TrimSpace(item.UserID) != strings.TrimSpace(userID) {
			continue
		}
		if strings.TrimSpace(item.ScopeType) != strings.TrimSpace(scopeType) {
			continue
		}
		if strings.TrimSpace(item.ScopeID) != strings.TrimSpace(scopeID) {
			continue
		}
		if strings.TrimSpace(item.CanonicalKey) != strings.TrimSpace(canonicalKey) {
			continue
		}
		if strings.TrimSpace(item.Status) != domain.MemoryStatusActive {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *inMemoryLongTermMemoryRepo) ListActiveSingleValueConflicts(context.Context, []string) ([]port.ActiveMemoryConflict, error) {
	return nil, nil
}

func (r *inMemoryLongTermMemoryRepo) TouchLastUsed(context.Context, string, []string, time.Time) error {
	return nil
}

func (r *inMemoryLongTermMemoryRepo) ExpireByIDs(context.Context, []string, string, time.Time) (int64, error) {
	return 0, nil
}

func (r *inMemoryLongTermMemoryRepo) DeleteByStatusesUpdatedBefore(context.Context, []string, time.Time, int) (int64, error) {
	return 0, nil
}

func matchesMemoryItemFilter(item domain.MemoryItem, filter port.MemoryItemListFilter) bool {
	if value := strings.TrimSpace(filter.UserID); value != "" && strings.TrimSpace(item.UserID) != value {
		return false
	}
	if len(filter.ScopeTypes) > 0 && !containsTrimmed(filter.ScopeTypes, item.ScopeType) {
		return false
	}
	if len(filter.ScopeIDs) > 0 && !containsTrimmed(filter.ScopeIDs, item.ScopeID) {
		return false
	}
	if len(filter.Namespaces) > 0 && !containsTrimmed(filter.Namespaces, item.Namespace) {
		return false
	}
	if len(filter.MemoryTypes) > 0 && !containsTrimmed(filter.MemoryTypes, item.MemoryType) {
		return false
	}
	if len(filter.Categories) > 0 && !containsTrimmed(filter.Categories, item.Category) {
		return false
	}
	if len(filter.CanonicalKeys) > 0 && !containsTrimmed(filter.CanonicalKeys, item.CanonicalKey) {
		return false
	}
	if len(filter.Statuses) > 0 && !containsTrimmed(filter.Statuses, item.Status) {
		return false
	}
	if value := strings.TrimSpace(filter.SourceMessageID); value != "" && strings.TrimSpace(item.SourceMessageID) != value {
		return false
	}
	if value := strings.TrimSpace(filter.SupersedesID); value != "" && strings.TrimSpace(item.SupersedesID) != value {
		return false
	}
	if value := strings.TrimSpace(filter.SearchText); value != "" {
		content := strings.ToLower(strings.TrimSpace(item.Content) + "\n" + strings.TrimSpace(item.Summary))
		if !strings.Contains(content, strings.ToLower(value)) {
			return false
		}
	}
	return true
}

func containsTrimmed(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func TestLongTermMemoryLifecycleE2EPersistsConfirmsAndRecallsPreference(t *testing.T) {
	repo := newInMemoryLongTermMemoryRepo()
	memory := longtermmemory.NewMemoryService(repo, longtermmemory.MemoryServiceOptions{
		MaxRecallItems: 5,
		MaxRecallChars: 800,
	})
	metrics := cachemetrics.NewService()
	memory.SetCacheMetrics(metrics)

	lifecycle := longtermmemory.NewPreferenceCandidateLifecycleService(memory)
	contract := longtermmemory.NewPreferenceCandidateContractService(lifecycle)
	extractorLLM := &e2ePreferenceExtractionLLMStub{
		responses: map[string]string{
			"Please answer in Chinese by default from now on.": `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"Answer in Chinese by default.","content":"Use Chinese by default.","confidence":0.94}`,
		},
	}
	writeback := &signalingLongTermMemoryWriteback{
		delegate: ltmwriteback.NewService(
			extraction.NewObservedLLMPreferenceExtractor(extractorLLM, metrics),
			lifecycle,
			metrics,
		),
		done: make(chan struct{}),
	}

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "Please answer in Chinese by default from now on.", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = memory.RecallService()
			opts.LongTermMemoryWriteback = writeback
		},
	)

	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "Please answer in Chinese by default from now on."},
		ragChatRuntimeState{
			meta:          RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"},
			title:         "title",
			userMessageID: "msg-user-1",
			traceID:       "trace-1",
		},
		ragChatTaskResult{content: "answer"},
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("handleSucceededResult returned error: %v", err)
	}
	waitForWritebackDone(t, writeback.done)

	pending, err := contract.ListPendingPreferenceCandidates(context.Background(), longtermmemory.ListPreferenceCandidatesInput{
		UserID: "user-1",
	})
	if err != nil {
		t.Fatalf("ListPendingPreferenceCandidates returned error: %v", err)
	}
	if len(pending.Items) != 1 {
		t.Fatalf("expected one pending candidate, got %+v", pending)
	}
	if pending.Items[0].CanonicalKey != "response.language" || pending.Items[0].Status != domain.MemoryStatusPending {
		t.Fatalf("unexpected pending candidate: %+v", pending.Items[0])
	}

	confirmed, err := contract.ConfirmPreferenceCandidate(context.Background(), longtermmemory.DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: pending.Items[0].ID,
	})
	if err != nil {
		t.Fatalf("ConfirmPreferenceCandidate returned error: %v", err)
	}
	if confirmed.Status != domain.MemoryStatusActive {
		t.Fatalf("expected active confirmed candidate, got %+v", confirmed)
	}

	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "How should you answer next time?",
	})
	if err != nil {
		t.Fatalf("prepareChat returned error: %v", err)
	}
	if !strings.Contains(prepared.memoryContext, "Answer in Chinese by default.") {
		t.Fatalf("expected recalled active preference in memory context, got %q", prepared.memoryContext)
	}
}

func TestLongTermMemoryLifecycleE2ERejectedCandidateDoesNotRecall(t *testing.T) {
	repo := newInMemoryLongTermMemoryRepo()
	memory := longtermmemory.NewMemoryService(repo, longtermmemory.MemoryServiceOptions{
		MaxRecallItems: 5,
		MaxRecallChars: 800,
	})
	metrics := cachemetrics.NewService()
	memory.SetCacheMetrics(metrics)

	lifecycle := longtermmemory.NewPreferenceCandidateLifecycleService(memory)
	contract := longtermmemory.NewPreferenceCandidateContractService(lifecycle)
	extractorLLM := &e2ePreferenceExtractionLLMStub{
		responses: map[string]string{
			"Please answer in Chinese by default from now on.": `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"Answer in Chinese by default.","content":"Use Chinese by default.","confidence":0.94}`,
		},
	}
	writeback := &signalingLongTermMemoryWriteback{
		delegate: ltmwriteback.NewService(
			extraction.NewObservedLLMPreferenceExtractor(extractorLLM, metrics),
			lifecycle,
			metrics,
		),
		done: make(chan struct{}),
	}

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "Please answer in Chinese by default from now on.", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = memory.RecallService()
			opts.LongTermMemoryWriteback = writeback
		},
	)

	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "Please answer in Chinese by default from now on."},
		ragChatRuntimeState{
			meta:          RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"},
			title:         "title",
			userMessageID: "msg-user-1",
			traceID:       "trace-1",
		},
		ragChatTaskResult{content: "answer"},
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("handleSucceededResult returned error: %v", err)
	}
	waitForWritebackDone(t, writeback.done)

	pending, err := contract.ListPendingPreferenceCandidates(context.Background(), longtermmemory.ListPreferenceCandidatesInput{
		UserID: "user-1",
	})
	if err != nil {
		t.Fatalf("ListPendingPreferenceCandidates returned error: %v", err)
	}
	if len(pending.Items) != 1 {
		t.Fatalf("expected one pending candidate, got %+v", pending)
	}

	rejected, err := contract.RejectPreferenceCandidate(context.Background(), longtermmemory.DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: pending.Items[0].ID,
	})
	if err != nil {
		t.Fatalf("RejectPreferenceCandidate returned error: %v", err)
	}
	if rejected.Status != domain.MemoryStatusRejected {
		t.Fatalf("expected rejected candidate, got %+v", rejected)
	}

	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "How should you answer next time?",
	})
	if err != nil {
		t.Fatalf("prepareChat returned error: %v", err)
	}
	if strings.TrimSpace(prepared.memoryContext) != "" {
		t.Fatalf("expected rejected preference not to be recalled, got %q", prepared.memoryContext)
	}
}

func TestLongTermMemoryLifecycleE2ESkipsOneOffInputWithoutTrigger(t *testing.T) {
	repo := newInMemoryLongTermMemoryRepo()
	memory := longtermmemory.NewMemoryService(repo, longtermmemory.MemoryServiceOptions{})
	metrics := cachemetrics.NewService()
	memory.SetCacheMetrics(metrics)

	lifecycle := longtermmemory.NewPreferenceCandidateLifecycleService(memory)
	contract := longtermmemory.NewPreferenceCandidateContractService(lifecycle)
	extractorLLM := &e2ePreferenceExtractionLLMStub{
		responses: map[string]string{},
	}
	writeback := &signalingLongTermMemoryWriteback{
		delegate: ltmwriteback.NewService(
			extraction.NewObservedLLMPreferenceExtractor(extractorLLM, metrics),
			lifecycle,
			metrics,
		),
		done: make(chan struct{}),
	}

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "17*23", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = memory.RecallService()
			opts.LongTermMemoryWriteback = writeback
		},
	)

	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "17*23"},
		ragChatRuntimeState{
			meta:          RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"},
			title:         "title",
			userMessageID: "msg-user-1",
			traceID:       "trace-1",
		},
		ragChatTaskResult{content: "answer"},
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("handleSucceededResult returned error: %v", err)
	}
	waitForWritebackDone(t, writeback.done)

	pending, err := contract.ListPendingPreferenceCandidates(context.Background(), longtermmemory.ListPreferenceCandidatesInput{
		UserID: "user-1",
	})
	if err != nil {
		t.Fatalf("ListPendingPreferenceCandidates returned error: %v", err)
	}
	if len(pending.Items) != 0 {
		t.Fatalf("expected one-off input to skip long-term memory, got %+v", pending)
	}
	if extractorLLM.requestCount() != 0 {
		t.Fatalf("expected pre-filter skip before llm extraction, got %d requests", extractorLLM.requestCount())
	}
}

func TestLongTermMemoryLifecycleE2ETriggeredOneOffInputStillGeneratesPendingCandidate(t *testing.T) {
	repo := newInMemoryLongTermMemoryRepo()
	memory := longtermmemory.NewMemoryService(repo, longtermmemory.MemoryServiceOptions{})
	metrics := cachemetrics.NewService()
	memory.SetCacheMetrics(metrics)

	lifecycle := longtermmemory.NewPreferenceCandidateLifecycleService(memory)
	contract := longtermmemory.NewPreferenceCandidateContractService(lifecycle)
	message := "以后做算法题也默认用中文回答"
	extractorLLM := &e2ePreferenceExtractionLLMStub{
		responses: map[string]string{
			message: `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"Answer in Chinese by default.","content":"Use Chinese by default.","confidence":0.94}`,
		},
	}
	writeback := &signalingLongTermMemoryWriteback{
		delegate: ltmwriteback.NewService(
			extraction.NewObservedLLMPreferenceExtractor(extractorLLM, metrics),
			lifecycle,
			metrics,
		),
		done: make(chan struct{}),
	}

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: message, NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = memory.RecallService()
			opts.LongTermMemoryWriteback = writeback
		},
	)

	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: message},
		ragChatRuntimeState{
			meta:          RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"},
			title:         "title",
			userMessageID: "msg-user-1",
			traceID:       "trace-1",
		},
		ragChatTaskResult{content: "answer"},
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("handleSucceededResult returned error: %v", err)
	}
	waitForWritebackDone(t, writeback.done)

	pending, err := contract.ListPendingPreferenceCandidates(context.Background(), longtermmemory.ListPreferenceCandidatesInput{
		UserID: "user-1",
	})
	if err != nil {
		t.Fatalf("ListPendingPreferenceCandidates returned error: %v", err)
	}
	if len(pending.Items) != 1 {
		t.Fatalf("expected triggered one-off input to produce pending candidate, got %+v", pending)
	}
	if extractorLLM.requestCount() != 1 {
		t.Fatalf("expected one llm extraction attempt, got %d", extractorLLM.requestCount())
	}
}

func waitForWritebackDone(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for writeback to finish")
	}
}
