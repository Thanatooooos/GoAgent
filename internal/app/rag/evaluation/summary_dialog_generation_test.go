package evaluation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSummaryDialogScriptRequiresSequentialTwentyFourTurns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "testdata", "evals", "summary",
		"long_dialogue_questions.json",
	))
	if err != nil {
		t.Fatal(err)
	}

	script, err := ParseSummaryDialogScript(raw)
	if err != nil {
		t.Fatalf("ParseSummaryDialogScript() error = %v", err)
	}
	if script.ScenarioID != "software_project_state_transitions_v1" {
		t.Fatalf("scenario_id = %q", script.ScenarioID)
	}
	if len(script.Turns) != 24 {
		t.Fatalf("turn count = %d, want 24", len(script.Turns))
	}
	for i, turn := range script.Turns {
		if turn.Turn != i+1 {
			t.Fatalf("turn[%d].turn = %d, want %d", i, turn.Turn, i+1)
		}
		if strings.TrimSpace(turn.Phase) == "" ||
			strings.TrimSpace(turn.Purpose) == "" ||
			strings.TrimSpace(turn.User) == "" {
			t.Fatalf("turn[%d] is incomplete: %+v", i, turn)
		}
	}
}
