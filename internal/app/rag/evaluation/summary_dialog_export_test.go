package evaluation

import (
	"fmt"
	"testing"
)

func TestBuildSummaryDialogReviewDraftExportsRoleContentAndCheckpoints(t *testing.T) {
	artifact := completedArtifactWithTwentyFourTurns()
	draft, err := BuildSummaryDialogReviewDraft(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Input.SourceMessages) != 48 {
		t.Fatalf("source message count = %d", len(draft.Input.SourceMessages))
	}
	if got := draft.Input.SourceMessages[0]; got.Role != "user" || got.Content != artifact.Turns[0].User {
		t.Fatalf("first message = %+v", got)
	}
	if got := draft.Input.SourceMessages[1]; got.Role != "assistant" || got.Content != artifact.Turns[0].Assistant {
		t.Fatalf("second message = %+v", got)
	}
	want := []int{6, 12, 18, 24}
	for i, checkpoint := range draft.StrategyEval.Checkpoints {
		if checkpoint.AfterTurn != want[i] {
			t.Fatalf("checkpoint[%d] = %d", i, checkpoint.AfterTurn)
		}
	}
	if draft.StrategyEval.FinalEval == nil || draft.StrategyEval.FinalEval.AfterTurn != 24 {
		t.Fatalf("final eval = %+v", draft.StrategyEval.FinalEval)
	}
	if len(draft.ExpectedSummary.Goal.MustCover) != 0 ||
		len(draft.CriticalContract.CriticalFacts) != 0 {
		t.Fatal("export must not invent gold annotations")
	}
}

func completedArtifactWithTwentyFourTurns() SummaryDialogArtifact {
	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "software_project_state_transitions_v1",
		Status:        SummaryDialogStatusComplete,
		Model:         "model-a",
		Suitability: SummaryDialogSuitability{
			Suitable:    true,
			FinalTokens: 2500,
			CrossedAt:   map[string]int{"800": 6, "1200": 10, "1600": 15},
		},
	}
	for i := 0; i < 24; i++ {
		artifact.Turns = append(artifact.Turns, SummaryDialogGeneratedTurn{
			Turn:      i + 1,
			Phase:     "phase",
			Purpose:   "purpose",
			User:      fmt.Sprintf("q%d", i+1),
			Assistant: fmt.Sprintf("a%d", i+1),
		})
	}
	return artifact
}
