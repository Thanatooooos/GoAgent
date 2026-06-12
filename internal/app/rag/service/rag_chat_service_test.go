package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type memoryServiceStub struct {
	history []convention.ChatMessage
}

func (s memoryServiceStub) Load(context.Context, string, string) ([]convention.ChatMessage, error) {
	return append([]convention.ChatMessage(nil), s.history...), nil
}

func (s memoryServiceStub) Append(context.Context, string, string, convention.ChatMessage) (string, error) {
	return "msg", nil
}

func (s memoryServiceStub) LoadAndAppend(context.Context, string, string, convention.ChatMessage) ([]convention.ChatMessage, error) {
	return append([]convention.ChatMessage(nil), s.history...), nil
}

type rewriteServiceStub struct {
	result ragrewrite.Result
}

func (s rewriteServiceStub) Rewrite(question string) string {
	if strings.TrimSpace(s.result.RewrittenQuestion) != "" {
		return s.result.RewrittenQuestion
	}
	return strings.TrimSpace(question)
}

func (s rewriteServiceStub) RewriteWithSplit(question string) ragrewrite.Result {
	return s.result
}

func (s rewriteServiceStub) RewriteWithHistory(question string, history []convention.ChatMessage) ragrewrite.Result {
	return s.result
}

type retrieveServiceStub struct {
	mu          sync.Mutex
	result      ragretrieve.Result
	err         error
	requests    []ragretrieve.Request
	retrieveFn  func(context.Context, ragretrieve.Request) (ragretrieve.Result, error)
	inFlight    int32
	maxInFlight int32
}

func (s *retrieveServiceStub) Retrieve(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	if s.retrieveFn != nil {
		current := atomic.AddInt32(&s.inFlight, 1)
		for {
			maxSeen := atomic.LoadInt32(&s.maxInFlight)
			if current <= maxSeen || atomic.CompareAndSwapInt32(&s.maxInFlight, maxSeen, current) {
				break
			}
		}
		defer atomic.AddInt32(&s.inFlight, -1)
		return s.retrieveFn(ctx, request)
	}
	return s.result, s.err
}

func (s *retrieveServiceStub) RetrieveByVector(ctx context.Context, vector []float32, request ragretrieve.Request) (ragretrieve.Result, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	return s.result, s.err
}

type retrieveSearcherStub struct {
	hits         []corevector.SearchHit
	keywordHits  []corevector.SearchHit
	metadataHits []corevector.SearchHit
}

func (s *retrieveSearcherStub) Search(context.Context, corevector.SearchRequest) ([]corevector.SearchHit, error) {
	return append([]corevector.SearchHit(nil), s.hits...), nil
}

func (s *retrieveSearcherStub) SearchByKeyword(context.Context, string, []string, int) ([]corevector.SearchHit, error) {
	return append([]corevector.SearchHit(nil), s.keywordHits...), nil
}

func (s *retrieveSearcherStub) SearchByMetadata(context.Context, string, []string, int) ([]corevector.SearchHit, error) {
	return append([]corevector.SearchHit(nil), s.metadataHits...), nil
}

type sessionRecallServiceStub struct {
	result SessionRecallResult
	err    error
	input  SessionRecallInput
	calls  int
}

func (s *sessionRecallServiceStub) Recall(ctx context.Context, input SessionRecallInput) (SessionRecallResult, error) {
	s.calls++
	s.input = input
	return s.result, s.err
}

type explicitMemoryRecallStub struct {
	result longtermmemory.RecallMemoriesResult
	err    error
	input  longtermmemory.RecallMemoriesInput
	calls  int
}

func (s *explicitMemoryRecallStub) RecallMemories(ctx context.Context, input longtermmemory.RecallMemoriesInput) (longtermmemory.RecallMemoriesResult, error) {
	s.calls++
	s.input = input
	return s.result, s.err
}

type inMemorySessionChunkStore struct {
	messages map[string]domain.ConversationMessage
	chunks   []domain.SessionChunk
}

func newInMemorySessionChunkStore() *inMemorySessionChunkStore {
	return &inMemorySessionChunkStore{
		messages: map[string]domain.ConversationMessage{},
	}
}

func (s *inMemorySessionChunkStore) PersistMessageChunks(ctx context.Context, message domain.ConversationMessage, chunks []ProcessedConversationMessageChunk) error {
	if s.messages == nil {
		s.messages = map[string]domain.ConversationMessage{}
	}
	s.messages[message.ID] = message
	for _, chunk := range chunks {
		s.chunks = append(s.chunks, domain.SessionChunk{
			ID:             message.ID + "-chunk-" + strconv.Itoa(chunk.ChunkIndex),
			ConversationID: message.ConversationID,
			MessageID:      message.ID,
			UserID:         message.UserID,
			ChunkIndex:     chunk.ChunkIndex,
			Content:        chunk.Content,
			ContentSummary: chunk.ContentSummary,
			TokenEstimate:  chunk.TokenEstimate,
			CreateTime:     message.CreateTime,
			UpdateTime:     message.UpdateTime,
		})
	}
	return nil
}

func (s *inMemorySessionChunkStore) CreateBatch(context.Context, []domain.SessionChunk) error {
	return nil
}

func (s *inMemorySessionChunkStore) ExistsRecallable(_ context.Context, conversationID string, userID string, excludeMessageID string) (bool, error) {
	for _, chunk := range s.chunks {
		message := s.messages[chunk.MessageID]
		if chunk.ConversationID != conversationID || chunk.UserID != userID {
			continue
		}
		if strings.TrimSpace(excludeMessageID) != "" && chunk.MessageID == strings.TrimSpace(excludeMessageID) {
			continue
		}
		if message.Role != string(convention.UserRole) || !message.IsSummarized {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (s *inMemorySessionChunkStore) GetRecallFingerprint(_ context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error) {
	fingerprint := domain.SessionRecallFingerprint{}
	for _, chunk := range s.chunks {
		message := s.messages[chunk.MessageID]
		if chunk.ConversationID != conversationID || chunk.UserID != userID {
			continue
		}
		if strings.TrimSpace(excludeMessageID) != "" && chunk.MessageID == strings.TrimSpace(excludeMessageID) {
			continue
		}
		if message.Role != string(convention.UserRole) || !message.IsSummarized {
			continue
		}
		fingerprint.Exists = true
		fingerprint.RecallableCount++
		if chunk.UpdateTime.After(fingerprint.LatestUpdateTime) || (chunk.UpdateTime.Equal(fingerprint.LatestUpdateTime) && chunk.ID > fingerprint.LatestChunkID) {
			fingerprint.LatestUpdateTime = chunk.UpdateTime
			fingerprint.LatestChunkID = chunk.ID
			fingerprint.LatestMessageID = chunk.MessageID
		}
	}
	return fingerprint, nil
}

func (s *inMemorySessionChunkStore) SearchRecallableByVector(_ context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error) {
	result := make([]domain.SessionChunkSearchHit, 0, topK)
	for idx, chunk := range s.chunks {
		message := s.messages[chunk.MessageID]
		if chunk.ConversationID != conversationID || chunk.UserID != userID {
			continue
		}
		if strings.TrimSpace(excludeMessageID) != "" && chunk.MessageID == strings.TrimSpace(excludeMessageID) {
			continue
		}
		if message.Role != string(convention.UserRole) || !message.IsSummarized {
			continue
		}
		result = append(result, domain.SessionChunkSearchHit{
			SessionChunk: chunk,
			Score:        float32(1.0 - float32(idx)*0.1),
		})
		if len(result) >= topK {
			break
		}
	}
	return result, nil
}

type toolWorkflowStub struct {
	result ragtool.WorkflowResult
	err    error
	input  ragtool.WorkflowInput
}

func (s *toolWorkflowStub) Run(ctx context.Context, input ragtool.WorkflowInput) (ragtool.WorkflowResult, error) {
	s.input = input
	return s.result, s.err
}

type streamHandleStub struct {
	cancelled bool
}

func (s *streamHandleStub) Cancel() {
	s.cancelled = true
}

type llmServiceStub struct {
	requests     []convention.ChatRequest
	streamHandle *streamHandleStub
	streamErr    error
	streamFn     func(request convention.ChatRequest, callback aichat.StreamCallback)
}

func (s *llmServiceStub) Chat(string) (string, error) {
	return "", nil
}

func (s *llmServiceStub) ChatWithRequest(convention.ChatRequest) (string, error) {
	return "", nil
}

func (s *llmServiceStub) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return "", nil
}

func (s *llmServiceStub) StreamChat(prompt string, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return s.StreamChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage(prompt)},
	}, callback)
}

