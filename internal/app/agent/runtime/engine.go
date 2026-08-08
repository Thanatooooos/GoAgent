package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

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
	runner                KernelRunner
	reducer               agentstate.Reducer
	pendingApprovalCompat PendingApprovalCompatResolver
}

type EngineOption func(*Engine)

func NewEngine(runner KernelRunner, opts ...EngineOption) *Engine {
	engine := &Engine{
		runner:  runner,
		reducer: agentstate.DefaultReducer{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}
	if engine.reducer == nil {
		engine.reducer = agentstate.DefaultReducer{}
	}
	return engine
}

func WithReducer(reducer agentstate.Reducer) EngineOption {
	return func(engine *Engine) {
		if engine == nil || reducer == nil {
			return
		}
		engine.reducer = reducer
	}
}

func WithPendingApprovalCompat(resolver PendingApprovalCompatResolver) EngineOption {
	return func(engine *Engine) {
		if engine == nil {
			return
		}
		engine.pendingApprovalCompat = resolver
	}
}

func (e *Engine) NormalizePendingApprovalSession(session *RuntimeSession, checkpointID string) bool {
	if normalizePendingApprovalSession(session, checkpointID, e.reducerOrDefault()) {
		return true
	}
	return e.normalizeLegacyPendingApprovalSession(session, checkpointID)
}

func (e *Engine) ApplyApprovalDecision(session *RuntimeSession, checkpointID string, decision string, note string) error {
	return applyApprovalDecision(session, checkpointID, decision, note, e.reducerOrDefault())
}

func (e *Engine) FinalizeRejectedApproval(session *RuntimeSession) (*RuntimeSession, error) {
	return finalizeRejectedApproval(session, e.reducerOrDefault())
}

func (e *Engine) reducerOrDefault() agentstate.Reducer {
	if e == nil || e.reducer == nil {
		return agentstate.DefaultReducer{}
	}
	return e.reducer
}

func (e *Engine) RunWithCheckpoint(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RunResult, error) {
	if e == nil || e.runner == nil {
		return nil, errEngineNotInitialized
	}
	finalSession, err := e.runner.RunWithCheckpoint(ctx, session, checkpointID, opts...)
	e.NormalizePendingApprovalSession(finalSession, checkpointID)
	return classifyRunResult(finalSession, checkpointID, false, err)
}

func (e *Engine) Resume(ctx context.Context, session *RuntimeSession, checkpointID string, opts ...compose.Option) (*RunResult, error) {
	if e == nil || e.runner == nil {
		return nil, errEngineNotInitialized
	}
	if shouldRerunApprovedApproval(session) {
		if err := prepareApprovedApprovalRerun(session, checkpointID, e.reducerOrDefault()); err != nil {
			return classifyRunResult(session, checkpointID, true, err)
		}
		finalSession, err := e.runner.RunWithCheckpoint(ctx, session, checkpointID, opts...)
		MergeApprovalResumeHistory(session, finalSession)
		recordResumeCompletion(finalSession, checkpointID)
		e.NormalizePendingApprovalSession(finalSession, checkpointID)
		return classifyRunResult(finalSession, checkpointID, true, err)
	}
	finalSession, err := e.runner.Resume(ctx, session, checkpointID, opts...)
	MergeApprovalResumeHistory(session, finalSession)
	e.NormalizePendingApprovalSession(finalSession, checkpointID)
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

func shouldRerunApprovedApproval(session *RuntimeSession) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.Snapshot.Approval.Status) == agentstate.ApprovalStatusApproved
}

func prepareApprovedApprovalRerun(session *RuntimeSession, checkpointID string, reducer agentstate.Reducer) error {
	if session == nil {
		return nil
	}
	now := time.Now()
	if err := newStateApplier(reducer).apply(session, "approval", agentstate.StateDelta{
		Execution: &agentstate.ExecutionDelta{
			Interrupted:     boolPtr(false),
			InterruptReason: stringPtr(""),
		},
	}, now); err != nil {
		return err
	}
	session.Metadata.ResumedFrom = strings.TrimSpace(checkpointID)
	session.Metadata.UpdatedAt = now
	return nil
}

func (e *Engine) normalizeLegacyPendingApprovalSession(session *RuntimeSession, checkpointID string) bool {
	if e == nil || e.pendingApprovalCompat == nil || session == nil || !session.Snapshot.Execution.Interrupted {
		return false
	}
	if strings.TrimSpace(session.Snapshot.Approval.Status) != "" {
		return false
	}
	compat, ok := e.pendingApprovalCompat(session)
	if !ok {
		return false
	}
	now := time.Now()
	reason := firstNonEmpty(strings.TrimSpace(compat.Reason), approvalReason(session))
	finalCheckpointID := firstNonEmpty(
		strings.TrimSpace(checkpointID),
		firstNonEmptyRuntimeCheckpointID(session, ""),
		strings.TrimSpace(session.SessionID),
	)
	requestedAt := session.Snapshot.Approval.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = now
	}
	delta := agentstate.StateDelta{
		Approval: BuildPendingApprovalDelta(reason, strings.TrimSpace(compat.Capability), strings.TrimSpace(compat.RerunNode), finalCheckpointID, requestedAt),
		Execution: &agentstate.ExecutionDelta{
			Interrupted:     boolPtr(true),
			InterruptReason: stringPtr(reason),
		},
	}
	if err := newStateApplier(e.reducerOrDefault()).apply(session, "approval", delta, now); err != nil {
		return false
	}
	if !hasApprovalLifecycleEvent(session, agentstate.EventTypeApprovalPending, finalCheckpointID) {
		appendApprovalRuntimeEvent(session, "approval", agentstate.EventTypeApprovalPending, reason, finalCheckpointID)
	}
	return true
}

func recordResumeCompletion(session *RuntimeSession, checkpointID string) {
	if session == nil || isInterruptedSession(session) || hasRuntimeEventTypeInSession(session, agentstate.EventTypeResumeCompleted) {
		return
	}
	session.Metadata.ResumedFrom = strings.TrimSpace(checkpointID)
	session.Metadata.ResumeCount++
	session.Metadata.UpdatedAt = time.Now()
	node := "approval"
	if session.Checkpoint != nil && strings.TrimSpace(session.Checkpoint.Node) != "" {
		node = strings.TrimSpace(session.Checkpoint.Node)
	}
	event := agentstate.NewRuntimeEventAt(
		time.Now(),
		session.SessionID,
		node,
		agentstate.EventTypeResumeCompleted,
		"checkpoint_id="+strings.TrimSpace(checkpointID),
	)
	if strings.TrimSpace(checkpointID) != "" {
		event.Checkpoint = agentstate.NewCheckpointRef(strings.TrimSpace(checkpointID), node)
	}
	appendRuntimeEvent(session, event)
}
