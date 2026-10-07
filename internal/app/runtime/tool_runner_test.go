package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"local/rag-project/internal/app/runtime/capability"
)

func TestToolRunnerPersistsFullSuccessfulLifecycle(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)
	registry := capability.NewRegistry()
	if err := registry.Register(testCapability("lookup", nil)); err != nil {
		t.Fatal(err)
	}
	runner := NewToolRunner(registry, lifecycle)

	_, result, err := runner.Run(context.Background(), session, ToolCall{ID: "call-1", CapabilityID: "lookup", Arguments: capability.Value(`{"q":"x"}`)}, capability.Context{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Content != "answer" {
		t.Fatalf("result = %#v", result)
	}
	assertToolStates(t, store.entries(session.ID), ToolStatePending, ToolStateExecuting, ToolStateCompleted)
}

func TestToolRunnerSettlesDeniedWithoutExecution(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)
	registry := capability.NewRegistry()
	executed := false
	if err := registry.Register(testCapability("restricted", capability.Deny("not allowed"), func() { executed = true })); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewToolRunner(registry, lifecycle).Run(context.Background(), session, ToolCall{ID: "call-1", CapabilityID: "restricted", Arguments: capability.Value(`{}`)}, capability.Context{})
	if err == nil {
		t.Fatal("denied call must return an error")
	}
	if executed {
		t.Fatal("denied call executed")
	}
	assertToolStates(t, store.entries(session.ID), ToolStatePending, ToolStateDenied)
}

func TestToolRunnerSettlesInvalidArgumentsAsFailed(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)
	registry := capability.NewRegistry()
	if err := registry.Register(capability.Def{ID: "validate", Description: "validate", JSONSchema: json.RawMessage(`{"type":"object"}`), Validate: func(capability.Value) error { return errors.New("bad argument") }, Describe: func(capability.Value, capability.Context) (capability.Operation, error) {
		return capability.Operation{}, nil
	}, Execute: func(capability.Value, capability.Context) (capability.Result, error) {
		t.Fatal("invalid call executed")
		return capability.Result{}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewToolRunner(registry, lifecycle).Run(context.Background(), session, ToolCall{ID: "call-1", CapabilityID: "validate", Arguments: capability.Value(`{}`)}, capability.Context{})
	if err == nil {
		t.Fatal("invalid call must return an error")
	}
	assertToolStates(t, store.entries(session.ID), ToolStatePending, ToolStateFailed)
}

func TestToolRunnerSettlesResultEncodingFailureAsFailed(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session := mustStartSession(t, lifecycle)
	registry := capability.NewRegistry()
	if err := registry.Register(capability.Def{ID: "bad-result", Description: "bad result", JSONSchema: json.RawMessage(`{"type":"object"}`), Validate: func(capability.Value) error { return nil }, Describe: func(capability.Value, capability.Context) (capability.Operation, error) {
		return capability.Operation{}, nil
	}, Execute: func(capability.Value, capability.Context) (capability.Result, error) {
		return capability.Result{Value: capability.Value(`not-json`)}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewToolRunner(registry, lifecycle).Run(context.Background(), session, ToolCall{ID: "call-1", CapabilityID: "bad-result", Arguments: capability.Value(`{}`)}, capability.Context{})
	if err == nil {
		t.Fatal("unencodable result must return an error")
	}
	assertToolStates(t, store.entries(session.ID), ToolStatePending, ToolStateExecuting, ToolStateFailed)
}

func testCapability(id string, describeErr error, onExecute ...func()) capability.Def {
	return capability.Def{ID: id, Description: id, JSONSchema: json.RawMessage(`{"type":"object"}`), Validate: func(capability.Value) error { return nil }, Describe: func(capability.Value, capability.Context) (capability.Operation, error) {
		return capability.Operation{ID: id}, describeErr
	}, Execute: func(capability.Value, capability.Context) (capability.Result, error) {
		if len(onExecute) > 0 {
			onExecute[0]()
		}
		return capability.Result{Content: "answer", Value: capability.Value(`{"ok":true}`)}, nil
	}}
}

func assertToolStates(t *testing.T, entries []JournalEntry, states ...string) {
	t.Helper()
	if len(entries) != len(states) {
		t.Fatalf("entries = %d, want %d: %#v", len(entries), len(states), entries)
	}
	for i, state := range states {
		if entries[i].ToolState != state {
			t.Fatalf("entry %d state = %q, want %q", i, entries[i].ToolState, state)
		}
	}
}