func (s *llmServiceStub) StreamChatWithRequest(request convention.ChatRequest, callback aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	s.requests = append(s.requests, request)
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	if s.streamHandle == nil {
		s.streamHandle = &streamHandleStub{}
	}
	if s.streamFn != nil {
		s.streamFn(request, callback)
	} else {
		callback.OnContent("answer")
		callback.OnComplete()
	}
	return s.streamHandle, nil
}

type fallbackSinkStub struct {
	metaCalls       int
	agentThinkCalls int
	fallbackCalls   int
	fallbackReason  string
	agentOutcomes   []RagChatAgentOutcomePayload
	approvalPending []RagChatApprovalPendingPayload
	agentErrors     []RagChatAgentServiceErrorPayload
	memoryStored    []RagChatMemoryStoredPayload
	sessionRecalls  []RagChatSessionRecallPayload
	finishCalls     int
	cancelCalls     int
	errorCalls      int
	doneCalls       int
	toolCalls       int
	toolNames       []string
	toolStarts      []ragtool.ToolCallEvent
	toolResults     []ragtool.ToolCallEvent
}

func (s *fallbackSinkStub) SendMeta(meta RagChatMeta) error {
	s.metaCalls++
	return nil
}

func (s *fallbackSinkStub) SendFallback(reason string) error {
	s.fallbackCalls++
	s.fallbackReason = reason
	return nil
}

func (s *fallbackSinkStub) SendAgentThink(message string) error {
	s.agentThinkCalls++
	return nil
}

func (s *fallbackSinkStub) SendAgentOutcome(payload RagChatAgentOutcomePayload) error {
	s.agentOutcomes = append(s.agentOutcomes, payload)
	return nil
}

func (s *fallbackSinkStub) SendApprovalPending(payload RagChatApprovalPendingPayload) error {
	s.approvalPending = append(s.approvalPending, payload)
	return nil
}

func (s *fallbackSinkStub) SendAgentServiceError(payload RagChatAgentServiceErrorPayload) error {
	s.agentErrors = append(s.agentErrors, payload)
	return nil
}

func (s *fallbackSinkStub) SendMemoryStored(payload RagChatMemoryStoredPayload) error {
	s.memoryStored = append(s.memoryStored, payload)
	return nil
}

func (s *fallbackSinkStub) SendSessionRecall(payload RagChatSessionRecallPayload) error {
	s.sessionRecalls = append(s.sessionRecalls, payload)
	return nil
}

func (s *fallbackSinkStub) SendThinking(delta string) error { return nil }
func (s *fallbackSinkStub) SendMessage(delta string) error  { return nil }
func (s *fallbackSinkStub) SendToolStart(payload ragtool.ToolCallEvent) error {
	s.toolStarts = append(s.toolStarts, payload)
	return nil
}
func (s *fallbackSinkStub) SendToolResult(payload ragtool.ToolCallEvent) error {
	s.toolResults = append(s.toolResults, payload)
	return nil
}
func (s *fallbackSinkStub) SendTitle(title string) error { return nil }
func (s *fallbackSinkStub) SendTool(name string, status string, summary string) error {
	s.toolCalls++
	s.toolNames = append(s.toolNames, name)
	return nil
}

func (s *fallbackSinkStub) SendFinish(payload RagChatFinishPayload) error {
	s.finishCalls++
	return nil
}

func (s *fallbackSinkStub) SendCancel(payload RagChatFinishPayload) error {
	s.cancelCalls++
	return nil
}

func (s *fallbackSinkStub) SendError(err error) error {
	s.errorCalls++
	return nil
}

func (s *fallbackSinkStub) SendDone() error {
	s.doneCalls++
	return nil
}

type traceNodeRepoRecorder struct {
	created []domain.RagTraceNode
}

func (r *traceNodeRepoRecorder) Create(_ context.Context, node domain.RagTraceNode) (domain.RagTraceNode, error) {
	r.created = append(r.created, node)
	return node, nil
}

func (r *traceNodeRepoRecorder) UpdateByTraceIDAndNodeID(context.Context, string, string, domain.RagTraceNode) error {
	return nil
}

func (r *traceNodeRepoRecorder) UpdateWhere(context.Context, port.RagTraceNodeConditions, port.RagTraceNodePatch) (int64, error) {
	return 0, nil
}

func (r *traceNodeRepoRecorder) ListByTraceID(context.Context, string) ([]domain.RagTraceNode, error) {
	return nil, nil
}

type traceRunRepoRecorder struct {
	runs map[string]domain.RagTraceRun
}

func (r *traceRunRepoRecorder) Create(_ context.Context, run domain.RagTraceRun) (domain.RagTraceRun, error) {
	if r.runs == nil {
		r.runs = map[string]domain.RagTraceRun{}
	}
	r.runs[run.TraceID] = run
	return run, nil
}

func (r *traceRunRepoRecorder) UpdateByTraceID(_ context.Context, traceID string, run domain.RagTraceRun) error {
	if r.runs == nil {
		r.runs = map[string]domain.RagTraceRun{}
	}
	existing := r.runs[traceID]
	if run.ID == "" {
		run.ID = existing.ID
	}
	if run.TraceID == "" {
		run.TraceID = existing.TraceID
	}
	if run.TraceName == "" {
		run.TraceName = existing.TraceName
	}
	if run.EntryMethod == "" {
		run.EntryMethod = existing.EntryMethod
	}
	if run.ConversationID == "" {
		run.ConversationID = existing.ConversationID
	}
	if run.TaskID == "" {
		run.TaskID = existing.TaskID
	}
	if run.UserID == "" {
		run.UserID = existing.UserID
	}
	if run.ExtraData == "" {
		run.ExtraData = existing.ExtraData
	}
	if run.StartTime == nil {
		run.StartTime = existing.StartTime
	}
	if run.CreateTime.IsZero() {
		run.CreateTime = existing.CreateTime
	}
	r.runs[traceID] = run
	return nil
}

func (r *traceRunRepoRecorder) UpdateWhere(context.Context, port.RagTraceRunConditions, port.RagTraceRunPatch) (int64, error) {
	return 0, nil
}

func (r *traceRunRepoRecorder) GetByTraceID(_ context.Context, traceID string) (domain.RagTraceRun, error) {
	if r.runs == nil {
		return domain.RagTraceRun{}, nil
	}
	return r.runs[traceID], nil
}

func (r *traceRunRepoRecorder) Count(context.Context, port.RagTraceRunListFilter) (int, error) {
	return len(r.runs), nil
}

