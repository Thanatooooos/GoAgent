package agent

import (
	"context"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestServiceRunDetailed_RuntimeApprovalThenResume(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "runtime approval flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Restricted",
					URL:     "https://restricted.example/doc",
					Snippet: "needs approval",
					Domain:  "restricted.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if len(urls) != 1 || urls[0] != "https://restricted.example/doc" {
				t.Fatalf("unexpected urls: %v", urls)
			}
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "fetched approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "approved readable evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	result, err := service.RunDetailed(context.Background(), Request{
		Question: "runtime approval flow",
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusAwaitingApproval || result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}
	if result.Outcome.Approval.Node != "approval" || result.Outcome.Approval.Capability != agentcapability.NameWebFetch {
		t.Fatalf("expected runtime approval to stop at approval gate for web_fetch, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.Status != agentstate.ApprovalStatusPending ||
		result.Outcome.Approval.ReasonCode != "fetch_approval_required" ||
		result.Outcome.Approval.Trigger != "capability_permission_error" {
		t.Fatalf("expected enriched approval state for runtime approval, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.CapabilityKind != agentcapability.KindTool ||
		result.Outcome.Approval.CapabilityFamily != agentcapability.FamilyExternalEvidence ||
		result.Outcome.Approval.RiskLevel != agentcapability.RiskLevelMedium {
		t.Fatalf("expected capability metadata in approval outcome, got %+v", result.Outcome.Approval)
	}
	if !result.Outcome.Approval.SupportsParallel ||
		result.Outcome.Approval.Idempotency != agentcapability.IdempotencyBestEffort ||
		result.Outcome.Approval.SupportsResume {
		t.Fatalf("expected scheduler policy metadata in approval outcome, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.SearchQuery != "runtime approval flow" ||
		len(result.Outcome.Approval.CandidateURLs) != 1 ||
		result.Outcome.Approval.CandidateURLs[0] != "https://restricted.example/doc" {
		t.Fatalf("expected approval context for runtime approval, got %+v", result.Outcome.Approval)
	}
	if !result.Outcome.Approval.CanApprove || !result.Outcome.Approval.CanReject || result.Outcome.Approval.RejectOutcome != RunStatusDegraded {
		t.Fatalf("expected approval actions metadata, got %+v", result.Outcome.Approval)
	}

	resumed, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: result.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for retry",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome after approval resume, got %+v", resumed.Outcome)
	}
	if !strings.Contains(resumed.Response.Summary, "approved readable evidence") {
		t.Fatalf("expected resumed response to use approved evidence, got %+v", resumed.Response)
	}
}

func TestServiceRunDetailed_CapabilityApprovalGateThenResume(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{
				{
					Title:   "Gated",
					URL:     "https://gated.example/doc",
					Snippet: "needs capability approval",
					Domain:  "gated.example",
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
				Summary: "fetched gated content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "capability approval content"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	result, err := service.RunDetailed(context.Background(), Request{
		Question: "capability approval flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusAwaitingApproval || result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}
	if result.Outcome.Approval.Node != "approval" || result.Outcome.Approval.Capability != agentcapability.NameWebFetch {
		t.Fatalf("expected capability approval to stop before fetch, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.Trigger != "interrupt_before_node" ||
		result.Outcome.Approval.RerunNode != "fetch" ||
		result.Outcome.Approval.SearchQuery != "capability approval flow" {
		t.Fatalf("expected capability approval context, got %+v", result.Outcome.Approval)
	}
	if len(result.Outcome.Approval.CandidateURLs) != 1 || result.Outcome.Approval.CandidateURLs[0] != "https://gated.example/doc" {
		t.Fatalf("expected candidate urls in approval outcome, got %+v", result.Outcome.Approval)
	}

	resumed, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: result.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome after capability approval resume, got %+v", resumed.Outcome)
	}
	if !strings.Contains(resumed.Response.Summary, "capability approval content") {
		t.Fatalf("expected resumed response to use fetch output, got %+v", resumed.Response)
	}
}

func TestServiceResumeAfterApproval_RejectsCapabilityApproval(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{
				{
					Title:   "Gated",
					URL:     "https://reject.example/doc",
					Snippet: "needs capability approval",
					Domain:  "reject.example",
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
				Summary: "should not be reached",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "should not be returned"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	result, err := service.RunDetailed(context.Background(), Request{
		Question: "capability approval reject flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusAwaitingApproval || result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}

	rejected, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: result.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "not approved",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if rejected.Outcome.Status != RunStatusDegraded {
		t.Fatalf("expected degraded outcome after rejection, got %+v", rejected.Outcome)
	}
	if rejected.Response.DegradeReason != "approval_rejected" || !strings.Contains(rejected.Response.Summary, "required approval was not granted") {
		t.Fatalf("expected reject path to degrade with approval_rejected, got %+v", rejected.Response)
	}
	assertPendingSessionMissing(t, service, result.Outcome.CheckpointID, result.Outcome.Approval.SessionID)
}

func TestServiceResumeHandoffAfterApproval_CompletesAndClearsPendingSession(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "handoff approval flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Restricted Handoff",
					URL:     "https://handoff.example/doc",
					Snippet: "needs approval",
					Domain:  "handoff.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if len(urls) != 1 || urls[0] != "https://handoff.example/doc" {
				t.Fatalf("unexpected urls: %v", urls)
			}
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "handoff approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "approved handoff evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff approval flow",
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", initial.Outcome)
	}

	resumed, err := service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for handoff resume",
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed handoff outcome after approval resume, got %+v", resumed.Outcome)
	}
	if resumed.Handoff.EvidenceBundle.Question != "handoff approval flow" {
		t.Fatalf("expected handoff evidence bundle to preserve question, got %+v", resumed.Handoff)
	}
	if len(resumed.Handoff.EvidenceBundle.AcceptedEvidence) != 1 || !strings.Contains(resumed.Handoff.EvidenceBundle.AcceptedEvidence[0].Content, "approved handoff evidence") {
		t.Fatalf("expected resumed handoff to contain approved accepted evidence, got %+v", resumed.Handoff.EvidenceBundle)
	}
	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestServiceResumeAfterApproval_ApprovedSessionUpdatesMetadataAndClearsPendingState(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "approval metadata flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Metadata",
					URL:     "https://metadata.example/doc",
					Snippet: "needs approval",
					Domain:  "metadata.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if len(urls) != 1 || urls[0] != "https://metadata.example/doc" {
				t.Fatalf("unexpected urls: %v", urls)
			}
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "metadata approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "metadata approved evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	result, err := service.RunDetailed(context.Background(), Request{
		Question: "approval metadata flow",
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if result.Outcome.Status != RunStatusAwaitingApproval || result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}

	stored, ok, err := service.sessionStore.Get(context.Background(), result.Outcome.CheckpointID)
	if err != nil {
		t.Fatalf("sessionStore.Get() error = %v", err)
	}
	if !ok || stored == nil {
		t.Fatalf("expected stored pending session, got ok=%v session=%+v", ok, stored)
	}
	if stored.Snapshot.Approval.RequestedAt.IsZero() {
		t.Fatalf("expected pending approval session to record requested time, got %+v", stored.Snapshot.Approval)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: result.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for metadata test",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	storedByCheckpoint, ok, err := service.sessionStore.Get(context.Background(), result.Outcome.CheckpointID)
	if err != nil {
		t.Fatalf("sessionStore.Get(checkpoint) error = %v", err)
	}
	if ok || storedByCheckpoint != nil {
		t.Fatalf("expected checkpoint session to be cleared, got ok=%v session=%+v", ok, storedByCheckpoint)
	}
	storedBySessionID, ok, err := service.sessionStore.Get(context.Background(), result.Outcome.Approval.SessionID)
	if err != nil {
		t.Fatalf("sessionStore.Get(sessionID) error = %v", err)
	}
	if ok || storedBySessionID != nil {
		t.Fatalf("expected session id entry to be cleared, got ok=%v session=%+v", ok, storedBySessionID)
	}
}

func TestServiceResumeAfterApproval_RejectsInvalidDecision(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-invalid-decision",
		Decision:     "later",
	})
	if err == nil {
		t.Fatal("expected invalid decision error")
	}
	if ServiceErrorCode(err) != ErrorCodeApprovalDecisionInvalid {
		t.Fatalf("expected invalid decision error code, got %q (%v)", ServiceErrorCode(err), err)
	}
	detail := DescribeServiceError(err)
	if detail.Kind != ErrorKindInvalidRequest || detail.Retryable {
		t.Fatalf("expected invalid decision descriptor, got %+v", detail)
	}
}

func TestServiceResumeAfterApproval_ReturnsSessionNotFound(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-missing-session",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected missing session error")
	}
	if ServiceErrorCode(err) != ErrorCodeApprovalSessionNotFound {
		t.Fatalf("expected missing session error code, got %q (%v)", ServiceErrorCode(err), err)
	}
	detail := DescribeServiceError(err)
	if detail.Kind != ErrorKindNotFound || detail.Retryable {
		t.Fatalf("expected missing session descriptor, got %+v", detail)
	}
}

func TestServiceResumeAfterApproval_ReturnsApprovalNotPending(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	session := newRuntimeSession(Request{
		Question: "already reviewed approval",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.Snapshot.Approval.Status = agentstate.ApprovalStatusApproved
	session.Snapshot.Approval.CheckpointID = "cp-not-pending"
	if err := service.sessionStore.Put(context.Background(), "cp-not-pending", session); err != nil {
		t.Fatalf("sessionStore.Put() error = %v", err)
	}

	_, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-not-pending",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected approval not pending error")
	}
	if ServiceErrorCode(err) != ErrorCodeApprovalNotPending {
		t.Fatalf("expected approval not pending error code, got %q (%v)", ServiceErrorCode(err), err)
	}
	detail := DescribeServiceError(err)
	if detail.Kind != ErrorKindFailedPrecondition || detail.Retryable {
		t.Fatalf("expected approval not pending descriptor, got %+v", detail)
	}
}

func TestServiceRunDetailed_ReturnsQuestionRequired(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	_, err := service.RunDetailed(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected question required error")
	}
	if ServiceErrorCode(err) != ErrorCodeQuestionRequired {
		t.Fatalf("expected question required error code, got %q (%v)", ServiceErrorCode(err), err)
	}
	detail := DescribeServiceError(err)
	if detail.Kind != ErrorKindInvalidRequest || detail.Retryable {
		t.Fatalf("expected question required descriptor, got %+v", detail)
	}
}

func TestServiceResumeAfterApproval_StoresApprovalAuditMetadataOnApprove(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "approval audit approve flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Audit Approve",
					URL:     "https://audit-approve.example/doc",
					Snippet: "needs approval",
					Domain:  "audit-approve.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if len(urls) != 1 || urls[0] != "https://audit-approve.example/doc" {
				t.Fatalf("unexpected urls: %v", urls)
			}
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "audit approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "audit approved evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	store := newRecordingSessionStore()
	service := newTestAgentServiceWithPatternAndStore(t, PatternReactive, searchHandle, fetchHandle, store)
	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "approval audit approve flow",
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for audit",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	stored, ok := store.lastPut(initial.Outcome.CheckpointID)
	if !ok || stored == nil {
		t.Fatalf("expected approval decision snapshot to be recorded, got ok=%v session=%+v", ok, stored)
	}
	if stored.Snapshot.Approval.Status != agentstate.ApprovalStatusApproved || stored.Snapshot.Approval.ReviewedAt.IsZero() {
		t.Fatalf("expected approved approval snapshot with reviewed time, got %+v", stored.Snapshot.Approval)
	}
	if stored.Snapshot.Approval.DecisionNote != "approved for audit" {
		t.Fatalf("expected approval decision note in snapshot, got %+v", stored.Snapshot.Approval)
	}
	if stored.Metadata.ApprovalDecision != agentstate.ApprovalStatusApproved || stored.Metadata.ApprovalNote != "approved for audit" {
		t.Fatalf("expected approval metadata to be recorded, got %+v", stored.Metadata)
	}
	if stored.Snapshot.Approval.RequestedAt.IsZero() {
		t.Fatalf("expected requested time to remain set, got %+v", stored.Snapshot.Approval)
	}
}

func TestServiceResumeAfterApproval_StoresApprovalAuditMetadataOnReject(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{
				{
					Title:   "Audit Reject",
					URL:     "https://audit-reject.example/doc",
					Snippet: "needs approval",
					Domain:  "audit-reject.example",
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
				Summary: "should not be reached",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "should not be returned"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(true))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	store := newRecordingSessionStore()
	service := newTestAgentServiceWithPatternAndStore(t, PatternReactive, searchHandle, fetchHandle, store)
	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "approval audit reject flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "rejected for audit",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	stored, ok := store.lastPut(initial.Outcome.CheckpointID)
	if !ok || stored == nil {
		t.Fatalf("expected rejection decision snapshot to be recorded, got ok=%v session=%+v", ok, stored)
	}
	if stored.Snapshot.Approval.Status != agentstate.ApprovalStatusRejected || stored.Snapshot.Approval.ReviewedAt.IsZero() {
		t.Fatalf("expected rejected approval snapshot with reviewed time, got %+v", stored.Snapshot.Approval)
	}
	if stored.Snapshot.Approval.DecisionNote != "rejected for audit" {
		t.Fatalf("expected rejection decision note in snapshot, got %+v", stored.Snapshot.Approval)
	}
	if stored.Metadata.ApprovalDecision != agentstate.ApprovalStatusRejected || stored.Metadata.ApprovalNote != "rejected for audit" {
		t.Fatalf("expected rejection metadata to be recorded, got %+v", stored.Metadata)
	}
}

func TestServiceResumeAfterApproval_IncrementsResumeCountOnRunnerResume(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			if query != "approval resume count flow" {
				t.Fatalf("unexpected query: %q", query)
			}
			return []searchprovider.SearchResult{
				{
					Title:   "Approval Resume Count",
					URL:     "https://resume-count.example/doc",
					Snippet: "needs approval",
					Domain:  "resume-count.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if len(urls) != 1 || urls[0] != "https://resume-count.example/doc" {
				t.Fatalf("unexpected urls: %v", urls)
			}
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "resume count approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "resume count approved evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "approval resume count flow",
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected initial awaiting approval outcome, got %+v", initial.Outcome)
	}

	pendingSession, ok, err := service.sessionStore.Get(context.Background(), initial.Outcome.CheckpointID)
	if err != nil {
		t.Fatalf("sessionStore.Get() error = %v", err)
	}
	if !ok || pendingSession == nil {
		t.Fatalf("expected pending session to be stored, got ok=%v session=%+v", ok, pendingSession)
	}
	decision, err := resolveApprovalResumeDecision(ResumeApprovalRequest{
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for resume count",
	})
	if err != nil {
		t.Fatalf("resolveApprovalResumeDecision() error = %v", err)
	}
	if err := service.applyApprovalDecision(pendingSession, initial.Outcome.CheckpointID, ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for resume count",
	}, decision); err != nil {
		t.Fatalf("applyApprovalDecision() error = %v", err)
	}
	if err := service.sessionStore.Put(context.Background(), initial.Outcome.CheckpointID, pendingSession); err != nil {
		t.Fatalf("sessionStore.Put() error = %v", err)
	}
	if strings.TrimSpace(pendingSession.SessionID) != "" {
		if err := service.sessionStore.Put(context.Background(), pendingSession.SessionID, pendingSession); err != nil {
			t.Fatalf("sessionStore.Put(sessionID) error = %v", err)
		}
	}

	finalSession, err := service.kernelRunner.Resume(context.Background(), pendingSession, initial.Outcome.CheckpointID)
	if err != nil {
		t.Fatalf("runner.Resume() error = %v", err)
	}
	if finalSession.Metadata.ResumeCount != 1 || finalSession.Metadata.ResumedFrom != initial.Outcome.CheckpointID {
		t.Fatalf("expected resume metadata to increment and record checkpoint, got %+v", finalSession.Metadata)
	}
}
