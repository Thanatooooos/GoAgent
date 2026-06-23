package evaluation

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriteAndLoadSummaryDialogArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dialog.json")
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "scenario",
		Status:        SummaryDialogStatusInProgress,
		Provider:      "configured",
		Model:         "model-a",
		Estimator: SummaryDialogEstimatorMetadata{
			Name: "fixed", Version: "test", MessageOverheadTokens: 4,
		},
		Turns: []SummaryDialogGeneratedTurn{{
			Turn: 1, Phase: "scope", Purpose: "goal",
			User: "question", Assistant: "answer", CumulativeTokens: 28,
		}},
	}
	if err := WriteSummaryDialogArtifact(path, artifact); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSummaryDialogArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifact, loaded) {
		t.Fatalf("artifact mismatch:\nwant: %+v\ngot:  %+v", artifact, loaded)
	}
}

func TestValidateSummaryDialogResumeRejectsDifferentModel(t *testing.T) {
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "scenario",
		Provider:      "configured",
		Model:         "model-a",
		Estimator: SummaryDialogEstimatorMetadata{
			MessageOverheadTokens: 4,
		},
	}
	err := ValidateSummaryDialogResume(
		artifact, "scenario", "configured", "model-b", 4, 24,
	)
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model mismatch, got %v", err)
	}
}

func TestSummaryDialogSuitabilityRequiresDistinctCrossingTurns(t *testing.T) {
	turns := []SummaryDialogGeneratedTurn{
		{Turn: 5, CumulativeTokens: 790},
		{Turn: 6, CumulativeTokens: 810},
		{Turn: 10, CumulativeTokens: 1210},
		{Turn: 15, CumulativeTokens: 1610},
		{Turn: 24, CumulativeTokens: 2500},
	}
	got := EvaluateSummaryDialogSuitability(turns)
	if !got.Suitable || got.CrossedAt["800"] != 6 ||
		got.CrossedAt["1200"] != 10 || got.CrossedAt["1600"] != 15 {
		t.Fatalf("unexpected suitability: %+v", got)
	}
}

func TestSummaryDialogSuitabilityRejectsSameTurnCrossings(t *testing.T) {
	turns := []SummaryDialogGeneratedTurn{
		{Turn: 23, CumulativeTokens: 700},
		{Turn: 24, CumulativeTokens: 2500},
	}
	got := EvaluateSummaryDialogSuitability(turns)
	if got.Suitable {
		t.Fatalf("expected unsuitable result, got %+v", got)
	}
}
