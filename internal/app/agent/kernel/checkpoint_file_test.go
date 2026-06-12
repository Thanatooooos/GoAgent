package kernel

import (
	"context"
	"testing"
)

func TestFileCheckpointStorePersistsAcrossInstances(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewFileCheckpointStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore() error = %v", err)
	}

	payload := []byte("checkpoint-payload")
	if err := store.Set(context.Background(), "checkpoint-file-store", payload); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	reopened, err := NewFileCheckpointStore(rootDir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore(reopen) error = %v", err)
	}

	stored, ok, err := reopened.Get(context.Background(), "checkpoint-file-store")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("expected checkpoint to exist after reopen")
	}
	if string(stored) != "checkpoint-payload" {
		t.Fatalf("unexpected checkpoint payload: %q", string(stored))
	}

	stored[0] = 'X'
	storedAgain, ok, err := reopened.Get(context.Background(), "checkpoint-file-store")
	if err != nil {
		t.Fatalf("Get(after mutate) error = %v", err)
	}
	if !ok {
		t.Fatal("expected checkpoint to remain readable after mutate")
	}
	if string(storedAgain) != "checkpoint-payload" {
		t.Fatalf("expected defensive copy on Get(), got %q", string(storedAgain))
	}
}

func TestFileCheckpointStoreGetMissingReturnsNotFound(t *testing.T) {
	store, err := NewFileCheckpointStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCheckpointStore() error = %v", err)
	}

	stored, ok, err := store.Get(context.Background(), "missing-checkpoint")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok || stored != nil {
		t.Fatalf("expected missing checkpoint, got ok=%v payload=%q", ok, string(stored))
	}
}
