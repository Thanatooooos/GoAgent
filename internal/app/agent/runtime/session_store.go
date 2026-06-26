package runtime

import (
	"context"
	"fmt"
	"strings"

	agentstate "local/rag-project/internal/app/agent/state"
)

// SessionStore persists resumable runtime sessions outside the checkpoint bytes.
//
// Responsibility boundary:
// - checkpoint store owns kernel-level execution recovery bytes
// - session store owns caller-facing approval/resume lookup state
//
// In the current approval lifecycle, agent.Service may store the same session
// under more than one lookup key, such as checkpoint ID and session ID. A
// SessionStore implementation must therefore tolerate alias-style Put/Get/Delete
// access patterns without assuming a one-key-per-session model.
//
// Public resume requests still use checkpoint ID as the canonical lookup key.
// Session-ID aliases exist for internal lifecycle management, cleanup, and
// audit projections; they are not part of the outward resume contract.
type SessionStore interface {
	Put(ctx context.Context, checkpointID string, session *RuntimeSession) error
	Get(ctx context.Context, checkpointID string) (*RuntimeSession, bool, error)
	Delete(ctx context.Context, checkpointID string) error
}

// CloneSession deep-copies a runtime session for safe persistence and replay.
// Snapshot compatibility defaults are normalized as part of the clone.
func CloneSession(session *RuntimeSession) *RuntimeSession {
	if session == nil {
		return nil
	}
	cloned := *session
	cloned.Request = cloneRequestEnvelope(session.Request)
	cloned.InitialSnapshot = cloneSnapshot(session.InitialSnapshot)
	cloned.Snapshot = cloneSnapshot(session.Snapshot)
	cloned.Journal = cloneRuntimeEvents(session.Journal)
	cloned.Checkpoint = cloneRuntimeCheckpoint(session.Checkpoint)
	return &cloned
}

// cloneCompatibleSession enforces the explicit snapshot compatibility path used
// at session persistence boundaries: older snapshots are normalized during the
// clone, while unsupported future versions are rejected.
func cloneCompatibleSession(session *RuntimeSession) (*RuntimeSession, error) {
	if session == nil {
		return nil, nil
	}
	if err := validateSessionSnapshotCompatibility(session); err != nil {
		return nil, err
	}
	return CloneSession(session), nil
}

func validateSessionSnapshotCompatibility(session *RuntimeSession) error {
	if session == nil {
		return nil
	}
	if err := validateSessionSnapshot("initial_snapshot", session.InitialSnapshot); err != nil {
		return err
	}
	if err := validateSessionSnapshot("snapshot", session.Snapshot); err != nil {
		return err
	}
	return nil
}

func validateSessionSnapshot(label string, snapshot agentstate.StateSnapshot) error {
	if err := agentstate.ValidateSnapshotCompatibility(snapshot); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

func checkpointKey(checkpointID string) (string, error) {
	trimmed := strings.TrimSpace(checkpointID)
	if trimmed == "" {
		return "", fmt.Errorf("checkpoint id is required")
	}
	return trimmed, nil
}
