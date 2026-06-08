package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestApprovalLifecycle_RejectedResumeProducesDegradeEvent(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "approval lifecycle rejected flow",
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval initial outcome, got %+v", initial.Outcome)
	}

	rejected, err := service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "reject lifecycle",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	if rejected.Outcome.Status != RunStatusDegraded {
		t.Fatalf("expected degraded outcome after rejection, got %+v", rejected.Outcome)
	}
	if rejected.Outcome.Interrupted || strings.TrimSpace(rejected.Outcome.InterruptReason) != "" {
		t.Fatalf("expected rejection to clear interrupted execution state, got %+v", rejected.Outcome)
	}
	if rejected.Response.DegradeReason != "approval_rejected" {
		t.Fatalf("expected approval_rejected degrade reason, got %+v", rejected.Response)
	}
	if !hasRuntimeEventType(rejected.Journal, agentstate.EventTypeDegraded) {
		t.Fatalf("expected rejected lifecycle journal to contain a degraded event, got %+v", rejected.Journal)
	}
	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestApprovalLifecycle_AwaitingApprovalOutcomeReflectsResumeLineage(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	session := newRuntimeSession(Request{
		Question: "approval lineage projection",
	}, 2, agentstate.OutputModeFinalAnswer, runtimeNameForPattern(PatternReactive))
	session.SessionID = "approval-lineage-session"
	session.Metadata.ResumeCount = 1
	session.Metadata.ResumedFrom = "cp-previous-approval"
	session.Snapshot.Execution.Interrupted = true
	session.Snapshot.Execution.CurrentNode = "fetch"
	session.Snapshot.Context.SearchQuery = "approval lineage projection"
	session.Snapshot.Context.PreferredURLs = []string{"https://approval-lineage.example/doc"}
	session.Snapshot.Approval = agentstate.ApprovalState{
		Status:       agentstate.ApprovalStatusPending,
		Reason:       "fetch_approval_required",
		Node:         "fetch",
		RerunNode:    "fetch",
		Capability:   agentcapability.NameWebFetch,
		CheckpointID: "cp-current-approval",
		RequestedAt:  time.Now(),
	}

	outcome := service.outcomeFromSession(session)
	if outcome.Status != RunStatusAwaitingApproval || outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome projection, got %+v", outcome)
	}
	if outcome.Approval.ResumeCount != 1 {
		t.Fatalf("expected approval payload to expose prior resume count, got %+v", outcome.Approval)
	}
	if outcome.Approval.SessionID != "approval-lineage-session" || outcome.CheckpointID != "cp-current-approval" {
		t.Fatalf("expected approval payload to preserve session/checkpoint lineage, got %+v", outcome)
	}
	if outcome.Approval.RerunNode != "fetch" || outcome.Approval.CapabilityName != agentcapability.NameWebFetch {
		t.Fatalf("expected approval payload to preserve rerun capability context, got %+v", outcome.Approval)
	}
	if outcome.InterruptReason != "fetch_approval_required" {
		t.Fatalf("expected awaiting approval outcome to preserve interrupt reason, got %+v", outcome)
	}
}

func TestApprovalLifecycle_HandoffRejectMatchesFinalAnswerSemantics(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff approval reject lifecycle",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", initial.Outcome)
	}

	rejected, err := service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
		DecisionNote: "handoff rejected",
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval() error = %v", err)
	}

	if rejected.Outcome.Status != RunStatusDegraded {
		t.Fatalf("expected degraded handoff outcome after rejection, got %+v", rejected.Outcome)
	}
	if rejected.Outcome.Interrupted || strings.TrimSpace(rejected.Outcome.CheckpointID) != "" || rejected.Outcome.Approval != nil {
		t.Fatalf("expected handoff reject to use same terminal approval semantics as final-answer, got %+v", rejected.Outcome)
	}
	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestApprovalLifecycle_HandoffDuplicateResumeAfterApprovalReturnsNotFound(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff duplicate approval lifecycle",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval(first) error = %v", err)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected duplicate handoff resume to fail")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionNotFound, ErrorKindNotFound, false)
}

