package runtime

import (
	"context"
	"fmt"
	"strings"
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
type SessionStore interface {
	Put(ctx context.Context, checkpointID string, session *RuntimeSession) error
	Get(ctx context.Context, checkpointID string) (*RuntimeSession, bool, error)
	Delete(ctx context.Context, checkpointID string) error
}

// CloneSession deep-copies a runtime session for safe persistence and replay.
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

func checkpointKey(checkpointID string) (string, error) {
	trimmed := strings.TrimSpace(checkpointID)
	if trimmed == "" {
		return "", fmt.Errorf("checkpoint id is required")
	}
	return trimmed, nil
}