func (r *traceRunRepoRecorder) List(context.Context, port.RagTraceRunListFilter) ([]domain.RagTraceRun, error) {
	result := make([]domain.RagTraceRun, 0, len(r.runs))
	for _, run := range r.runs {
		result = append(result, run)
	}
	return result, nil
}

func TestTopChunkScore(t *testing.T) {
	if got := topChunkScore(ragretrieve.Result{}); got != 0 {
		t.Fatalf("empty result: expected 0, got %v", got)
	}

	result := ragretrieve.Result{
		Chunks: []convention.RetrievedChunk{
			{ID: "c1", Score: 0.85},
		},
	}
	if got := topChunkScore(result); got != 0.85 {
		t.Fatalf("single chunk: expected 0.85, got %v", got)
	}

	result = ragretrieve.Result{
		Chunks: []convention.RetrievedChunk{
			{ID: "c1", Score: 0.45},
			{ID: "c2", Score: 0.92},
			{ID: "c3", Score: 0.67},
		},
	}
	if got := topChunkScore(result); got != 0.92 {
		t.Fatalf("multi chunk: expected 0.92, got %v", got)
	}
}

func TestBuildFallbackPrompt(t *testing.T) {
	question := "what is the weather today"
	prompt := buildFallbackPrompt(question)

	if !strings.Contains(prompt, question) {
		t.Fatalf("expected question %q in fallback prompt, got: %s", question, prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "general model") {
		t.Fatalf("expected general-model fallback warning in fallback prompt, got: %s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "respond in chinese") {
		t.Fatalf("expected Chinese-response hint in fallback prompt, got: %s", prompt)
	}
}

func TestNewRagChatServiceWithDepsRejectsMissingRequiredDeps(t *testing.T) {
	tracer := NewChatTracer(nil, nil)
	_, err := NewRagChatServiceWithDeps(RagChatDeps{}, RagChatOptions{})
	if err == nil {
		t.Fatal("expected error for missing required deps")
	}

	_, err = NewRagChatServiceWithDeps(RagChatDeps{
		ConversationService: &ConversationService{},
		MessageService:      &ConversationMessageService{},
		HistoryService:      memoryServiceStub{},
		RetrieveService:     &retrieveServiceStub{},
		PromptService:       &ragprompt.Service{},
		ChatService:         &llmServiceStub{},
		Tracer:              tracer,
	}, RagChatOptions{})
	if err != nil {
		t.Fatalf("expected valid deps to succeed, got %v", err)
	}
}

func TestNewRagChatServiceWithDepsAppliesOptions(t *testing.T) {
	tracer := NewChatTracer(nil, nil)
	workflow := &toolWorkflowStub{}
	recall := &sessionRecallServiceStub{}
	service, err := NewRagChatServiceWithDeps(RagChatDeps{
		ConversationService: &ConversationService{},
		MessageService:      &ConversationMessageService{},
		HistoryService:      memoryServiceStub{},
		RetrieveService:     &retrieveServiceStub{},
		PromptService:       &ragprompt.Service{},
		ChatService:         &llmServiceStub{},
		Tracer:              tracer,
		AgentRuntime:        agentRuntimeServiceStub{},
	}, RagChatOptions{
		ConfidenceThreshold:    0.75,
		ParallelSubquestions:   false,
		SubquestionConcurrency: 4,
		RequestCacheMaxEntries: 64,
		AgentRuntimeMode:       ragChatAgentModeAlways,
		SessionRecall:          recall,
		ToolWorkflow:           workflow,
	})
	if err != nil {
		t.Fatalf("NewRagChatServiceWithDeps returned error: %v", err)
	}
	if service.confidenceThreshold != 0.75 {
		t.Fatalf("expected confidenceThreshold=0.75, got %v", service.confidenceThreshold)
	}
	if service.parallelSubquestions {
		t.Fatal("expected parallel subquestions disabled")
	}
	if service.subquestionConcurrency != 4 {
		t.Fatalf("expected subquestionConcurrency=4, got %d", service.subquestionConcurrency)
	}
	if service.requestCacheMaxEntries != 64 {
		t.Fatalf("expected requestCacheMaxEntries=64, got %d", service.requestCacheMaxEntries)
	}
	if service.agentRuntimeMode != ragChatAgentModeAlways {
		t.Fatalf("expected agent mode always, got %q", service.agentRuntimeMode)
	}
	if service.sessionRecall != recall {
		t.Fatal("expected session recall to be assigned")
	}
	if service.toolWorkflow != workflow {
		t.Fatal("expected tool workflow to be assigned")
	}
}

func TestRagChatServiceConfidenceThresholdDefaultsOff(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if svc.confidenceThreshold != 0 {
		t.Fatalf("expected confidenceThreshold=0 by default, got %v", svc.confidenceThreshold)
	}
}

func TestNewRagChatServiceWithDepsNormalizesParallelDefaults(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		ParallelSubquestions:   false,
		SubquestionConcurrency: 0,
		RequestCacheMaxEntries: 0,
	})
	if service.parallelSubquestions {
		t.Fatal("expected parallel subquestion retrieval to be disabled")
	}
	if service.subquestionConcurrency != 2 {
		t.Fatalf("expected default subquestion concurrency=2, got %d", service.subquestionConcurrency)
	}
	if service.requestCacheMaxEntries != 128 {
		t.Fatalf("expected default request cache max entries=128, got %d", service.requestCacheMaxEntries)
	}
}

func TestRagChatServiceValidateDependencies(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.validateDependencies(); err == nil {
		t.Fatal("expected validation error for nil dependencies")
	}
}

func TestRagChatServiceNilSink(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.Chat(nil, RagChatInput{Question: "doc_fail_01 why import failed", UserID: "u1"}, nil); err == nil {
		t.Fatal("expected error for nil sink")
	}
}

func TestRagChatServiceEmptyQuestion(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	sink := &fallbackSinkStub{}
	if err := svc.Chat(nil, RagChatInput{Question: "doc_fail_01 why import failed", UserID: "u1"}, sink); err == nil {
		t.Fatal("expected error for empty question or missing dependencies")
	}
}

