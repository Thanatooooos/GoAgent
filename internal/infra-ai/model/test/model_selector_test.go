package test

import (
	"testing"

	aimodel "local/rag-project/internal/infra-ai/model"
)

func TestSelectChatCandidatesOrdersAndFilters(t *testing.T) {
	loadModelTestConfig(t)

	selector := aimodel.NewModelSelector(nil)
	targets := selector.SelectChatCandidates(false)

	if len(targets) != 2 {
		t.Fatalf("expected 2 chat targets, got %d", len(targets))
	}
	if targets[0].Id != "first" {
		t.Fatalf("expected first model to be first choice, got %s", targets[0].Id)
	}
}

func TestSelectChatCandidatesDeepThinking(t *testing.T) {
	loadModelTestConfig(t)

	selector := aimodel.NewModelSelector(nil)
	targets := selector.SelectChatCandidates(true)

	if len(targets) != 1 {
		t.Fatalf("expected 1 deep thinking target, got %d", len(targets))
	}
	if targets[0].Id != "deep" {
		t.Fatalf("expected deep model, got %s", targets[0].Id)
	}
}

func TestSelectEmbeddingCandidates(t *testing.T) {
	loadModelTestConfig(t)

	selector := aimodel.NewModelSelector(nil)
	targets := selector.SelectEmbeddingCandidates()

	if len(targets) != 1 {
		t.Fatalf("expected 1 embedding target, got %d", len(targets))
	}
	if targets[0].Candidate.DimensionInt(0) != 1024 {
		t.Fatalf("expected dimension 1024, got %d", targets[0].Candidate.DimensionInt(0))
	}
}

func TestSelectorSkipsUnavailableTargets(t *testing.T) {
	loadModelTestConfig(t)

	healthStore := aimodel.NewModelHealthStore()
	healthStore.MarkFailure("first")

	selector := aimodel.NewModelSelector(healthStore)
	targets := selector.SelectChatCandidates(false)

	if len(targets) != 1 {
		t.Fatalf("expected 1 available target after health filter, got %d", len(targets))
	}
	if targets[0].Id != "deep" {
		t.Fatalf("expected remaining target to be deep, got %s", targets[0].Id)
	}
}
