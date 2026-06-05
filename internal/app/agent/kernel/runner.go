package kernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

// Runner executes a compiled M1 runtime graph.
type Runner struct {
	runnable compose.Runnable[*agentruntime.RuntimeSession, *agentruntime.RuntimeSession]
	reducer  agentstate.Reducer
}

// Run executes the compiled graph against the supplied runtime session.
func (r *Runner) Run(ctx context.Context, session *agentruntime.RuntimeSession, opts ...compose.Option) (*agentruntime.RuntimeSession, error) {
	return r.run(ctx, session, "", false, opts...)
}

// RunWithCheckpoint executes a run that persists interrupt checkpoints under the supplied ID.
func (r *Runner) RunWithCheckpoint(ctx context.Context, session *agentruntime.RuntimeSession, checkpointID string, opts ...compose.Option) (*agentruntime.RuntimeSession, error) {
	return r.run(ctx, session, checkpointID, false, opts...)
}

// Resume continues a previously interrupted run from the supplied checkpoint ID.
func (r *Runner) Resume(ctx context.Context, session *agentruntime.RuntimeSession, checkpointID string, opts ...compose.Option) (*agentruntime.RuntimeSession, error) {
	return r.run(ctx, session, checkpointID, true, opts...)
}

func (r *Runner) run(ctx context.Context, session *agentruntime.RuntimeSession, checkpointID string, resume bool, opts ...compose.Option) (*agentruntime.RuntimeSession, error) {
	if r == nil || r.runnable == nil {
		return nil, fmt.Errorf("kernel runner is not initialized")
	}
	if session == nil {
		return nil, fmt.Errorf("runtime session is required")
	}
	if !agentstate.HasContent(session.InitialSnapshot) {
		session.InitialSnapshot = agentstate.CloneSnapshot(session.Snapshot)
	}

	invokeOpts := append([]compose.Option(nil), opts...)
	if trimmedCheckpointID := strings.TrimSpace(checkpointID); trimmedCheckpointID != "" {
		invokeOpts = append(invokeOpts, compose.WithCheckPointID(trimmedCheckpointID))
		checkpointID = trimmedCheckpointID
	}

	result, err := r.runnable.Invoke(ctx, session, invokeOpts...)
	finalSession := session
	if result != nil {
		finalSession = result
	}
	r.updateRunMetadata(finalSession, checkpointID, resume, err)
	return finalSession, err
}

func (r *Runner) updateRunMetadata(session *agentruntime.RuntimeSession, checkpointID string, resume bool, err error) {
	if session == nil {
		return
	}

	now := time.Now()
	if session.Metadata.CreatedAt.IsZero() {
		session.Metadata.CreatedAt = now
	}
	session.Metadata.UpdatedAt = now

	if strings.TrimSpace(checkpointID) == "" {
		return
	}

	if interruptInfo, ok := compose.ExtractInterruptInfo(err); ok {
		node := checkpointNode(interruptInfo)
		if applyErr := r.applySessionDelta(session, node, agentstate.StateDelta{
			Execution: &agentstate.ExecutionDelta{
				CurrentNode:     stringPtr(node),
				Interrupted:     boolPtr(true),
				InterruptReason: stringPtr(interruptSummary(interruptInfo)),
			},
		}, now); applyErr != nil {
			appendSessionEvent(session, agentstate.NewRuntimeEventAt(
				now,
				session.SessionID,
				node,
				agentstate.EventTypeReducerError,
				applyErr.Error(),
			))
			return
		}
		session.Checkpoint = &agentruntime.CheckpointRef{
			ID:          checkpointID,
			Node:        node,
			EventOffset: len(session.Journal),
			CreatedAt:   now,
		}
		event := agentstate.NewRuntimeEventAt(
			now,
			session.SessionID,
			node,
			agentstate.EventTypeInterrupt,
			interruptSummary(interruptInfo),
		)
		event.Checkpoint = agentstate.NewCheckpointRef(checkpointID, node)
		appendSessionEvent(session, event)
		if resume {
			session.Metadata.ResumedFrom = checkpointID
			session.Metadata.ResumeCount++
		}
		return
	}

	if resume {
		node := checkpointNodeFromSession(session)
		if applyErr := r.applySessionDelta(session, node, agentstate.StateDelta{
			Execution: &agentstate.ExecutionDelta{
				CurrentNode:     stringPtrIfNotEmpty(node),
				Interrupted:     boolPtr(false),
				InterruptReason: stringPtr(""),
			},
		}, now); applyErr != nil {
			appendSessionEvent(session, agentstate.NewRuntimeEventAt(
				now,
				session.SessionID,
				node,
				agentstate.EventTypeReducerError,
				applyErr.Error(),
			))
			return
		}
		session.Metadata.ResumedFrom = checkpointID
		session.Metadata.ResumeCount++
		event := agentstate.NewRuntimeEventAt(
			now,
			session.SessionID,
			node,
			agentstate.EventTypeResumeCompleted,
			"checkpoint_id="+checkpointID,
		)
		event.Checkpoint = agentstate.NewCheckpointRef(checkpointID, node)
		appendSessionEvent(session, event)
	}

	if session.Checkpoint == nil {
		session.Checkpoint = &agentruntime.CheckpointRef{
			ID:          checkpointID,
			EventOffset: len(session.Journal),
			CreatedAt:   now,
		}
	}
}

func (r *Runner) applySessionDelta(session *agentruntime.RuntimeSession, node string, delta agentstate.StateDelta, now time.Time) error {
	if session == nil {
		return nil
	}
	reducer := r.reducer
	if reducer == nil {
		reducer = agentstate.DefaultReducer{}
	}
	nextSnapshot, err := reducer.Apply(session.Snapshot, delta)
	if err != nil {
		return err
	}
	session.Snapshot = nextSnapshot
	session.Metadata.UpdatedAt = now
	stateApplied := agentstate.NewRuntimeEventAt(
		now,
		session.SessionID,
		node,
		agentstate.EventTypeStateApplied,
		"",
	)
	cloned := agentstate.CloneDelta(delta)
	stateApplied.Delta = &cloned
	appendSessionEvent(session, stateApplied)
	return nil
}

func checkpointNode(info *compose.InterruptInfo) string {
	if info == nil {
		return ""
	}
	if len(info.BeforeNodes) > 0 {
		return info.BeforeNodes[0]
	}
	if len(info.AfterNodes) > 0 {
		return info.AfterNodes[0]
	}
	if len(info.RerunNodes) > 0 {
		return info.RerunNodes[0]
	}
	return ""
}

func checkpointNodeFromSession(session *agentruntime.RuntimeSession) string {
	if session == nil || session.Checkpoint == nil {
		return ""
	}
	return session.Checkpoint.Node
}

func interruptSummary(info *compose.InterruptInfo) string {
	if info == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if len(info.BeforeNodes) > 0 {
		parts = append(parts, "before_nodes="+strings.Join(info.BeforeNodes, ","))
	}
	if len(info.AfterNodes) > 0 {
		parts = append(parts, "after_nodes="+strings.Join(info.AfterNodes, ","))
	}
	if len(info.RerunNodes) > 0 {
		parts = append(parts, "rerun_nodes="+strings.Join(info.RerunNodes, ","))
	}
	return strings.Join(parts, " ")
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}
