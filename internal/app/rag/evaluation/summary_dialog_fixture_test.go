package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLongSummaryStrategyFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "evals", "summary", "strategy_long_samples.json"))
	if err != nil {
		t.Fatalf("ReadFile(strategy_long_samples.json) error = %v", err)
	}

	rawSamples, err := ExtractSampleArray(raw)
	if err != nil {
		t.Fatalf("ExtractSampleArray() error = %v", err)
	}
	if len(rawSamples) != 1 {
		t.Fatalf("rawSamples len = %d, want 1", len(rawSamples))
	}

	samples, err := ParseSummarySamples(rawSamples)
	if err != nil {
		t.Fatalf("ParseSummarySamples() error = %v", err)
	}

	sample := samples[0]
	if got := len(sample.Input.SourceMessages); got != 48 {
		t.Fatalf("source_messages len = %d, want 48", got)
	}
	if sample.StrategyEval == nil {
		t.Fatal("StrategyEval is nil")
	}
	if got := len(sample.StrategyEval.Checkpoints); got != 3 {
		t.Fatalf("checkpoints len = %d, want 3", got)
	}
	if sample.StrategyEval.FinalEval == nil {
		t.Fatal("FinalEval is nil")
	}
	if got := sample.StrategyEval.FinalEval.AfterTurn; got != 24 {
		t.Fatalf("final_eval after_turn = %d, want 24", got)
	}
	if len(sample.CriticalContract.ForbiddenClaims) == 0 {
		t.Fatal("final critical contract should include forbidden claims")
	}
}

func TestGeneratedLongSummaryDialogArtifactFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "testdata", "evals", "summary", "generated", "software_project_state_transitions_v1.json"))
	if err != nil {
		t.Fatalf("ReadFile(generated artifact) error = %v", err)
	}

	var artifact SummaryDialogArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if artifact.Status != SummaryDialogStatusComplete {
		t.Fatalf("status = %q, want %q", artifact.Status, SummaryDialogStatusComplete)
	}
	if got := len(artifact.Turns); got != 24 {
		t.Fatalf("turns len = %d, want 24", got)
	}
	if !artifact.Suitability.Suitable {
		t.Fatalf("artifact should be suitable: %#v", artifact.Suitability)
	}
	if artifact.Suitability.FinalTokens < 2400 {
		t.Fatalf("final tokens = %d, want >= 2400", artifact.Suitability.FinalTokens)
	}
}
