package agent

import (
	"context"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestRunDetailedCompletedOutcomeContract(t *testing.T) {
	service := newContractTestService(t, false)

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "completed contract flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}

	if result.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", result.Outcome)
	}
	if result.Outcome.Interrupted {
		t.Fatalf("expected completed outcome to be non-interrupted, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.InterruptReason) != "" {
		t.Fatalf("expected completed outcome interrupt reason to be empty, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.CheckpointID) != "" {
		t.Fatalf("expected completed outcome checkpoint to be empty, got %+v", result.Outcome)
	}
	if result.Outcome.Approval != nil {
		t.Fatalf("expected completed outcome approval to be nil, got %+v", result.Outcome)
	}
	if result.Response.Degraded || strings.TrimSpace(result.Response.DegradeReason) != "" {
		t.Fatalf("expected non-degraded response, got %+v", result.Response)
	}
	if !strings.Contains(result.Response.Summary, "completed contract evidence") {
		t.Fatalf("expected summary to use completed evidence, got %+v", result.Response)
	}
}

func TestRunDetailedAwaitingApprovalOutcomeContract(t *testing.T) {
	service := newContractTestService(t, true)

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "approval contract flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}

	if result.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval outcome, got %+v", result.Outcome)
	}
	if !result.Outcome.Interrupted {
		t.Fatalf("expected awaiting approval outcome to remain interrupted, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.CheckpointID) == "" {
		t.Fatalf("expected awaiting approval checkpoint to be populated, got %+v", result.Outcome)
	}
	if result.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval payload, got %+v", result.Outcome)
	}
	if !result.Outcome.Approval.Required {
		t.Fatalf("expected approval payload to mark required=true, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.Status != agentstate.ApprovalStatusPending {
		t.Fatalf("expected pending approval status, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.Reason != result.Outcome.Approval.ReasonCode {
		t.Fatalf("expected reason alias to match reason code, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.Capability != result.Outcome.Approval.CapabilityName {
		t.Fatalf("expected capability alias to match capability name, got %+v", result.Outcome.Approval)
	}
	if result.Outcome.Approval.RejectOutcome != RunStatusDegraded {
		t.Fatalf("expected reject outcome to degrade, got %+v", result.Outcome.Approval)
	}
	if strings.TrimSpace(result.Outcome.InterruptReason) == "" {
		t.Fatalf("expected awaiting approval outcome to expose an interrupt reason, got %+v", result.Outcome)
	}
}

func TestRunDetailedDegradedOutcomeContract(t *testing.T) {
	service := newDegradedContractTestService(t)

	result, err := service.RunDetailed(context.Background(), Request{
		Question: "degraded contract flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}

	if result.Outcome.Status != RunStatusDegraded {
		t.Fatalf("expected degraded outcome, got %+v", result.Outcome)
	}
	if result.Outcome.Interrupted {
		t.Fatalf("expected degraded outcome to clear interrupted state, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.InterruptReason) != "" {
		t.Fatalf("expected degraded outcome interrupt reason to be empty, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.CheckpointID) != "" || result.Outcome.Approval != nil {
		t.Fatalf("expected degraded outcome to expose no resume state, got %+v", result.Outcome)
	}
	if !result.Response.Degraded || strings.TrimSpace(result.Response.DegradeReason) == "" {
		t.Fatalf("expected degraded response projection, got %+v", result.Response)
	}
	if strings.TrimSpace(result.Response.Summary) == "" {
		t.Fatalf("expected degraded summary to remain caller-readable, got %+v", result.Response)
	}
}

func TestResumeAfterApprovalRejectedOutcomeContract(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "approval reject contract flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected initial awaiting approval outcome, got %+v", initial.Outcome)
	}

	rejected, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "contract reject",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	if rejected.Outcome.Status != RunStatusDegraded {
		t.Fatalf("expected degraded outcome after rejection, got %+v", rejected.Outcome)
	}
	if rejected.Outcome.Interrupted {
		t.Fatalf("expected rejected outcome to clear interrupted state, got %+v", rejected.Outcome)
	}
	if strings.TrimSpace(rejected.Outcome.CheckpointID) != "" {
		t.Fatalf("expected rejected outcome checkpoint to be empty, got %+v", rejected.Outcome)
	}
	if rejected.Outcome.Approval != nil {
		t.Fatalf("expected rejected outcome approval to be nil, got %+v", rejected.Outcome)
	}
	if !rejected.Response.Degraded || rejected.Response.DegradeReason != "approval_rejected" {
		t.Fatalf("expected rejected response to expose degrade state, got %+v", rejected.Response)
	}
	if !strings.Contains(rejected.Response.Summary, "required approval was not granted") {
		t.Fatalf("expected rejected summary to explain approval failure, got %+v", rejected.Response)
	}
}

func TestRunHandoffDetailedAwaitingApprovalOutcomeContract(t *testing.T) {
	service := newContractTestService(t, true)

	result, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff approval contract flow",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}

	if result.Outcome.Status != RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", result.Outcome)
	}
	if strings.TrimSpace(result.Outcome.CheckpointID) == "" || result.Outcome.Approval == nil {
		t.Fatalf("expected handoff approval payload and checkpoint, got %+v", result.Outcome)
	}
	if result.Outcome.Approval.Capability != result.Outcome.Approval.CapabilityName {
		t.Fatalf("expected handoff approval capability alias to remain aligned, got %+v", result.Outcome.Approval)
	}
}

func TestResolveApprovalResumeDecisionSupportsBooleanCompatibility(t *testing.T) {
	approved, err := resolveApprovalResumeDecision(ResumeApprovalRequest{Approved: true})
	if err != nil {
		t.Fatalf("resolveApprovalResumeDecision(approved bool) error = %v", err)
	}
	if !approved.approved || approved.value != agentstate.ApprovalStatusApproved {
		t.Fatalf("expected approved bool fallback to resolve approved, got %+v", approved)
	}

	rejected, err := resolveApprovalResumeDecision(ResumeApprovalRequest{Approved: false})
	if err != nil {
		t.Fatalf("resolveApprovalResumeDecision(rejected bool) error = %v", err)
	}
	if rejected.approved || rejected.value != agentstate.ApprovalStatusRejected {
		t.Fatalf("expected false bool fallback to resolve rejected, got %+v", rejected)
	}
}

func TestResolveApprovalResumeDecisionPrefersCanonicalDecisionOverBooleanCompatibility(t *testing.T) {
	decision, err := resolveApprovalResumeDecision(ResumeApprovalRequest{
		Decision: ApprovalDecisionRejected,
		Approved: true,
	})
	if err != nil {
		t.Fatalf("resolveApprovalResumeDecision() error = %v", err)
	}
	if decision.approved || decision.value != agentstate.ApprovalStatusRejected {
		t.Fatalf("expected explicit decision to win over compatibility bool, got %+v", decision)
	}
}

func TestNewRuntimeSession_SeedsToolStageContext(t *testing.T) {
	session := newRuntimeSession(Request{
		Question: "why did doc fail",
		UserID:   "u1",
		TraceID:  "trace-1",
		ToolStage: &ToolStageContext{
			ConversationID:    "conv-1",
			KnowledgeBaseIDs:  []string{"kb-1", "kb-2"},
			RewrittenQuestion: "why did doc fail in indexer",
			SubQuestions:      []string{"indexer failure", "vector store health"},
			NeedRetrieval:     true,
			KnowledgeContext:  "indexer failed because vector store refused connection",
			SearchChannels:    []string{"vector_global", "keyword"},
			HistorySummary:    "user: doc_fail_01 failed || assistant: checking",
		},
	}, 2, "final_answer", "agent_runtime_test")

	if session.Snapshot.Request.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id seed, got %+v", session.Snapshot.Request)
	}
	if len(session.Snapshot.Request.KnowledgeBaseIDs) != 2 {
		t.Fatalf("expected knowledge base ids seed, got %+v", session.Snapshot.Request)
	}
	if session.Snapshot.Context.RewrittenQuery != "why did doc fail in indexer" {
		t.Fatalf("expected rewritten query seed, got %+v", session.Snapshot.Context)
	}
	if session.Snapshot.Context.SearchQuery != "why did doc fail in indexer" {
		t.Fatalf("expected search query to prefer rewritten query, got %+v", session.Snapshot.Context)
	}
	if len(session.Snapshot.Context.Notes) == 0 {
		t.Fatalf("expected tool-stage notes to be seeded, got %+v", session.Snapshot.Context)
	}
}

func TestStorePendingSessionStoresCheckpointAndSessionAliases(t *testing.T) {
	store := newRecordingSessionStore()
	service := &Service{sessionStore: store}
	session := newRuntimeSession(Request{
		Question: "store alias contract",
		TraceID:  "trace-store-alias",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.SessionID = "session-store-alias"

	if err := service.storePendingSession(context.Background(), "checkpoint-store-alias", session); err != nil {
		t.Fatalf("storePendingSession() error = %v", err)
	}

	storedByCheckpoint, ok, err := store.Get(context.Background(), "checkpoint-store-alias")
	if err != nil {
		t.Fatalf("sessionStore.Get(checkpoint) error = %v", err)
	}
	if !ok || storedByCheckpoint == nil {
		t.Fatalf("expected checkpoint alias to be persisted, got ok=%v session=%+v", ok, storedByCheckpoint)
	}

	storedBySessionID, ok, err := store.Get(context.Background(), "session-store-alias")
	if err != nil {
		t.Fatalf("sessionStore.Get(sessionID) error = %v", err)
	}
	if !ok || storedBySessionID == nil {
		t.Fatalf("expected session alias to be persisted, got ok=%v session=%+v", ok, storedBySessionID)
	}
}

func TestStorePendingSessionSkipsDuplicateAliasWhenSessionIDMatchesCheckpointID(t *testing.T) {
	store := newRecordingSessionStore()
	service := &Service{sessionStore: store}
	session := newRuntimeSession(Request{
		Question: "store same alias contract",
		TraceID:  "checkpoint-same-alias",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.SessionID = "checkpoint-same-alias"

	if err := service.storePendingSession(context.Background(), "checkpoint-same-alias", session); err != nil {
		t.Fatalf("storePendingSession() error = %v", err)
	}

	if len(store.puts) != 1 {
		t.Fatalf("expected exactly one store operation when session id matches checkpoint id, got %d puts", len(store.puts))
	}
	if store.puts[0].key != "checkpoint-same-alias" {
		t.Fatalf("expected single put to use checkpoint alias, got %+v", store.puts)
	}
}

func TestDeletePendingSessionRemovesCheckpointAndSessionAliases(t *testing.T) {
	store := newRecordingSessionStore()
	service := &Service{sessionStore: store}
	session := newRuntimeSession(Request{
		Question: "delete alias contract",
		TraceID:  "trace-delete-alias",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.SessionID = "session-delete-alias"

	if err := service.storePendingSession(context.Background(), "checkpoint-delete-alias", session); err != nil {
		t.Fatalf("storePendingSession() error = %v", err)
	}
	if err := service.deletePendingSession(context.Background(), "checkpoint-delete-alias"); err != nil {
		t.Fatalf("deletePendingSession() error = %v", err)
	}

	assertPendingSessionMissing(t, service, "checkpoint-delete-alias", "session-delete-alias")
	if len(store.deletes) != 2 {
		t.Fatalf("expected checkpoint and session alias deletes, got %+v", store.deletes)
	}
	if store.deletes[0] != "checkpoint-delete-alias" || store.deletes[1] != "session-delete-alias" {
		t.Fatalf("expected delete order checkpoint then session alias, got %+v", store.deletes)
	}
}

func TestDeletePendingSessionRemovesOnlyCheckpointAliasWhenSessionIDMatches(t *testing.T) {
	store := newRecordingSessionStore()
	service := &Service{sessionStore: store}
	session := newRuntimeSession(Request{
		Question: "delete same alias contract",
		TraceID:  "checkpoint-delete-same-alias",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.SessionID = "checkpoint-delete-same-alias"

	if err := store.Put(context.Background(), "checkpoint-delete-same-alias", session); err != nil {
		t.Fatalf("sessionStore.Put() error = %v", err)
	}
	if err := service.deletePendingSession(context.Background(), "checkpoint-delete-same-alias"); err != nil {
		t.Fatalf("deletePendingSession() error = %v", err)
	}

	stored, ok, err := store.Get(context.Background(), "checkpoint-delete-same-alias")
	if err != nil {
		t.Fatalf("sessionStore.Get() error = %v", err)
	}
	if ok || stored != nil {
		t.Fatalf("expected checkpoint alias to be deleted, got ok=%v session=%+v", ok, stored)
	}
	if len(store.deletes) != 1 || store.deletes[0] != "checkpoint-delete-same-alias" {
		t.Fatalf("expected only checkpoint alias delete, got %+v", store.deletes)
	}
}

func TestMemorySessionStoreSupportsAliasLookupIndependently(t *testing.T) {
	store := agentruntime.NewMemorySessionStore()
	session := &agentruntime.RuntimeSession{SessionID: "session-memory-alias"}

	if err := store.Put(context.Background(), "checkpoint-memory-alias", session); err != nil {
		t.Fatalf("Put(checkpoint) error = %v", err)
	}
	if err := store.Put(context.Background(), "session-memory-alias", session); err != nil {
		t.Fatalf("Put(sessionID) error = %v", err)
	}

	storedByCheckpoint, ok, err := store.Get(context.Background(), "checkpoint-memory-alias")
	if err != nil {
		t.Fatalf("Get(checkpoint) error = %v", err)
	}
	if !ok || storedByCheckpoint == nil {
		t.Fatalf("expected checkpoint alias to resolve, got ok=%v session=%+v", ok, storedByCheckpoint)
	}

	storedBySessionID, ok, err := store.Get(context.Background(), "session-memory-alias")
	if err != nil {
		t.Fatalf("Get(sessionID) error = %v", err)
	}
	if !ok || storedBySessionID == nil {
		t.Fatalf("expected session alias to resolve independently, got ok=%v session=%+v", ok, storedBySessionID)
	}
}

func newContractTestService(t *testing.T, requireApproval bool) *Service {
	t.Helper()

	searchHandle, err := agentsearch.NewCapability(contractSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query:        query,
				Provider:     "contract-provider",
				ResultCount:  1,
				AllowedCount: 1,
				NeutralCount: 0,
				DeniedCount:  0,
				URLs:         []string{"https://contract.example/doc"},
				Results: []agentsearch.SearchResultItem{
					{
						Title:   "Contract Evidence",
						URL:     "https://contract.example/doc",
						Snippet: "contract evidence for " + query,
						Domain:  "contract.example",
					},
				},
				Summary: "contract search evidence",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}

	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary: "completed contract evidence",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "completed contract evidence"},
				},
			}, nil
		},
	}, agentcapability.WithRequiresApproval(requireApproval))
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	return newTestAgentService(t, searchHandle, fetchHandle)
}

func newDegradedContractTestService(t *testing.T) *Service {
	t.Helper()

	searchHandle, err := agentsearch.NewCapability(contractSearchInvoker{
		search: func(_ context.Context, query string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{
				Query:        query,
				Provider:     "contract-provider",
				ResultCount:  1,
				AllowedCount: 1,
				URLs:         []string{"https://contract.example/degraded"},
				Results: []agentsearch.SearchResultItem{
					{
						Title:   "Contract Degraded Evidence",
						URL:     "https://contract.example/degraded",
						Snippet: "degraded contract evidence for " + query,
						Domain:  "contract.example",
					},
				},
				Summary: "degraded contract search evidence",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}

	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			return agentfetch.Output{
				Summary:       "degraded contract evidence",
				Degraded:      true,
				DegradeReason: "contract_degraded",
				ErrorMessage:  "synthetic contract degrade",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], ErrorMessage: "contract degraded page"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	return newTestAgentService(t, searchHandle, fetchHandle)
}

type contractSearchInvoker struct {
	search func(context.Context, string) (agentsearch.SearchOutput, error)
}

func (s contractSearchInvoker) Search(ctx context.Context, query string) (agentsearch.SearchOutput, error) {
	return s.search(ctx, query)
}
