package agent

import (
	"context"
	"errors"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestServiceRunDetailed_ReturnsServiceNotInitialized(t *testing.T) {
	var service *Service

	_, err := service.RunDetailed(context.Background(), Request{
		Question: "service not initialized run",
	})
	if err == nil {
		t.Fatal("expected service not initialized error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeServiceNotInitialized, ErrorKindInternal, false)
}

func TestServiceRunHandoffDetailed_ReturnsServiceNotInitialized(t *testing.T) {
	var service *Service

	_, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "service not initialized handoff run",
	})
	if err == nil {
		t.Fatal("expected service not initialized error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeServiceNotInitialized, ErrorKindInternal, false)
}

func TestServiceResumeAfterApproval_ReturnsServiceNotInitialized(t *testing.T) {
	var service *Service

	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-service-not-initialized",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected service not initialized error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeServiceNotInitialized, ErrorKindInternal, false)
}

func TestServiceResumeHandoffAfterApproval_ReturnsServiceNotInitialized(t *testing.T) {
	var service *Service

	_, err := service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-handoff-service-not-initialized",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected service not initialized error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeServiceNotInitialized, ErrorKindInternal, false)
}

func TestServiceResumeHandoffAfterApproval_RequiresSessionStore(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	service.sessionStore = nil

	_, err := service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-handoff-no-store",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected session store not initialized error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeSessionStoreNotInitialized, ErrorKindInternal, false)
}

func TestServiceRunDetailed_ReturnsApprovalSessionSaveFailed(t *testing.T) {
	store := &faultingSessionStore{
		inner:  agentruntime.NewMemorySessionStore(),
		putErr: errors.New("put failed"),
	}
	service := newApprovalContractServiceWithStore(t, store)

	_, err := service.RunDetailed(context.Background(), Request{
		Question: "save failure final answer",
	})
	if err == nil {
		t.Fatal("expected approval session save failed error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionSaveFailed, ErrorKindUnavailable, true)
}

func TestServiceRunHandoffDetailed_ReturnsApprovalSessionSaveFailed(t *testing.T) {
	store := &faultingSessionStore{
		inner:  agentruntime.NewMemorySessionStore(),
		putErr: errors.New("put failed"),
	}
	service := newApprovalContractServiceWithStore(t, store)

	_, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "save failure handoff",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err == nil {
		t.Fatal("expected approval session save failed error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionSaveFailed, ErrorKindUnavailable, true)
}

func TestServiceResumeAfterApproval_ReturnsApprovalSessionDeleteFailed(t *testing.T) {
	store := &faultingSessionStore{
		inner:     agentruntime.NewMemorySessionStore(),
		deleteErr: errors.New("delete failed"),
	}
	service := newApprovalContractServiceWithStore(t, store)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "delete failure final answer",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval initial outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected approval session delete failed error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionDeleteFailed, ErrorKindUnavailable, true)
}

func TestServiceResumeHandoffAfterApproval_ReturnsApprovalSessionDeleteFailed(t *testing.T) {
	store := &faultingSessionStore{
		inner:     agentruntime.NewMemorySessionStore(),
		deleteErr: errors.New("delete failed"),
	}
	service := newApprovalContractServiceWithStore(t, store)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "delete failure handoff",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval initial outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected approval session delete failed error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionDeleteFailed, ErrorKindUnavailable, true)
}

func newApprovalContractServiceWithStore(t *testing.T, store agentruntime.SessionStore) *Service {
	t.Helper()

	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{
				{
					Title:   "Approval Contract",
					URL:     "https://approval-contract.example/doc",
					Snippet: "approval required for " + query,
					Domain:  "approval-contract.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary: "approval contract fetched content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "approval contract fetched content"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	return newTestAgentServiceWithPatternAndStore(t, PatternReactive, searchHandle, fetchHandle, store)
}

func assertServiceErrorDescriptor(t *testing.T, err error, code string, kind string, retryable bool) {
	t.Helper()
	if ServiceErrorCode(err) != code {
		t.Fatalf("expected service error code %q, got %q (%v)", code, ServiceErrorCode(err), err)
	}
	desc := DescribeServiceError(err)
	if desc.Kind != kind || desc.Retryable != retryable {
		t.Fatalf("expected descriptor kind=%q retryable=%v, got %+v", kind, retryable, desc)
	}
}

type faultingSessionStore struct {
	inner     *agentruntime.MemorySessionStore
	putErr    error
	getErr    error
	deleteErr error
}

func (s *faultingSessionStore) Put(ctx context.Context, checkpointID string, session *agentruntime.RuntimeSession) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.inner.Put(ctx, checkpointID, session)
}

func (s *faultingSessionStore) Get(ctx context.Context, checkpointID string) (*agentruntime.RuntimeSession, bool, error) {
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	return s.inner.Get(ctx, checkpointID)
}

func (s *faultingSessionStore) Delete(ctx context.Context, checkpointID string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.inner.Delete(ctx, checkpointID)
}