func TestApprovalLifecycle_HandoffApprovedResumeRecordsAuditMetadataAndClearsPendingState(t *testing.T) {
	store := newRecordingSessionStore()
	service := newContractTestServiceWithStore(t, true, store)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff approval audit lifecycle",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
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
		DecisionNote: "approved for handoff audit",
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval() error = %v", err)
	}
	if resumed.Outcome.Status != RunStatusCompleted {
		t.Fatalf("expected completed handoff outcome after approval resume, got %+v", resumed.Outcome)
	}

	stored, ok := store.lastPut(initial.Outcome.CheckpointID)
	if !ok || stored == nil {
		t.Fatalf("expected approval decision snapshot to be recorded, got ok=%v session=%+v", ok, stored)
	}
	if stored.Snapshot.Approval.Status != agentstate.ApprovalStatusApproved || stored.Snapshot.Approval.ReviewedAt.IsZero() {
		t.Fatalf("expected approved handoff approval snapshot with reviewed time, got %+v", stored.Snapshot.Approval)
	}
	if stored.Snapshot.Approval.DecisionNote != "approved for handoff audit" {
		t.Fatalf("expected handoff approval decision note in snapshot, got %+v", stored.Snapshot.Approval)
	}
	if stored.Metadata.ApprovalDecision != agentstate.ApprovalStatusApproved || stored.Metadata.ApprovalNote != "approved for handoff audit" {
		t.Fatalf("expected handoff approval metadata to be recorded, got %+v", stored.Metadata)
	}
	assertPendingSessionMissing(t, service, initial.Outcome.CheckpointID, initial.Outcome.Approval.SessionID)
}

func TestApprovalLifecycle_HandoffResumeAfterRejectDuplicateReturnsNotFound(t *testing.T) {
	service := newContractTestService(t, true)

	initial, err := service.RunHandoffDetailed(context.Background(), Request{
		Question: "handoff duplicate reject lifecycle",
		Options: RequestOptions{
			OutputMode: agentstate.OutputModeHandoff,
		},
	})
	if err != nil {
		t.Fatalf("RunHandoffDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval handoff outcome, got %+v", initial.Outcome)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
	})
	if err != nil {
		t.Fatalf("ResumeHandoffAfterApproval(first reject) error = %v", err)
	}

	_, err = service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionRejected,
	})
	if err == nil {
		t.Fatal("expected duplicate handoff reject resume to fail")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalSessionNotFound, ErrorKindNotFound, false)
}

func TestApprovalLifecycle_HandoffResumeWhenApprovalNotPendingReturnsFailedPrecondition(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))
	session := newRuntimeSession(Request{
		Question: "handoff approval already reviewed",
	}, 2, agentstate.OutputModeHandoff, runtimeNameForPattern(PatternReactive))
	session.Snapshot.Approval.Status = agentstate.ApprovalStatusApproved
	session.Snapshot.Approval.CheckpointID = "cp-handoff-not-pending"
	if err := service.sessionStore.Put(context.Background(), "cp-handoff-not-pending", session); err != nil {
		t.Fatalf("sessionStore.Put() error = %v", err)
	}

	_, err := service.ResumeHandoffAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: "cp-handoff-not-pending",
		Decision:     ApprovalDecisionApproved,
	})
	if err == nil {
		t.Fatal("expected handoff approval not pending error")
	}
	assertServiceErrorDescriptor(t, err, ErrorCodeApprovalNotPending, ErrorKindFailedPrecondition, false)
}

func hasRuntimeEventType(events []agentstate.RuntimeEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func newContractTestServiceWithStore(t *testing.T, requireApproval bool, store agentruntime.SessionStore) *Service {
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

	return newTestAgentServiceWithPatternAndStore(t, PatternReactive, searchHandle, fetchHandle, store)
}
