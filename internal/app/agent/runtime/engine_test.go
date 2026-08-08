package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

type fakeKernelRunner struct {
	runSession       *RuntimeSession
	runErr           error
	resumeSession    *RuntimeSession
	resumeErr        error
	lastCheckpointID string
	runCalls         int
	resumeCalls      int
}

func (f *fakeKernelRunner) RunWithCheckpoint(_ context.Context, session *RuntimeSession, checkpointID string, _ ...compose.Option) (*RuntimeSession, error) {
	f.runCalls++
	f.lastCheckpointID = checkpointID
	if f.runSession != nil {
		return f.runSession, f.runErr
	}
	return session, f.runErr
}

func (f *fakeKernelRunner) Resume(_ context.Context, session *RuntimeSession, checkpointID string, _ ...compose.Option) (*RuntimeSession, error) {
	f.resumeCalls++
	f.lastCheckpointID = checkpointID
	if f.resumeSession != nil {
		return f.resumeSession, f.resumeErr
	}
	return session, f.resumeErr
}

func TestEngineRunWithCheckpoint_MapsPendingApprovalToWaitDecision(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-approval",
		Snapshot:  newInterruptedPendingApprovalSnapshot("cp-engine-approval"),
		Checkpoint: &CheckpointRef{
			ID: "cp-engine-approval",
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
		runErr:     errors.New("interrupt"),
	}

	engine := NewEngine(runner)
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-approval")
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}
	if result.Outcome.Decision != DecisionWaitApproval {
		t.Fatalf("expected wait_approval decision, got %+v", result)
	}
	if result.Outcome.CheckpointID != "cp-engine-approval" {
		t.Fatalf("expected checkpoint id from session, got %+v", result)
	}
	if runner.runCalls != 1 || runner.lastCheckpointID != "cp-engine-approval" {
		t.Fatalf("expected delegated run call, got calls=%d checkpoint=%q", runner.runCalls, runner.lastCheckpointID)
	}
}

func TestEngineRunWithCheckpoint_NormalizesPendingApprovalStateAndEvent(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-approval-normalize",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status: agentstate.ApprovalStatusPending,
				Reason: "fetch_approval_required",
			},
			Execution: agentstate.ExecutionState{
				Interrupted: true,
			},
		},
		Checkpoint: &CheckpointRef{
			ID:   "cp-engine-normalize",
			Node: "approval",
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
		runErr:     errors.New("interrupt"),
	}

	engine := NewEngine(runner)
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-normalize")
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}
	if result.Outcome.Decision != DecisionWaitApproval {
		t.Fatalf("expected wait_approval decision, got %+v", result)
	}
	if session.Snapshot.Approval.CheckpointID != "cp-engine-normalize" {
		t.Fatalf("expected checkpoint id to be normalized into approval state, got %+v", session.Snapshot.Approval)
	}
	if session.Snapshot.Approval.RequestedAt.IsZero() {
		t.Fatalf("expected requested_at to be normalized into approval state, got %+v", session.Snapshot.Approval)
	}
	if runtimeEventCount(session, agentstate.EventTypeApprovalPending, "approval") != 1 {
		t.Fatalf("expected exactly one approval_pending event, got %+v", session.Journal)
	}
}

func TestEngineRunWithCheckpoint_NormalizesLegacyPendingApprovalViaCompatResolver(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-legacy-approval-normalize",
		Snapshot: agentstate.StateSnapshot{
			Execution: agentstate.ExecutionState{
				CurrentNode:     "fetch",
				Interrupted:     true,
				InterruptReason: "fetch_approval_required",
			},
		},
		Checkpoint: &CheckpointRef{
			ID:   "cp-engine-legacy-normalize",
			Node: "fetch",
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
		runErr:     errors.New("interrupt"),
	}

	engine := NewEngine(runner, WithPendingApprovalCompat(func(session *RuntimeSession) (PendingApprovalCompat, bool) {
		if session == nil || session.Snapshot.Execution.CurrentNode != "fetch" {
			return PendingApprovalCompat{}, false
		}
		return PendingApprovalCompat{
			Reason:     "fetch_approval_required",
			Capability: "web_fetch",
			RerunNode:  "fetch",
		}, true
	}))
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-legacy-normalize")
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}
	if result.Outcome.Decision != DecisionWaitApproval {
		t.Fatalf("expected wait_approval decision, got %+v", result)
	}
	if session.Snapshot.Approval.Status != agentstate.ApprovalStatusPending || session.Snapshot.Approval.Capability != "web_fetch" {
		t.Fatalf("expected runtime compat normalization to populate shared approval state, got %+v", session.Snapshot.Approval)
	}
	if runtimeEventCount(session, agentstate.EventTypeStateApplied, "approval") == 0 {
		t.Fatalf("expected legacy compat normalization to append shared state_applied event, got %+v", session.Journal)
	}
}