func TestRagChatServiceEmptyUserID(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	sink := &fallbackSinkStub{}
	if err := svc.Chat(nil, RagChatInput{Question: "hello", UserID: ""}, sink); err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestShouldRunRetrieve(t *testing.T) {
	if shouldRunRetrieve(RagChatInput{}, ragrewrite.Result{NeedRetrieval: true}) {
		t.Fatal("expected no retrieve when knowledge base ids are empty")
	}
	if shouldRunRetrieve(RagChatInput{KnowledgeBaseIDs: []string{"kb-1"}}, ragrewrite.Result{NeedRetrieval: false}) {
		t.Fatal("expected no retrieve when rewrite says retrieval is unnecessary")
	}
	if !shouldRunRetrieve(RagChatInput{KnowledgeBaseIDs: []string{"kb-1"}}, ragrewrite.Result{NeedRetrieval: true}) {
		t.Fatal("expected retrieve when knowledge base exists and rewrite requires it")
	}
}

func TestShouldSerializeSubQuestions(t *testing.T) {
	if !shouldSerializeSubQuestions([]string{"this node error details", "continue checking"}) {
		t.Fatal("expected dependency-risk subquestions to serialize")
	}
	if !shouldSerializeSubQuestions([]string{"which node failed", "What error was returned"}) {
		t.Fatal("expected short subject-less follow-up to serialize")
	}
	if shouldSerializeSubQuestions([]string{"doc-1 indexing failure reason", "vector store connection refused troubleshooting"}) {
		t.Fatal("expected independent subquestions to remain parallelizable")
	}
}

func TestRunRetrieveStageParallelSuccessTrace(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	tracer := NewChatTracer(nil, repo)
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			time.Sleep(50 * time.Millisecond)
			return ragretrieve.Result{
				Chunks: []convention.RetrievedChunk{{ID: strings.TrimSpace(request.Query), Score: 0.8}},
			}, nil
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, tracer, RagChatOptions{
		ParallelSubquestions:   true,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"first independent question", "second independent question"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if !result.used {
		t.Fatal("expected retrieve stage to be used")
	}
	if result.executionMode != retrieveExecutionModeParallel {
		t.Fatalf("expected parallel execution mode, got %q", result.executionMode)
	}
	if len(result.subQuestions) != 2 {
		t.Fatalf("expected 2 subquestion results, got %d", len(result.subQuestions))
	}
	if retrieve.maxInFlight < 2 {
		t.Fatalf("expected parallel in-flight execution, got max=%d", retrieve.maxInFlight)
	}
	if result.wallClockDurationMs <= 0 || result.wallClockDurationMs >= 95 {
		t.Fatalf("expected wall clock to reflect parallel speedup, got %dms", result.wallClockDurationMs)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one trace node, got %d", len(repo.created))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal retrieve trace payload: %v", err)
	}
	if payload["executionMode"] != retrieveExecutionModeParallel {
		t.Fatalf("expected executionMode=parallel, got %+v", payload)
	}
	if payload["subQuestionSucceeded"] != float64(2) {
		t.Fatalf("expected two successful subquestions, got %+v", payload)
	}
}

func TestRunRetrieveStagePartialFailureStillReturnsMergedResult(t *testing.T) {
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			if strings.Contains(request.Query, "second") {
				return ragretrieve.Result{}, errors.New("vector store unavailable")
			}
			return ragretrieve.Result{
				Chunks: []convention.RetrievedChunk{{ID: "chunk-1", Score: 0.9}},
			}, nil
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, NewChatTracer(nil, nil), RagChatOptions{
		ParallelSubquestions:   true,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"first independent question", "second independent question"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if len(result.result.Chunks) != 1 {
		t.Fatalf("expected merged result from successful subquestion, got %+v", result.result.Chunks)
	}
	if len(retrieve.requests) != 2 {
		t.Fatalf("expected no fallback retrieve on partial failure, got requests=%d", len(retrieve.requests))
	}
	if result.subQuestions[1].Status != subQuestionStatusFailed {
		t.Fatalf("expected failed subquestion status, got %+v", result.subQuestions[1])
	}
}

func TestRunRetrieveStageFallsBackWhenAllSubQuestionsEmpty(t *testing.T) {
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			if request.Query == "original question" {
				return ragretrieve.Result{
					Chunks: []convention.RetrievedChunk{{ID: "fallback", Score: 0.88}},
				}, nil
			}
			return ragretrieve.Result{}, nil
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, NewChatTracer(nil, nil), RagChatOptions{
		ParallelSubquestions:   true,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"first independent question", "second independent question"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if len(result.result.Chunks) != 1 || result.result.Chunks[0].ID != "fallback" {
		t.Fatalf("expected fallback retrieve result, got %+v", result.result.Chunks)
	}
	if len(retrieve.requests) != 3 {
		t.Fatalf("expected two subquestions plus one fallback, got %d requests", len(retrieve.requests))
	}
	if retrieve.requests[2].Query != "original question" {
		t.Fatalf("expected fallback to original question, got %+v", retrieve.requests[2])
	}
}

func TestRunRetrieveStageFallsBackWhenAllSubQuestionsFail(t *testing.T) {
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			if request.Query == "original question" {
				return ragretrieve.Result{
					Chunks: []convention.RetrievedChunk{{ID: "fallback", Score: 0.77}},
				}, nil
			}
			return ragretrieve.Result{}, errors.New("backend unavailable")
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, NewChatTracer(nil, nil), RagChatOptions{
		ParallelSubquestions:   true,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"first independent question", "second independent question"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if len(result.result.Chunks) != 1 || result.result.Chunks[0].ID != "fallback" {
		t.Fatalf("expected fallback retrieve result, got %+v", result.result.Chunks)
	}
}

func TestRunRetrieveStageSerializesDependencyRiskSubQuestions(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	tracer := NewChatTracer(nil, repo)
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			time.Sleep(30 * time.Millisecond)
			return ragretrieve.Result{
				Chunks: []convention.RetrievedChunk{{ID: request.Query, Score: 0.6}},
			}, nil
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, tracer, RagChatOptions{
		ParallelSubquestions:   true,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"this node error details", "continue checking"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if result.executionMode != retrieveExecutionModeSerialDependencyRisk {
		t.Fatalf("expected dependency-risk serial mode, got %q", result.executionMode)
	}
	if retrieve.maxInFlight > 1 {
		t.Fatalf("expected dependency-risk execution to stay serial, got maxInFlight=%d", retrieve.maxInFlight)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal retrieve trace payload: %v", err)
	}
	if payload["executionMode"] != retrieveExecutionModeSerialDependencyRisk {
		t.Fatalf("expected serial dependency risk trace mode, got %+v", payload)
	}
}

func TestRunRetrieveStageHonorsParallelDisable(t *testing.T) {
	retrieve := &retrieveServiceStub{
		retrieveFn: func(ctx context.Context, request ragretrieve.Request) (ragretrieve.Result, error) {
			time.Sleep(20 * time.Millisecond)
			return ragretrieve.Result{
				Chunks: []convention.RetrievedChunk{{ID: request.Query, Score: 0.6}},
			}, nil
		},
	}
	service := newTestRagChatServiceWithRetrieve(t, retrieve, NewChatTracer(nil, nil), RagChatOptions{
		ParallelSubquestions:   false,
		SubquestionConcurrency: 2,
	})

	result, err := service.runRetrieveStage(context.Background(), RagChatInput{
		Question:         "original question",
		UserID:           "user-1",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, ragrewrite.Result{
		NeedRetrieval: true,
		SubQuestions:  []string{"first independent question", "second independent question"},
	}, "trace-1")
	if err != nil {
		t.Fatalf("runRetrieveStage returned error: %v", err)
	}
	if result.executionMode != retrieveExecutionModeSerial {
		t.Fatalf("expected serial mode when parallel disabled, got %q", result.executionMode)
	}
	if retrieve.maxInFlight > 1 {
		t.Fatalf("expected serial execution when parallel disabled, got maxInFlight=%d", retrieve.maxInFlight)
	}
}

func TestShouldRunToolWorkflow(t *testing.T) {
	if shouldRunToolWorkflow(RagChatInput{Question: "hello"}, ragrewrite.Result{NeedRetrieval: false}, false) {
		t.Fatal("expected no tool workflow for greeting without retrieval")
	}
	if !shouldRunToolWorkflow(RagChatInput{Question: "doc_fail_01 why failed"}, ragrewrite.Result{NeedRetrieval: false}, false) {
		t.Fatal("expected tool workflow for structured document id question")
	}
	if !shouldRunToolWorkflow(RagChatInput{Question: "How do Go generics work"}, ragrewrite.Result{NeedRetrieval: true}, false) {
		t.Fatal("expected tool workflow for general retrieval-style question")
	}
	if !shouldRunToolWorkflow(RagChatInput{Question: "random question"}, ragrewrite.Result{NeedRetrieval: false}, true) {
		t.Fatal("expected tool workflow when retrieval was used")
	}
}

func TestRunToolWorkflowStageSkipsWhenWorkflowUnset(t *testing.T) {
	svc := newRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	result, err := svc.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "q", UserID: "u"},
		nil,
		"",
		"",
		ragrewrite.Result{},
		ragretrieve.Result{},
		false,
		"trace-1",
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.result.Used {
		t.Fatal("expected empty workflow result when workflow is unset")
	}
}

func TestRunToolWorkflowStageRunsForStructuredIDWithoutRetrieve(t *testing.T) {
	workflow := &toolWorkflowStub{
		result: ragtool.WorkflowResult{Used: true},
	}
	svc := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		ToolWorkflow: workflow,
	})

	result, err := svc.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "doc_fail_01 why import failed", UserID: "u1"},
		nil,
		"",
		"",
		ragrewrite.Result{NeedRetrieval: false},
		ragretrieve.Result{},
		false,
		"trace-1",
		nil,
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage: %v", err)
	}
	if !result.result.Used {
		t.Fatal("expected workflow to run for structured id question")
	}
	if strings.TrimSpace(workflow.input.Question) != "doc_fail_01 why import failed" {
		t.Fatalf("unexpected workflow input question: %q", workflow.input.Question)
	}
}

func TestRunToolWorkflowStageReturnsWorkflowResult(t *testing.T) {
	workflow := &toolWorkflowStub{
		result: ragtool.WorkflowResult{
			Used:    true,
			Context: "tool context",
			Calls: []ragtool.CallSummary{
				{Name: "document_query", Status: ragtool.CallStatusSuccess, Summary: "matched doc-1"},
			},
		},
	}
	svc := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		ToolWorkflow: workflow,
	})

	history := []convention.ChatMessage{convention.UserMessage("previous")}
	rewriteResult := ragrewrite.Result{RewrittenQuestion: "rewritten"}
	retrieveResult := ragretrieve.Result{KnowledgeContext: "knowledge"}
	result, err := svc.runToolWorkflowStage(
		context.Background(),
		RagChatInput{
			ConversationID:   "conv-1",
			UserID:           "user-1",
			Question:         "why failed",
			KnowledgeBaseIDs: []string{"kb-1"},
		},
		history,
		"memory context",
		"session context",
		rewriteResult,
		retrieveResult,
		true,
		"trace-1",
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.result.Used {
		t.Fatal("expected workflow result to be used")
	}
	if result.result.Context != "tool context" {
		t.Fatalf("unexpected tool context: %q", result.result.Context)
	}
	if workflow.input.TraceID != "trace-1" {
		t.Fatalf("unexpected trace id: %q", workflow.input.TraceID)
	}
	if workflow.input.Control.ExecutionMode != ragtool.ExecutionModeReadOnly {
		t.Fatalf("unexpected workflow execution mode: %q", workflow.input.Control.ExecutionMode)
	}
	if workflow.input.Control.RiskLevel != ragtool.RiskLevelLow {
		t.Fatalf("unexpected workflow risk level: %q", workflow.input.Control.RiskLevel)
	}
	if workflow.input.Control.ApprovalRequirement != ragtool.ApprovalRequirementNone {
		t.Fatalf("unexpected workflow approval requirement: %q", workflow.input.Control.ApprovalRequirement)
	}
	if len(workflow.input.History) != 1 || workflow.input.History[0].Content != "previous" {
		t.Fatalf("unexpected history: %+v", workflow.input.History)
	}
}

