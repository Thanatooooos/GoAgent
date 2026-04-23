package model

import (
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
)

func TestFilterAndSortCandidatesOrderAndFiltering(t *testing.T) {
	ms := newModelSelector(nil)

	ptrue := true
	pfalse := false

	candidates := []config.ModelCandidate{
		{Id: "c2", Provider: "p", Model: "m2", Priority: 50, Enabled: nil, SupportsThinking: &ptrue},
		{Id: "first", Provider: "p", Model: "m1", Priority: 10, Enabled: nil, SupportsThinking: &ptrue},
		{Provider: "p", Model: "m3", Priority: 0, Enabled: nil, SupportsThinking: &ptrue},
		{Id: "disabled", Provider: "p", Model: "mx", Priority: 1, Enabled: &pfalse, SupportsThinking: &ptrue},
	}

	out := ms.filterAndSortCandidates(candidates, "first", false)
	if len(out) != 3 {
		t.Fatalf("expected 3 candidates after filtering, got %d", len(out))
	}
	if resolveId(out[0]) != "first" {
		t.Fatalf("expected first candidate to be 'first', got %q", resolveId(out[0]))
	}
	if out[1].Priority != 50 {
		t.Fatalf("expected second candidate priority 50, got %d", out[1].Priority)
	}
	if out[2].Priority != 0 {
		t.Fatalf("expected default-priority candidate last, got %d", out[2].Priority)
	}
}

func TestFilterAndSortCandidatesDeepThinkingFiltersOut(t *testing.T) {
	ms := newModelSelector(nil)

	ptrue := true
	pfalse := false

	candidates := []config.ModelCandidate{
		{Id: "c1", Provider: "p", Model: "m1", Priority: 1, SupportsThinking: nil},
		{Id: "c2", Provider: "p", Model: "m2", Priority: 2, SupportsThinking: &ptrue},
		{Id: "c3", Provider: "p", Model: "m3", Priority: 3, SupportsThinking: &pfalse},
	}

	out := ms.filterAndSortCandidates(candidates, "", true)
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate after deep-thinking filter, got %d", len(out))
	}
	if resolveId(out[0]) != "c2" {
		t.Fatalf("expected candidate c2, got %s", resolveId(out[0]))
	}
}

func TestBuildModelTargetSkipsWhenHealthUnavailable(t *testing.T) {
	healthStore := NewModelHealthStore()
	healthStore.healthByID.Store("c1", &modelHealth{
		state:     Open,
		openUntil: time.Now().Add(time.Second),
	})
	ms := newModelSelector(healthStore)

	candidate := config.ModelCandidate{Id: "c1", Provider: "p", Model: "m1"}
	providers := map[string]config.ProviderConfig{"p": {Url: "http://example"}}

	if target := ms.buildModelTarget(candidate, providers); target != nil {
		t.Fatal("expected nil modelTarget when health store marks unavailable")
	}
}

func TestBuildAvailableTargetsWithProvidersAndHealth(t *testing.T) {
	ms := newModelSelector(nil)

	candidate := config.ModelCandidate{Id: "c1", Provider: "p", Model: "m1", Priority: 1}
	providers := map[string]config.ProviderConfig{"p": {Url: "http://example"}}

	target := ms.buildModelTarget(candidate, providers)
	if target == nil {
		t.Fatal("expected non-nil modelTarget when provider exists and healthy")
	}
	if target.id != "c1" {
		t.Fatalf("unexpected model id: %s", target.id)
	}
}