func TestEngineResume_MapsSuccessToResumeDecision(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-resume",
		Checkpoint: &CheckpointRef{
			ID: "cp-engine-resume",
		},
	}
	runner := &fakeKernelRunner{
		resumeSession: session,
	}

	engine := NewEngine(runner)
	result, err := engine.Resume(context.Background(), session, "cp-engine-resume")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Outcome.Decision != DecisionResume {
		t.Fatalf("expected resume decision, got %+v", result)
	}
	if runner.resumeCalls != 1 || runner.lastCheckpointID != "cp-engine-resume" {
		t.Fatalf("expected delegated resume call, got calls=%d checkpoint=%q", runner.resumeCalls, runner.lastCheckpointID)
	}
}

func TestEngineResume_MergesApprovalHistoryIntoReturnedSession(t *testing.T) {
	now := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-engine-resume-history",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusPending,
				Reason:       "fetch_approval_required",
				CheckpointID: "cp-engine-resume-history",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID:   "sess-engine-resume-history",
				Sequence:    1,
				Node:        "approval",
				EventType:   agentstate.EventTypeApprovalPending,
				Timestamp:   now,
				PayloadText: "fetch_approval_required",
			},
			{
				SessionID:   "sess-engine-resume-history",
				Sequence:    2,
				Node:        "approval",
				EventType:   agentstate.EventTypeApprovalResolved,
				Timestamp:   now.Add(1 * time.Second),
				PayloadText: agentstate.ApprovalStatusApproved,
			},
		},
		Checkpoint: &CheckpointRef{
			ID:   "cp-engine-resume-history",
			Node: "approval",
		},
	}
	resumed := &RuntimeSession{
		SessionID: "sess-engine-resume-history",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusApproved,
				Reason:       "fetch_approval_required",
				CheckpointID: "cp-engine-resume-history",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID: "sess-engine-resume-history",
				Sequence:  1,
				Node:      "",
				EventType: agentstate.EventTypeSessionStarted,
				Timestamp: now.Add(2 * time.Second),
			},
			{
				SessionID:   "sess-engine-resume-history",
				Sequence:    2,
				Node:        "approval",
				EventType:   agentstate.EventTypeResumeCompleted,
				Timestamp:   now.Add(3 * time.Second),
				PayloadText: "checkpoint_id=cp-engine-resume-history",
			},
		},
		Checkpoint: &CheckpointRef{
			ID:   "cp-engine-resume-history",
			Node: "approval",
		},
	}
	runner := &fakeKernelRunner{
		resumeSession: resumed,
	}

	engine := NewEngine(runner)
	result, err := engine.Resume(context.Background(), session, "cp-engine-resume-history")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Outcome.Decision != DecisionResume {
		t.Fatalf("expected resume decision, got %+v", result)
	}
	if runtimeEventCount(resumed, agentstate.EventTypeApprovalPending, "approval") != 1 {
		t.Fatalf("expected merged approval_pending event, got %+v", resumed.Journal)
	}
	if runtimeEventCount(resumed, agentstate.EventTypeApprovalResolved, "approval") != 1 {
		t.Fatalf("expected merged approval_resolved event, got %+v", resumed.Journal)
	}
	if runtimeEventCount(resumed, agentstate.EventTypeResumeCompleted, "approval") != 1 {
		t.Fatalf("expected single resume_completed event, got %+v", resumed.Journal)
	}
}