func TestRecordToolCallTraceNodes(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tracer := NewChatTracer(nil, repo)
	_ = newRagChatService(nil, nil, nil, nil, nil, nil, nil, tracer)
	tracer.now = func() time.Time { return now }

	tracer.recordToolCallTraceNodes(context.Background(), "trace-1", []ragtool.CallSummary{
		{Name: "document_query", Status: ragtool.CallStatusSuccess, Summary: "matched doc-1", DurationMs: 12},
		{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, Summary: "task not found", DurationMs: 34},
	})

	if len(repo.created) != 2 {
		t.Fatalf("expected 2 tool call trace nodes, got %d", len(repo.created))
	}
	if repo.created[0].ParentNodeID != "tool_workflow" || repo.created[0].Depth != 2 {
		t.Fatalf("unexpected parent/depth: %+v", repo.created[0])
	}
	if repo.created[0].NodeID != "tool_01" || repo.created[1].NodeID != "tool_02" {
		t.Fatalf("unexpected tool node ids: %+v", repo.created)
	}
	if repo.created[0].NodeName != "document_query" || repo.created[1].NodeName != "task_ingestion_diagnose" {
		t.Fatalf("unexpected node names: %+v", repo.created)
	}
	if repo.created[1].ErrorMessage != "task not found" {
		t.Fatalf("expected failed tool error message to be persisted, got %q", repo.created[1].ErrorMessage)
	}
	if repo.created[0].DurationMs == nil || *repo.created[0].DurationMs != 12 {
		t.Fatalf("unexpected first duration: %+v", repo.created[0].DurationMs)
	}
	if repo.created[1].StartTime == nil || repo.created[1].EndTime == nil || !repo.created[1].EndTime.After(*repo.created[1].StartTime) {
		t.Fatalf("expected second node to have increasing timestamps: %+v", repo.created[1])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal extra data: %v", err)
	}
	if payload["summary"] != "matched doc-1" {
		t.Fatalf("unexpected summary payload: %+v", payload)
	}
}

func TestRecordAgentWorkflowTraceNodesUsesDatabaseSafeNames(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	now := time.Date(2026, 5, 10, 14, 48, 0, 0, time.UTC)
	tracer := NewChatTracer(nil, repo)
	tracer.now = func() time.Time { return now }

	tracer.recordAgentWorkflowTraceNodes(context.Background(), "trace-1", ragtool.WorkflowResult{
		TraceMeta: ragtool.WorkflowTraceMeta{
			Capability:          ragtool.CapabilitySearch,
			ExecutionMode:       ragtool.ExecutionModeReadOnly,
			RiskLevel:           ragtool.RiskLevelLow,
			ApprovalRequirement: ragtool.ApprovalRequirementNone,
			EvidenceSources:     []string{ragtool.EvidenceSourceKnowledgeBase, ragtool.EvidenceSourceExternalWeb},
		},
		Rounds: []ragtool.RoundSummary{
			{
				Round:               1,
				Done:                true,
				Reasoning:           "enough evidence",
				PlanningSource:      "hint_calls",
				LLMPlannerSkipped:   true,
				NextHintCallCount:   1,
				ExecutionMode:       "parallel",
				WallClockDurationMs: 10,
				ToolCallCount:       1,
				TotalToolDurationMs: 12,
				Calls: []ragtool.CallSummary{
					{
						CallID:     "round_1_call_01",
						Round:      1,
						Sequence:   1,
						Name:       "document_ingestion_diagnose",
						Status:     ragtool.CallStatusSuccess,
						Summary:    "doc failed",
						DurationMs: 12,
					},
				},
			},
		},
	})

	if len(repo.created) != 3 {
		t.Fatalf("expected 3 trace nodes, got %d", len(repo.created))
	}
	if repo.created[0].NodeID != "agt_round_01" || repo.created[0].NodeType != "agt_round" {
		t.Fatalf("unexpected round node: %+v", repo.created[0])
	}
	if repo.created[2].NodeID != "agt_obs_01" || repo.created[2].NodeType != "agt_obs" {
		t.Fatalf("unexpected observation node: %+v", repo.created[2])
	}
	if len(repo.created[0].NodeType) > 16 || len(repo.created[2].NodeType) > 16 {
		t.Fatal("expected trace node types to stay within varchar(16)")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal round extra data: %v", err)
	}
	if payload["executionMode"] != "parallel" {
		t.Fatalf("expected executionMode=parallel, got %+v", payload)
	}
	if payload["planningSource"] != "hint_calls" {
		t.Fatalf("expected planningSource=hint_calls, got %+v", payload)
	}
	if payload["llmPlannerSkipped"] != true {
		t.Fatalf("expected llmPlannerSkipped=true, got %+v", payload)
	}
	if payload["nextHintCallCount"] != float64(1) {
		t.Fatalf("expected nextHintCallCount=1, got %+v", payload)
	}
	if payload["capability"] != ragtool.CapabilitySearch {
		t.Fatalf("expected capability=search, got %+v", payload)
	}
	if payload["workflowMode"] != ragtool.ExecutionModeReadOnly {
		t.Fatalf("expected workflowMode=read_only, got %+v", payload)
	}
	if payload["wallClockDurationMs"] != float64(10) {
		t.Fatalf("expected wallClockDurationMs=10, got %+v", payload)
	}
}

