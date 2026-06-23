package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"
)

type SummaryDialogScript struct {
	SchemaVersion int                 `json:"schema_version"`
	ScenarioID    string              `json:"scenario_id"`
	SystemPrompt  string              `json:"system_prompt"`
	Turns         []SummaryDialogTurn `json:"turns"`
}

type SummaryDialogTurn struct {
	Turn    int    `json:"turn"`
	Phase   string `json:"phase"`
	Purpose string `json:"purpose"`
	User    string `json:"user"`
}

func ParseSummaryDialogScript(raw []byte) (SummaryDialogScript, error) {
	var script SummaryDialogScript
	if err := json.Unmarshal(raw, &script); err != nil {
		return SummaryDialogScript{}, fmt.Errorf("decode summary dialog script: %w", err)
	}
	script.ScenarioID = strings.TrimSpace(script.ScenarioID)
	script.SystemPrompt = strings.TrimSpace(script.SystemPrompt)
	if script.SchemaVersion != 1 {
		return SummaryDialogScript{}, fmt.Errorf("unsupported script schema_version %d", script.SchemaVersion)
	}
	if script.ScenarioID == "" {
		return SummaryDialogScript{}, fmt.Errorf("scenario_id is required")
	}
	if script.SystemPrompt == "" {
		return SummaryDialogScript{}, fmt.Errorf("system_prompt is required")
	}
	if len(script.Turns) != 24 {
		return SummaryDialogScript{}, fmt.Errorf("script requires exactly 24 turns, got %d", len(script.Turns))
	}
	for i := range script.Turns {
		turn := &script.Turns[i]
		turn.Phase = strings.TrimSpace(turn.Phase)
		turn.Purpose = strings.TrimSpace(turn.Purpose)
		turn.User = strings.TrimSpace(turn.User)
		if turn.Turn != i+1 {
			return SummaryDialogScript{}, fmt.Errorf("turn %d must have sequential number %d", i, i+1)
		}
		if turn.Phase == "" || turn.Purpose == "" || turn.User == "" {
			return SummaryDialogScript{}, fmt.Errorf("turn %d requires phase, purpose, and user", turn.Turn)
		}
	}
	return script, nil
}
