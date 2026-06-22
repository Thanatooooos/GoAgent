package memory_recall

import (
	"context"
	"testing"

	longtermmemory "local/rag-project/internal/app/rag/service/longtermmemory"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

type stubRecaller struct {
	lastInput longtermmemory.RecallMemoriesInput
	result    longtermmemory.RecallMemoriesResult
	err       error
}

func (s *stubRecaller) RecallMemories(_ context.Context, input longtermmemory.RecallMemoriesInput) (longtermmemory.RecallMemoriesResult, error) {
	s.lastInput = input
	if s.err != nil {
		return longtermmemory.RecallMemoriesResult{}, s.err
	}
	if s.result.SelectedCount > 0 || s.result.Used {
		return s.result, nil
	}
	return longtermmemory.RecallMemoriesResult{
		Used:           true,
		Context:        "memory context",
		SelectedCount:  1,
		CandidateCount: 2,
		SelectedMemoryIDs: []string{
			"mem-1",
		},
		SelectedEntries: []longtermmemory.RecallMemoryEntry{
			{ID: "mem-1", MemoryType: "preference", Summary: "user prefers zh", FinalScore: 8},
		},
	}, nil
}

func TestCapabilityInvokeBuildsInvocationResult(t *testing.T) {
	recaller := &stubRecaller{}
	handle, err := NewCapability(recaller)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: " preference ", UserID: "u1"},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{UserID: "u1"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if recaller.lastInput.UserID != "u1" || recaller.lastInput.Query != "preference" {
		t.Fatalf("unexpected recall input: %+v", recaller.lastInput)
	}
	if result.Status != agentcapability.StatusSucceeded {
		t.Fatalf("expected succeeded status, got %+v", result)
	}
	if result.Delta.Context == nil || len(result.Delta.Context.MemoryRefs) != 1 {
		t.Fatalf("expected memory refs in delta, got %+v", result.Delta)
	}
	if len(result.EvidenceRefs) != 1 || result.EvidenceRefs[0].SourceRef != "mem-1" {
		t.Fatalf("expected evidence refs, got %+v", result.EvidenceRefs)
	}
}

func TestCapabilityInvokeUsesSnapshotUserID(t *testing.T) {
	recaller := &stubRecaller{}
	handle, err := NewCapability(recaller)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	_, err = handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input:    CapabilityInput{Query: "prefs"},
		Snapshot: agentstate.StateSnapshot{Request: agentstate.RequestState{UserID: "snapshot-user"}},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if recaller.lastInput.UserID != "snapshot-user" {
		t.Fatalf("expected snapshot user id, got %+v", recaller.lastInput)
	}
}

func TestCapabilityInvokeRequestsActiveGlobalPreferenceRecall(t *testing.T) {
	recaller := &stubRecaller{}
	handle, err := NewCapability(recaller)
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}

	_, err = handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "answer in Chinese", UserID: "u1"},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{UserID: "u1"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(recaller.lastInput.ScopeTypes) != 1 || recaller.lastInput.ScopeTypes[0] != "global" {
		t.Fatalf("expected global scope filter, got %+v", recaller.lastInput)
	}
	if len(recaller.lastInput.MemoryTypes) != 1 || recaller.lastInput.MemoryTypes[0] != "preference" {
		t.Fatalf("expected preference type filter, got %+v", recaller.lastInput)
	}
	if len(recaller.lastInput.Statuses) != 1 || recaller.lastInput.Statuses[0] != "active" {
		t.Fatalf("expected active status filter, got %+v", recaller.lastInput)
	}
}

func TestCapabilityInvokeRejectsMissingUserID(t *testing.T) {
	handle, err := NewCapability(&stubRecaller{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input: CapabilityInput{Query: "prefs"},
	}); err == nil {
		t.Fatal("expected missing user id to fail")
	}
}

func TestCapabilityInvokeRejectsUnexpectedInput(t *testing.T) {
	handle, err := NewCapability(&stubRecaller{})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	if _, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: 1}); err == nil {
		t.Fatal("expected unexpected input type to fail")
	}
}

func TestCapabilityInvokeFailsOpenOnDependencyError(t *testing.T) {
	handle, err := NewCapability(&stubRecaller{err: context.Canceled})
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	result, err := handle.Invoke(context.Background(), agentcapability.InvocationRequest{
		Input:    CapabilityInput{Query: "prefs", UserID: "u1"},
		Snapshot: agentstate.StateSnapshot{Request: agentstate.RequestState{UserID: "u1"}},
	})
	if err != nil {
		t.Fatalf("expected fail-open invoke, got %v", err)
	}
	if result.Status != agentcapability.StatusDegraded {
		t.Fatalf("expected degraded status, got %+v", result)
	}
	if result.Output != nil {
		t.Fatalf("expected empty output on fail-open path, got %+v", result.Output)
	}
}