func newPrepareChatTestService(
	t *testing.T,
	rewriteResult ragrewrite.Result,
	sessionRecall SessionRecallService,
	retrieve ragretrieve.Service,
	configure ...func(*RagChatDeps, *RagChatOptions),
) (*RagChatService, *domain.ConversationMessage) {
	t.Helper()

	var createdMessage domain.ConversationMessage
	conversationService := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				if conversation.ID == "" {
					conversation.ID = "conv-internal-1"
				}
				if conversation.Title == "" {
					conversation.Title = "test conversation"
				}
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
		30,
		nil,
	)

	messageService := NewConversationMessageService(
		conversationMessageConversationRepoStub{
			getByConversationIDAndUserFn: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{ID: "conv-internal-1", ConversationID: "conv-1", UserID: "user-1"}, nil
			},
		},
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				createdMessage = message
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{
			createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
				return domain.ConversationSummary{}, nil
			},
		},
		nil,
	)

	if retrieve == nil {
		retrieve = &retrieveServiceStub{}
	}

	deps := RagChatDeps{
		ConversationService: conversationService,
		MessageService:      messageService,
		HistoryService:      memoryServiceStub{},
		RewriteService:      rewriteServiceStub{result: rewriteResult},
		RetrieveService:     retrieve,
		PromptService:       ragprompt.NewService(nil),
		ChatService:         &llmServiceStub{},
		Tracer:              NewChatTracer(nil, nil),
	}
	opts := RagChatOptions{SessionRecall: sessionRecall}
	for _, apply := range configure {
		if apply != nil {
			apply(&deps, &opts)
		}
	}
	service := mustNewTestRagChatService(t, deps, opts)
	return service, &createdMessage
}

func TestRunSessionRecallStageUsesRewrittenQuestion(t *testing.T) {
	recall := &sessionRecallServiceStub{
		result: SessionRecallResult{Used: true, Context: "ctx"},
	}
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		SessionRecall: recall,
	})

	result, err := service.runSessionRecallStage(
		context.Background(),
		"conv-1",
		RagChatInput{Question: "original question", UserID: "user-1"},
		"msg-current",
		ragrewrite.Result{RewrittenQuestion: "rewritten question"},
		"trace-1",
	)
	if err != nil {
		t.Fatalf("runSessionRecallStage returned error: %v", err)
	}
	if !result.result.Used {
		t.Fatalf("expected used session recall result, got %+v", result.result)
	}
	if recall.input.Query != "rewritten question" {
		t.Fatalf("expected rewritten query, got %q", recall.input.Query)
	}
	if recall.input.ExcludeMessageID != "msg-current" {
		t.Fatalf("expected exclude message id to be forwarded, got %q", recall.input.ExcludeMessageID)
	}
	if recall.input.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id to be forwarded, got %q", recall.input.ConversationID)
	}
}

func TestRunSessionRecallStageTraceContainsSelectedHits(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	tracer := NewChatTracer(nil, repo)
	service := mustNewTestRagChatService(t, RagChatDeps{
		ConversationService: &ConversationService{},
		MessageService:      &ConversationMessageService{},
		HistoryService:      memoryServiceStub{},
		RetrieveService:     &retrieveServiceStub{},
		PromptService:       ragprompt.NewService(nil),
		ChatService:         &llmServiceStub{},
		Tracer:              tracer,
	}, RagChatOptions{
		SessionRecall: &sessionRecallServiceStub{
			result: SessionRecallResult{
				Used:                   true,
				TopScore:               0.91,
				candidateCount:         4,
				skippedPerMessageLimit: 1,
				truncatedBy:            "max_prompt_tokens",
				Hits: []SessionRecallHit{
					{
						MessageID:     "msg-previous",
						ChunkIndex:    2,
						Score:         0.91,
						SourceChunkID: "chunk-2",
					},
				},
			},
		},
	})

	_, err := service.runSessionRecallStage(
		context.Background(),
		"conv-1",
		RagChatInput{Question: "follow-up", UserID: "user-1"},
		"msg-current",
		ragrewrite.Result{RewrittenQuestion: "rewritten follow-up"},
		"trace-1",
	)
	if err != nil {
		t.Fatalf("runSessionRecallStage returned error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one trace node, got %d", len(repo.created))
	}
	if repo.created[0].NodeID != "session_recall" || repo.created[0].NodeType != "memory" {
		t.Fatalf("unexpected trace node: %+v", repo.created[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal session recall extra data: %v", err)
	}
	if payload["candidateCount"] != float64(4) {
		t.Fatalf("expected candidateCount=4, got %+v", payload)
	}
	if payload["skippedPerMessageLimit"] != float64(1) {
		t.Fatalf("expected skippedPerMessageLimit=1, got %+v", payload)
	}
	if payload["truncatedBy"] != "max_prompt_tokens" {
		t.Fatalf("expected truncatedBy=max_prompt_tokens, got %+v", payload)
	}
	selectedHits, ok := payload["selectedHits"].([]any)
	if !ok || len(selectedHits) != 1 {
		t.Fatalf("expected selectedHits payload, got %+v", payload["selectedHits"])
	}
	firstHit, ok := selectedHits[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first hit map, got %#v", selectedHits[0])
	}
	if firstHit["messageId"] != "msg-previous" || firstHit["chunkIndex"] != float64(2) {
		t.Fatalf("unexpected selected hit payload: %+v", firstHit)
	}
}

func TestPrepareChatLongTermMemoryFailsOpen(t *testing.T) {
	explicitMemory := longtermmemory.NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(context.Context, port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			return nil, errors.New("memory recall failed")
		},
	}, longtermmemory.MemoryServiceOptions{})

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "rewritten", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(deps *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = explicitMemory.RecallService()
		},
	)

	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "follow-up question",
		KnowledgeBaseIDs: []string{
			"kb-ops",
		},
	})
	if err != nil {
		t.Fatalf("prepareChat returned error: %v", err)
	}
	if prepared.memoryContext != "" {
		t.Fatalf("expected empty memory context on fail-open path, got %q", prepared.memoryContext)
	}
}