func TestEngineResume_ApprovedRerunUsesSharedStateAppliedEvent(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-approved-rerun",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusApproved,
				Reason:       "fetch_approval_required",
				CheckpointID: "cp-engine-approved-rerun",
			},
			Execution: agentstate.ExecutionState{
				Interrupted:     true,
				InterruptReason: "fetch_approval_required",
			},
		},
		Checkpoint: &CheckpointRef{
			ID:   "cp-engine-approved-rerun",
			Node: "approval",
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
	}

	engine := NewEngine(runner)
	result, err := engine.Resume(context.Background(), session, "cp-engine-approved-rerun")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Outcome.Decision != DecisionResume {
		t.Fatalf("expected resume decision, got %+v", result)
	}
	if runtimeEventCount(session, agentstate.EventTypeStateApplied, "approval") == 0 {
		t.Fatalf("expected approved rerun prepare to append shared state_applied event, got %+v", session.Journal)
	}
	if session.Snapshot.Execution.Interrupted {
		t.Fatalf("expected approved rerun prepare to clear interrupted state via shared reducer, got %+v", session.Snapshot.Execution)
	}
}

func TestEngineRunWithCheckpoint_PreservesFailureDecision(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-fail",
	}
	runner := &fakeKernelRunner{
		runSession: session,
		runErr:     errors.New("boom"),
	}

	engine := NewEngine(runner)
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-fail")
	if err == nil {
		t.Fatal("expected failure error")
	}
	if result == nil || result.Outcome.Decision != DecisionFail {
		t.Fatalf("expected fail decision, got result=%+v err=%v", result, err)
	}
	if result.Outcome.ErrorClass != ErrorClassUnknown {
		t.Fatalf("expected unknown runtime error class, got %+v", result)
	}
}

func TestEngineRunWithCheckpoint_MapsDegradedSessionToDegradeDecision(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-degrade",
		Snapshot: agentstate.StateSnapshot{
			Answer: agentstate.AnswerState{
				DegradeReason: "iteration_budget_exhausted",
			},
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
	}

	engine := NewEngine(runner)
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-degrade")
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}
	if result.Outcome.Decision != DecisionDegrade {
		t.Fatalf("expected degrade decision, got %+v", result)
	}
	if result.Outcome.ErrorClass != ErrorClassBudget {
		t.Fatalf("expected budget error class, got %+v", result)
	}
}

func TestEngineRunWithCheckpoint_MapsRejectedApprovalToRejectDecision(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-engine-reject",
		Snapshot: agentstate.StateSnapshot{
			Approval: agentstate.ApprovalState{
				Status: agentstate.ApprovalStatusRejected,
			},
			Answer: agentstate.AnswerState{
				DegradeReason: "approval_rejected",
			},
		},
	}
	runner := &fakeKernelRunner{
		runSession: session,
	}

	engine := NewEngine(runner)
	result, err := engine.RunWithCheckpoint(context.Background(), session, "cp-engine-reject")
	if err != nil {
		t.Fatalf("RunWithCheckpoint() error = %v", err)
	}
	if result.Outcome.Decision != DecisionReject {
		t.Fatalf("expected reject decision, got %+v", result)
	}
	if result.Outcome.ErrorClass != ErrorClassApprovalRejected {
		t.Fatalf("expected approval rejected error class, got %+v", result)
	}
}

func newInterruptedPendingApprovalSnapshot(checkpointID string) agentstate.StateSnapshot {
	return agentstate.StateSnapshot{
		Approval: agentstate.ApprovalState{
			Status:       agentstate.ApprovalStatusPending,
			CheckpointID: checkpointID,
		},
		Execution: agentstate.ExecutionState{
			Interrupted: true,
		},
	}
}

func runtimeEventCount(session *RuntimeSession, eventType string, node string) int {
	if session == nil {
		return 0
	}
	count := 0
	for _, event := range session.Journal {
		if event.EventType != eventType {
			continue
		}
		if node != "" && event.Node != node {
			continue
		}
		count++
	}
	return count
}
