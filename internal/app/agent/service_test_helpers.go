package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentknowledgediscovery "local/rag-project/internal/app/agent/knowledge_discovery"
	agentmemoryrecall "local/rag-project/internal/app/agent/memory_recall"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	longtermmemory "local/rag-project/internal/app/rag/service/longtermmemory"
)

type stubSearchService struct{}

func (stubSearchService) Search(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
	return agentsearch.SearchOutput{}, nil
}

type stubFetchService struct{}

func (stubFetchService) Fetch(_ context.Context, _ []string) (agentfetch.Output, error) {
	return agentfetch.Output{}, nil
}

type stubRuntimeProvider struct {
	search func(query string) ([]searchprovider.SearchResult, error)
}

func (p stubRuntimeProvider) Search(query string) ([]searchprovider.SearchResult, error) {
	return p.search(query)
}

func (p stubRuntimeProvider) ProviderName() string {
	return "stub"
}

type stubFetchFlow struct {
	fetch func(ctx context.Context, urls []string) (agentfetch.Output, error)
}

func (s stubFetchFlow) Fetch(ctx context.Context, urls []string) (agentfetch.Output, error) {
	return s.fetch(ctx, urls)
}

type stubDocumentInvestigator struct{}

func (stubDocumentInvestigator) Get(context.Context, knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return knowledgedomain.KnowledgeDocument{}, nil
}

func (stubDocumentInvestigator) PageChunkLogs(context.Context, knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	return knowledgeservice.KnowledgeDocumentChunkLogPageResult{}, nil
}

var _ agentdocumentinvestigation.Investigator = stubDocumentInvestigator{}

type stubKnowledgeDiscoverer struct{}

func (stubKnowledgeDiscoverer) PageBases(context.Context, knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error) {
	return knowledgeservice.KnowledgeBasePageResult{}, nil
}

func (stubKnowledgeDiscoverer) PageDocuments(context.Context, knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error) {
	return knowledgeservice.KnowledgeDocumentPageResult{}, nil
}

func (stubKnowledgeDiscoverer) SearchDocuments(context.Context, knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error) {
	return nil, nil
}

var _ agentknowledgediscovery.KnowledgeDiscoverer = stubKnowledgeDiscoverer{}

type stubMemoryRecaller struct{}

func (stubMemoryRecaller) RecallMemories(context.Context, longtermmemory.RecallMemoriesInput) (longtermmemory.RecallMemoriesResult, error) {
	return longtermmemory.RecallMemoriesResult{}, nil
}

var _ agentmemoryrecall.MemoryRecaller = stubMemoryRecaller{}

func mustSearchHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentsearch.NewCapability(stubSearchService{})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	return handle
}

func mustFetchHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentfetch.NewCapability(stubFetchService{})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}
	return handle
}

func assertPendingSessionMissing(t *testing.T, service *Service, checkpointID string, sessionID string) {
	t.Helper()
	storedByCheckpoint, ok, err := service.sessionStore.Get(context.Background(), checkpointID)
	if err != nil {
		t.Fatalf("sessionStore.Get(checkpoint) error = %v", err)
	}
	if ok || storedByCheckpoint != nil {
		t.Fatalf("expected checkpoint session to be cleared, got ok=%v session=%+v", ok, storedByCheckpoint)
	}
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	storedBySessionID, ok, err := service.sessionStore.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("sessionStore.Get(sessionID) error = %v", err)
	}
	if ok || storedBySessionID != nil {
		t.Fatalf("expected session id entry to be cleared, got ok=%v session=%+v", ok, storedBySessionID)
	}
}

type recordingSessionStore struct {
	inner   *agentruntime.MemorySessionStore
	mu      sync.Mutex
	puts    []recordedSessionStorePut
	deletes []string
}

type recordedSessionStorePut struct {
	key     string
	session *agentruntime.RuntimeSession
}

func newRecordingSessionStore() *recordingSessionStore {
	return &recordingSessionStore{
		inner: agentruntime.NewMemorySessionStore(),
	}
}

func (s *recordingSessionStore) Put(ctx context.Context, checkpointID string, session *agentruntime.RuntimeSession) error {
	if err := s.inner.Put(ctx, checkpointID, session); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts = append(s.puts, recordedSessionStorePut{
		key:     strings.TrimSpace(checkpointID),
		session: agentruntime.CloneSession(session),
	})
	return nil
}

func (s *recordingSessionStore) Get(ctx context.Context, checkpointID string) (*agentruntime.RuntimeSession, bool, error) {
	return s.inner.Get(ctx, checkpointID)
}

func (s *recordingSessionStore) Delete(ctx context.Context, checkpointID string) error {
	if err := s.inner.Delete(ctx, checkpointID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes = append(s.deletes, strings.TrimSpace(checkpointID))
	return nil
}

func (s *recordingSessionStore) lastPut(key string) (*agentruntime.RuntimeSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	trimmed := strings.TrimSpace(key)
	for idx := len(s.puts) - 1; idx >= 0; idx-- {
		if s.puts[idx].key == trimmed {
			return agentruntime.CloneSession(s.puts[idx].session), true
		}
	}
	return nil, false
}

func newTestAgentService(t *testing.T, searchHandle agentcapability.Handle, fetchHandle agentcapability.Handle) *Service {
	return newTestAgentServiceWithPattern(t, PatternReactive, searchHandle, fetchHandle)
}

func newTestAgentServiceWithPattern(t *testing.T, patternName string, searchHandle agentcapability.Handle, fetchHandle agentcapability.Handle) *Service {
	return newTestAgentServiceWithPatternAndStore(t, patternName, searchHandle, fetchHandle, agentruntime.NewMemorySessionStore())
}

func newTestAgentServiceWithPatternAndStore(t *testing.T, patternName string, searchHandle agentcapability.Handle, fetchHandle agentcapability.Handle, sessionStore agentruntime.SessionStore) *Service {
	t.Helper()

	checkpointStore := agentkernel.NewMemoryCheckpointStore()
	pendingStore := agentruntime.NewMemoryPendingApprovalStore()
	registry := agentcapability.NewRegistry()
	if err := registry.Register(searchHandle); err != nil {
		t.Fatalf("Register(search) error = %v", err)
	}
	if err := registry.Register(fetchHandle); err != nil {
		t.Fatalf("Register(fetch) error = %v", err)
	}
	bindings := agentcapability.RoleBindings{
		agentcapability.RoleSearch: agentcapability.NameWebSearch,
		agentcapability.RoleFetch:  agentcapability.NameWebFetch,
	}
	patternName = normalizePattern(patternName)
	runner, err := compileRunner(context.Background(), patternName, registry, bindings, agentpattern.RuntimeConfig{
		OutputMode:           agentstate.OutputModeFinalAnswer,
		ApprovalSessionStore: sessionStore,
		Kernel: agentkernel.BuilderConfig{
			GraphName:       runtimeNameForPattern(patternName) + "_test",
			Reducer:         agentstate.DefaultReducer{},
			CheckpointStore: checkpointStore,
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return &Service{
		kernelRunner:  runner,
		runtimeEngine: agentruntime.NewEngine(runner),
		handoff:       buildHandoffBuilder(registry, bindings, patternName),
		registry:      registry,
		bindings:      bindings,
		sessionStore:  sessionStore,
		pendingStore:  pendingStore,
		reducer:       agentstate.DefaultReducer{},
		maxIterations: 2,
		outputMode:    agentstate.OutputModeFinalAnswer,
		pattern:       patternName,
		runtimeName:   runtimeNameForPattern(patternName),
	}
}