func TestPrepareChatIncludesLongTermMemoryContextInPrompt(t *testing.T) {
	explicitMemory := longtermmemory.NewMemoryService(memoryItemRepoStub{
		createFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		updateFn: func(context.Context, domain.MemoryItem) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		getByID:  func(context.Context, string) (domain.MemoryItem, error) { return domain.MemoryItem{}, nil },
		listFn: func(_ context.Context, filter port.MemoryItemListFilter) ([]domain.MemoryItem, error) {
			if len(filter.ScopeTypes) == 1 && filter.ScopeTypes[0] == domain.MemoryScopeKB {
				if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypePreference {
					return nil, nil
				}
				return []domain.MemoryItem{
					{
						ID:         "mem-kb-1",
						UserID:     "user-1",
						ScopeType:  domain.MemoryScopeKB,
						ScopeID:    "kb-ops",
						MemoryType: domain.MemoryTypeKnowledge,
						Summary:    "Ops troubleshooting note.",
						Content:    "When ingestion fails with connection refused, check vector store connectivity before retrying the pipeline.",
						Status:     domain.MemoryStatusActive,
						UpdateTime: time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC),
					},
				}, nil
			}
			if len(filter.MemoryTypes) == 1 && filter.MemoryTypes[0] == domain.MemoryTypeKnowledge {
				return nil, nil
			}
			return []domain.MemoryItem{
				{
					ID:         "mem-global-1",
					UserID:     "user-1",
					ScopeType:  domain.MemoryScopeGlobal,
					MemoryType: domain.MemoryTypePreference,
					Summary:    "Answer with the action order first.",
					Content:    "For incident-style questions, lead with investigation steps before background explanation.",
					Status:     domain.MemoryStatusActive,
					UpdateTime: time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC),
				},
			}, nil
		},
	}, longtermmemory.MemoryServiceOptions{MaxRecallItems: 5, MaxRecallChars: 1200})

	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{
			RewrittenQuestion: "How should we troubleshoot connection refused for doc-1?",
			SubQuestions:      []string{"connection refused troubleshooting", "vector store connectivity check"},
			NeedRetrieval:     false,
		},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryRecall = explicitMemory.RecallService()
		},
	)

	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		ConversationID:   "conv-1",
		UserID:           "user-1",
		Question:         "How should we troubleshoot connection refused for doc-1?",
		KnowledgeBaseIDs: []string{"kb-ops"},
	})
	if err != nil {
		t.Fatalf("prepareChat returned error: %v", err)
	}
	if !strings.Contains(prepared.memoryContext, "KB-Scoped Memories:") {
		t.Fatalf("expected scoped memory context, got %q", prepared.memoryContext)
	}
	if !strings.Contains(prepared.memoryContext, "Rule Memories:") || !strings.Contains(prepared.memoryContext, "Fact Memories:") {
		t.Fatalf("expected split long-term memory sections, got %q", prepared.memoryContext)
	}
	if !strings.Contains(prepared.memoryContext, "vector store connectivity") {
		t.Fatalf("expected projected detail in memory context, got %q", prepared.memoryContext)
	}

	promptStage, err := service.runPromptStage(
		context.Background(),
		"How should we troubleshoot connection refused for doc-1?",
		prepared.history,
		prepared.memoryContext,
		prepared.sessionContext,
		ragretrieve.Result{},
		"",
		"",
		"",
		"",
		prepared.state.traceID,
	)
	if err != nil {
		t.Fatalf("runPromptStage returned error: %v", err)
	}
	if len(promptStage.messages) < 2 {
		t.Fatalf("expected prompt to include memory context, got %+v", promptStage.messages)
	}
	if !strings.Contains(promptStage.messages[1].Content, "## Long-Term Memory") {
		t.Fatalf("expected long-term memory section, got %q", promptStage.messages[1].Content)
	}
	if !strings.Contains(promptStage.messages[1].Content, "vector store") {
		t.Fatalf("expected memory context prompt to include troubleshooting detail, got %q", promptStage.messages[1].Content)
	}
}

func TestPrepareChatRecallsEarlierConfigMessageIntoPrompt(t *testing.T) {
	chunkStore := newInMemorySessionChunkStore()
	var storedMessages []domain.ConversationMessage
	messageService := NewConversationMessageService(
		nil,
		conversationMessageRepoServiceStub{
			createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
				storedMessages = append(storedMessages, message)
				return message, nil
			},
			listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
				return nil, nil
			},
		},
		conversationSummaryRepoServiceStub{
			createFn: func(context.Context, domain.ConversationSummary) (domain.ConversationSummary, error) {
				return domain.ConversationSummary{}, nil
			},
		},
		nil,
	)
	messageService.SetContentProcessor(NewLongMessageContentProcessor(LongMessageProcessorOptions{
		Enabled:                     true,
		DirectContextMaxTokens:      5,
		ChunkSummaryThresholdTokens: 40,
		LargeChunkTargetTokens:      20,
		LargeChunkOverlapTokens:     4,
		MediumSummaryMaxChars:       120,
		ChunkSummaryMaxChars:        80,
		Estimator:                   fixedTokenEstimator{factor: 4},
	}))
	messageService.SetChunkSink(chunkStore)

	previousConfig := strings.Join([]string{
		"rag:",
		"  search:",
		"    web-search:",
		"      provider: tavily-mcp",
		"      fallback-provider: tavily",
		"      source-policy:",
		"        allow-domains:",
		"          - go.dev",
	}, "\n")
	previousMessage, err := messageService.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Role:           convention.UserRole,
		Content:        previousConfig,
	})
	if err != nil {
		t.Fatalf("AddMessage returned error: %v", err)
	}
	if !previousMessage.IsSummarized || len(chunkStore.chunks) == 0 {
		t.Fatalf("expected previous config message to be summarized with session chunks: %+v", previousMessage)
	}

	conversationService := NewConversationService(
		conversationRepoStub{
			createFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				if conversation.ID == "" {
					conversation.ID = "conv-internal-1"
				}
				if conversation.Title == "" {
					conversation.Title = "test conversation"
				}
				return conversation, nil
			},
			updateFn: func(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
				if conversation.ID == "" {
					conversation.ID = "conv-internal-1"
				}
				if conversation.Title == "" {
					conversation.Title = "test conversation"
				}
				return conversation, nil
			},
			deleteFn: func(context.Context, string) error { return nil },
			getByConversationIDAndUser: func(context.Context, string, string) (domain.Conversation, error) {
				return domain.Conversation{ID: "conv-internal-1", ConversationID: "conv-1", UserID: "user-1", Title: "test conversation"}, nil
			},
			listByUserIDFn: func(context.Context, string) ([]domain.Conversation, error) { return nil, nil },
		},
		conversationMessageRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		conversationSummaryRepoStub{deleteFn: func(context.Context, string, string) error { return nil }},
		nil,
		nil,
		30,
		nil,
	)

	recallService := NewSessionRecallService(chunkStore, &sessionRecallEmbeddingStub{}, SessionRecallOptions{
		Enabled:              true,
		MaxExcerpts:          3,
		MaxChunksPerMessage:  2,
		ExcerptTargetTokens:  22,
		ExcerptOverlapTokens: 4,
		MaxPromptTokens:      140,
		Estimator:            fixedTokenEstimator{factor: 4},
	})

	service := mustNewTestRagChatService(t, RagChatDeps{
		ConversationService: conversationService,
		MessageService:      messageService,
		HistoryService: memoryServiceStub{
			history: []convention.ChatMessage{
				convention.UserMessage(previousMessage.Content),
			},
		},
		RewriteService: rewriteServiceStub{
			result: ragrewrite.Result{
				RewrittenQuestion: "What was the web-search provider in the earlier config?",
				SubQuestions:      []string{"public website access policy", "network access constraint"},
				NeedRetrieval:     false,
			},
		},
		RetrieveService: &retrieveServiceStub{},
		PromptService:   ragprompt.NewService(nil),
		ChatService:     &llmServiceStub{},
		Tracer:          NewChatTracer(nil, nil),
	}, RagChatOptions{
		SessionRecall: recallService,
	})

	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "What was the web-search provider in the earlier config?",
	})
	if err != nil {
		t.Fatalf("prepareChat returned error: %v", err)
	}
	if strings.TrimSpace(prepared.sessionContext) == "" {
		t.Fatalf("expected session context to be recalled")
	}
	if !strings.Contains(prepared.sessionContext, "provider: tavily-mcp") {
		t.Fatalf("expected recalled config detail, got %q", prepared.sessionContext)
	}
	if !strings.Contains(prepared.sessionContext, previousMessage.ID) {
		t.Fatalf("expected recalled context to mention previous message id %q, got %q", previousMessage.ID, prepared.sessionContext)
	}
	if len(storedMessages) < 2 {
		t.Fatalf("expected follow-up question to be persisted as a new message, got %d messages", len(storedMessages))
	}
	currentMessageID := storedMessages[len(storedMessages)-1].ID
	if strings.Contains(prepared.sessionContext, currentMessageID) {
		t.Fatalf("expected current message %q to be excluded from recalled context %q", currentMessageID, prepared.sessionContext)
	}

	promptStage, err := service.runPromptStage(
		context.Background(),
		"What was the web-search provider in the earlier config?",
		prepared.history,
		"",
		prepared.sessionContext,
		ragretrieve.Result{},
		"",
		"",
		"",
		"",
		prepared.state.traceID,
	)
	if err != nil {
		t.Fatalf("runPromptStage returned error: %v", err)
	}
	if len(promptStage.messages) < 2 || !strings.Contains(promptStage.messages[1].Content, "provider: tavily-mcp") {
		t.Fatalf("expected prompt session context to include recalled config detail, got %+v", promptStage.messages)
	}
}

