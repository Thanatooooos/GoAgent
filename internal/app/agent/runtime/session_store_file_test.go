package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

func TestFileSessionStorePersistsAcrossInstances(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	session := &RuntimeSession{
		SessionID: "session-file-store",
		Request: RequestEnvelope{
			Question: "why did fetch fail",
			UserID:   "u-1",
			Options: agentstate.RuntimeOptions{
				MaxIterations:   3,
				RequireApproval: true,
				OutputMode:      agentstate.OutputModeFinalAnswer,
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Context: agentstate.ContextState{
				SearchQuery: "fetch failure root cause",
				Notes:       []string{"pending approval"},
			},
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusPending,
				CheckpointID: "checkpoint-file-store",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			agentstate.NewRuntimeEventAt(time.Unix(1700000000, 0), "session-file-store", "approval", agentstate.EventTypeInterrupt, "awaiting approval"),
		},
		Checkpoint: &CheckpointRef{
			ID:          "checkpoint-file-store",
			Node:        "approval",
			EventOffset: 1,
			CreatedAt:   time.Unix(1700000001, 0),
		},
		Metadata: SessionMetadata{
			CreatedAt:   time.Unix(1700000000, 0),
			UpdatedAt:   time.Unix(1700000001, 0),
			RuntimeName: "agent_service_reactive",
		},
	}

	if err := store.Put(context.Background(), "checkpoint-file-store", session); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put(context.Background(), "session-file-store", session); err != nil {
		t.Fatalf("Put(session alias) error = %v", err)
	}

	reopened, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore(reopen) error = %v", err)
	}

	storedByCheckpoint, ok, err := reopened.Get(context.Background(), "checkpoint-file-store")
	if err != nil {
		t.Fatalf("Get(checkpoint) error = %v", err)
	}
	if !ok || storedByCheckpoint == nil {
		t.Fatalf("expected checkpoint session, got ok=%v session=%+v", ok, storedByCheckpoint)
	}
	if storedByCheckpoint.SessionID != "session-file-store" {
		t.Fatalf("unexpected stored session id: %+v", storedByCheckpoint)
	}
	if storedByCheckpoint.Snapshot.Context.SearchQuery != "fetch failure root cause" {
		t.Fatalf("unexpected stored snapshot: %+v", storedByCheckpoint.Snapshot.Context)
	}
	if len(storedByCheckpoint.Journal) != 1 || storedByCheckpoint.Journal[0].EventType != agentstate.EventTypeInterrupt {
		t.Fatalf("unexpected stored journal: %+v", storedByCheckpoint.Journal)
	}

	storedByCheckpoint.Snapshot.Context.Notes[0] = "mutated"
	storedAgain, ok, err := reopened.Get(context.Background(), "checkpoint-file-store")
	if err != nil {
		t.Fatalf("Get(after mutate) error = %v", err)
	}
	if !ok || storedAgain == nil {
		t.Fatalf("expected session after mutate, got ok=%v session=%+v", ok, storedAgain)
	}
	if storedAgain.Snapshot.Context.Notes[0] != "pending approval" {
		t.Fatalf("expected Get() to return a defensive copy, got %+v", storedAgain.Snapshot.Context.Notes)
	}

	storedBySession, ok, err := reopened.Get(context.Background(), "session-file-store")
	if err != nil {
		t.Fatalf("Get(session alias) error = %v", err)
	}
	if !ok || storedBySession == nil {
		t.Fatalf("expected session alias to resolve, got ok=%v session=%+v", ok, storedBySession)
	}
}

func TestFileSessionStoreDeleteIsIdempotent(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	if err := store.Put(context.Background(), "checkpoint-delete-file-store", &RuntimeSession{SessionID: "session-delete-file-store"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Delete(context.Background(), "checkpoint-delete-file-store"); err != nil {
		t.Fatalf("Delete(first) error = %v", err)
	}
	if err := store.Delete(context.Background(), "checkpoint-delete-file-store"); err != nil {
		t.Fatalf("Delete(second) error = %v", err)
	}

	stored, ok, err := store.Get(context.Background(), "checkpoint-delete-file-store")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok || stored != nil {
		t.Fatalf("expected deleted session to remain absent, got ok=%v session=%+v", ok, stored)
	}
}

func TestFileSessionStoreGet_NormalizesLegacySnapshotCompatibility(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	legacySession := &RuntimeSession{
		SessionID: "session-legacy-file-store",
		InitialSnapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "legacy session",
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "legacy session",
			},
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusPending,
				CheckpointID: "checkpoint-legacy-file-store",
			},
		},
	}

	payload, err := json.Marshal(legacySession)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	key, err := checkpointKey("checkpoint-legacy-file-store")
	if err != nil {
		t.Fatalf("checkpointKey() error = %v", err)
	}
	if err := writeStoreFile(store.pathForKey(key), payload); err != nil {
		t.Fatalf("writeStoreFile() error = %v", err)
	}

	stored, ok, err := store.Get(context.Background(), "checkpoint-legacy-file-store")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || stored == nil {
		t.Fatalf("expected legacy session to load, got ok=%v session=%+v", ok, stored)
	}
	if stored.InitialSnapshot.SchemaVersion != agentstate.CurrentSnapshotVersion {
		t.Fatalf("expected initial snapshot schema version %d, got %d", agentstate.CurrentSnapshotVersion, stored.InitialSnapshot.SchemaVersion)
	}
	if stored.Snapshot.SchemaVersion != agentstate.CurrentSnapshotVersion {
		t.Fatalf("expected snapshot schema version %d, got %d", agentstate.CurrentSnapshotVersion, stored.Snapshot.SchemaVersion)
	}
	if stored.Snapshot.Execution.Status != agentstate.ExecutionStatusInterrupted {
		t.Fatalf("expected legacy approval-pending snapshot to derive interrupted status, got %+v", stored.Snapshot.Execution)
	}
}

func TestFileSessionStoreGet_RejectsUnsupportedFutureSnapshotVersion(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	futureSession := &RuntimeSession{
		SessionID: "session-future-file-store",
		Snapshot: agentstate.StateSnapshot{
			SchemaVersion: agentstate.CurrentSnapshotVersion + 1,
			Request: agentstate.RequestState{
				Question: "future session",
			},
		},
	}

	payload, err := json.Marshal(futureSession)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	key, err := checkpointKey("checkpoint-future-file-store")
	if err != nil {
		t.Fatalf("checkpointKey() error = %v", err)
	}
	if err := writeStoreFile(store.pathForKey(key), payload); err != nil {
		t.Fatalf("writeStoreFile() error = %v", err)
	}

	_, _, err = store.Get(context.Background(), "checkpoint-future-file-store")
	if err == nil {
		t.Fatal("expected unsupported future snapshot version to fail")
	}
	if !strings.Contains(err.Error(), "unsupported snapshot schema version") {
		t.Fatalf("expected unsupported snapshot schema version error, got %v", err)
	}
}

func TestFileSessionStorePut_RejectsUnsupportedFutureSnapshotVersion(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileSessionStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}

	session := &RuntimeSession{
		SessionID: "session-future-put",
		Snapshot: agentstate.StateSnapshot{
			SchemaVersion: agentstate.CurrentSnapshotVersion + 1,
			Request: agentstate.RequestState{
				Question: "future put session",
			},
		},
	}

	err = store.Put(context.Background(), "checkpoint-future-put", session)
	if err == nil {
		t.Fatal("expected Put() to reject unsupported future snapshot version")
	}
	if !strings.Contains(err.Error(), "unsupported snapshot schema version") {
		t.Fatalf("expected unsupported snapshot schema version error, got %v", err)
	}
}
