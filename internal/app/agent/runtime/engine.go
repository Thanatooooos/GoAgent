package runtime

import (
	"context"
	"errors"
	"strings"

	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

var errEngineNotInitialized = errors.New("runtime engine is not initialized")

// KernelRunner is the compiled graph execution contract consumed by the
// runtime engine facade.
type KernelRunner interface {
	RunWithCheckpoint(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RuntimeSession, error)
	Resume(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RuntimeSession, error)
}

// Engine is the runtime facade between service orchestration and the compiled
// kernel runner.
type Engine struct {
	runner KernelRunner
}

func NewEngine(runner KernelRunner) *Engine {
	return &Engine{runner: runner}
}

func (e *Engine) RunWithCheckpoint(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RunResult, error) {
	if e == nil || e.runner == nil {
		return nil, errEngineNotInitialized
	}
	finalSession, err := e.runner.RunWithCheckpoint(ctx, session, checkpointID, opts...)
	return classifyRunResult(finalSession, checkpointID, false, err)
}

func (e *Engine) Resume(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RunResult, error) {
	if e == nil || e.runner == nil {
		return nil, errEngineNotInitialized
	}
	finalSession, err := e.runner.Resume(ctx, session, checkpointID, opts...)
	return classifyRunResult(finalSession, checkpointID, true, err)
}

func classifyRunResult(session *RuntimeSession, checkpointID string, resumed bool, err error) (*RunResult, error) {
	result := &RunResult{
		Session: session,
		Outcome: Outcome{
			CheckpointID: firstNonEmptyRuntimeCheckpointID(session, checkpointID),
		},
	}
	if session != nil {
		result.Outcome.Interrupted = session.Snapshot.Execution.Interrupted
	}

	if isInterruptedSession(session) {
		result.Outcome.Decision = DecisionWaitApproval
		result.Outcome.Reason = strings.TrimSpace(session.Snapshot.Approval.Reason)
		return result, nil
	}
	if err != nil {
		result.Outcome.Decision = DecisionFail
		result.Outcome.ErrorClass = ErrorClassForSession(session)
		return result, err
	}
	if isRejectedSession(session) {
		result.Outcome.Decision = DecisionReject
		result.Outcome.Reason = "approval_rejected"
		result.Outcome.ErrorClass = ErrorClassApprovalRejected
		result.Outcome.DegradeReason = strings.TrimSpace(session.Snapshot.Answer.DegradeReason)
		return result, nil
	}
	if degradeReason := degradeReasonFromSession(session); degradeReason != "" {
		result.Outcome.Decision = DecisionDegrade
		result.Outcome.Reason = degradeReason
		result.Outcome.DegradeReason = degradeReason
		result.Outcome.ErrorClass = ErrorClassForReason(degradeReason)
		return result, nil
	}
	if resumed {
		result.Outcome.Decision = DecisionResume
		return result, nil
	}
	result.Outcome.Decision = DecisionComplete
	return result, nil
}

func isInterruptedSession(session *RuntimeSession) bool {
	if session == nil {
		return false
	}
	if session.Snapshot.Execution.Interrupted {
		return true
	}
	return strings.TrimSpace(session.Snapshot.Approval.Status) == agentstate.ApprovalStatusPending
}

func firstNonEmptyRuntimeCheckpointID(session *RuntimeSession, fallback string) string {
	if session != nil && session.Checkpoint != nil {
		if trimmed := strings.TrimSpace(session.Checkpoint.ID); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(fallback)
}

func isRejectedSession(session *RuntimeSession) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.Snapshot.Approval.Status) == agentstate.ApprovalStatusRejected
}

func degradeReasonFromSession(session *RuntimeSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.Snapshot.Answer.DegradeReason)
}
