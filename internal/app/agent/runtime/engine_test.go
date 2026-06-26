package runtime

import (
	"context"
	"errors"
	"testing"

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
