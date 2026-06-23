package evaluation

import "fmt"

func BuildSummaryDialogReviewDraft(artifact SummaryDialogArtifact) (SummarySample, error) {
	if artifact.Status != SummaryDialogStatusComplete || len(artifact.Turns) != 24 {
		return SummarySample{}, fmt.Errorf("completed 24-turn artifact is required")
	}
	messages := make([]SummaryMessage, 0, 48)
	for _, turn := range artifact.Turns {
		messages = append(messages,
			SummaryMessage{Role: "user", Content: turn.User},
			SummaryMessage{Role: "assistant", Content: turn.Assistant},
		)
	}
	checkpoints := make([]SummaryStrategyCheckpoint, 0, 4)
	for _, afterTurn := range []int{6, 12, 18, 24} {
		checkpoints = append(checkpoints, SummaryStrategyCheckpoint{AfterTurn: afterTurn})
	}
	return SummarySample{
		Name: "software_project_state_transitions_long_dialogue",
		Tags: []string{
			"strategy", "long_dialog", "state_override",
			"goal_shift", "open_questions",
		},
		Input: SummaryInput{SourceMessages: messages},
		StrategyEval: &SummaryStrategyEval{
			Checkpoints: checkpoints,
			FinalEval:  &SummaryStrategyCheckpoint{AfterTurn: 24},
		},
		Metadata: map[string]any{
			"scenario_id":   artifact.ScenarioID,
			"source_model":  artifact.Model,
			"source_tokens": artifact.Suitability.FinalTokens,
			"review_status": "annotations_required",
		},
	}, nil
}