func TestRagChatServiceChatStreamsAndFinishes(t *testing.T) {
	service, createdMessage := newPrepareChatTestService(
		t,
		ragrewrite.Result{
			RewrittenQuestion: "hello",
			SubQuestions:      []string{"public internet access policy", "service network restriction"},
			NeedRetrieval:     false,
		},
		nil,
		&retrieveServiceStub{},
	)
	service.chatService = &llmServiceStub{
		streamFn: func(request convention.ChatRequest, callback aichat.StreamCallback) {
			callback.OnThinking("thinking")
			callback.OnContent("final answer")
			callback.OnComplete()
		},
	}

	sink := &fallbackSinkStub{}
	err := service.Chat(context.Background(), RagChatInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "hello",
	}, sink)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if sink.finishCalls != 1 || sink.doneCalls != 1 {
		t.Fatalf("expected finish/done once, got finish=%d done=%d", sink.finishCalls, sink.doneCalls)
	}
	if sink.errorCalls != 0 || sink.cancelCalls != 0 {
		t.Fatalf("expected no error/cancel events, got error=%d cancel=%d", sink.errorCalls, sink.cancelCalls)
	}
	if createdMessage == nil {
		t.Fatal("expected created assistant message")
	}
	if createdMessage.Role != string(convention.AssistantRole) {
		t.Fatalf("expected assistant role, got %q", createdMessage.Role)
	}
	if createdMessage.Content != "final answer" {
		t.Fatalf("expected persisted answer content, got %q", createdMessage.Content)
	}
	if createdMessage.ThinkingContent != "thinking" {
		t.Fatalf("expected persisted thinking content, got %q", createdMessage.ThinkingContent)
	}
}

func TestRagChatServiceChatTriggersFallbackGuard(t *testing.T) {
	traceRuns := &traceRunRepoRecorder{}
	traceNodes := &traceNodeRepoRecorder{}
	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{
			RewrittenQuestion: "kb question",
			SubQuestions:      []string{"public internet access policy", "service network restriction"},
			NeedRetrieval:     true,
		},
		nil,
		&retrieveServiceStub{
			result: ragretrieve.Result{
				KnowledgeContext: "knowledge context",
				Chunks: []convention.RetrievedChunk{
					{ID: "chunk-1", Score: 0.25},
				},
			},
		},
	)
	service.tracer = NewChatTracer(traceRuns, traceNodes)
	service.chatService = &llmServiceStub{
		streamFn: func(request convention.ChatRequest, callback aichat.StreamCallback) {
			callback.OnContent("fallback answer")
			callback.OnComplete()
		},
	}
	service.confidenceThreshold = 0.8

	sink := &fallbackSinkStub{}
	err := service.Chat(context.Background(), RagChatInput{
		ConversationID:   "conv-1",
		UserID:           "user-1",
		Question:         "kb question",
		KnowledgeBaseIDs: []string{"kb-1"},
	}, sink)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if sink.fallbackCalls != 1 {
		t.Fatalf("expected one fallback event, got %d", sink.fallbackCalls)
	}
	if strings.TrimSpace(sink.fallbackReason) == "" {
		t.Fatal("expected fallback reason to be recorded")
	}
	if len(traceRuns.runs) != 1 {
		t.Fatalf("expected one trace run, got %d", len(traceRuns.runs))
	}
	var run domain.RagTraceRun
	for _, item := range traceRuns.runs {
		run = item
	}
	if !strings.Contains(run.ExtraData, "\"fallback\"") || !strings.Contains(run.ExtraData, "\"triggered\":true") {
		t.Fatalf("expected fallback trace extra, got %q", run.ExtraData)
	}
	foundFallbackNode := false
	for _, node := range traceNodes.created {
		if node.NodeID == "fallback" && node.NodeType == "fallback" {
			foundFallbackNode = true
			break
		}
	}
	if !foundFallbackNode {
		t.Fatalf("expected fallback trace node, got %+v", traceNodes.created)
	}
}

func TestHandleCancelledResultSendsCancelAndDone(t *testing.T) {
	service, createdMessage := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "hello", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
	)
	sink := &fallbackSinkStub{}

	err := service.handleCancelledResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "hello"},
		ragChatRuntimeState{
			meta:    RagChatMeta{ConversationID: "conv-1"},
			title:   "title",
			traceID: "trace-1",
		},
		ragChatTaskResult{
			cancelled: true,
			content:   "partial answer",
		},
		sink,
	)
	if err != nil {
		t.Fatalf("handleCancelledResult returned error: %v", err)
	}
	if sink.cancelCalls != 1 || sink.doneCalls != 1 {
		t.Fatalf("expected cancel/done once, got cancel=%d done=%d", sink.cancelCalls, sink.doneCalls)
	}
	if createdMessage == nil || createdMessage.Role != string(convention.AssistantRole) {
		t.Fatalf("expected cancelled path to persist assistant message, got %+v", createdMessage)
	}
}

func TestHandleFailedResultSendsErrorAndDone(t *testing.T) {
	service := &RagChatService{tracer: NewChatTracer(nil, nil)}
	sink := &fallbackSinkStub{}
	expectedErr := errors.New("stream failed")

	err := service.handleFailedResult(
		context.Background(),
		ragChatRuntimeState{traceID: "trace-1"},
		ragChatTaskResult{err: expectedErr},
		sink,
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if sink.errorCalls != 1 || sink.doneCalls != 1 {
		t.Fatalf("expected error/done once, got error=%d done=%d", sink.errorCalls, sink.doneCalls)
	}
}
