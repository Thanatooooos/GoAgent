package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SummaryDialogStatus string

const (
	SummaryDialogStatusInProgress SummaryDialogStatus = "in_progress"
	SummaryDialogStatusComplete   SummaryDialogStatus = "complete"
)

type SummaryDialogEstimatorMetadata struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	MessageOverheadTokens int    `json:"message_overhead_tokens"`
}

type SummaryDialogGeneratedTurn struct {
	Turn             int       `json:"turn"`
	Phase            string    `json:"phase"`
	Purpose          string    `json:"purpose"`
	User             string    `json:"user"`
	Assistant        string    `json:"assistant"`
	CumulativeTokens int       `json:"cumulative_tokens"`
	GeneratedAt      time.Time `json:"generated_at"`
}

type SummaryDialogSuitability struct {
	Suitable    bool           `json:"suitable"`
	CrossedAt   map[string]int `json:"crossed_at"`
	FinalTokens int            `json:"final_tokens"`
	Reasons     []string       `json:"reasons,omitempty"`
}

type SummaryDialogArtifact struct {
	SchemaVersion int                            `json:"schema_version"`
	ScenarioID    string                         `json:"scenario_id"`
	Status        SummaryDialogStatus            `json:"status"`
	Provider      string                         `json:"provider"`
	Model         string                         `json:"model"`
	Estimator     SummaryDialogEstimatorMetadata `json:"estimator"`
	Turns         []SummaryDialogGeneratedTurn   `json:"turns"`
	Suitability   SummaryDialogSuitability       `json:"suitability"`
}

func WriteSummaryDialogArtifact(path string, artifact SummaryDialogArtifact) error {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary dialog artifact: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure summary dialog directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create summary dialog temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write summary dialog temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close summary dialog temp file: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace summary dialog artifact: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("commit summary dialog artifact: %w", err)
	}
	return nil
}

func LoadSummaryDialogArtifact(path string) (SummaryDialogArtifact, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SummaryDialogArtifact{}, err
	}
	var artifact SummaryDialogArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return SummaryDialogArtifact{}, fmt.Errorf("decode summary dialog artifact: %w", err)
	}
	if err := validateSummaryDialogArtifact(artifact); err != nil {
		return SummaryDialogArtifact{}, err
	}
	return artifact, nil
}

func ValidateSummaryDialogResume(
	artifact SummaryDialogArtifact,
	scenarioID string,
	provider string,
	modelID string,
	messageOverhead int,
	maxTurns int,
) error {
	if artifact.ScenarioID != strings.TrimSpace(scenarioID) {
		return fmt.Errorf("resume scenario mismatch: artifact %q, requested %q", artifact.ScenarioID, scenarioID)
	}
	if artifact.Provider != strings.TrimSpace(provider) {
		return fmt.Errorf("resume provider mismatch: artifact %q, requested %q", artifact.Provider, provider)
	}
	if artifact.Model != strings.TrimSpace(modelID) {
		return fmt.Errorf("resume model mismatch: artifact %q, requested %q", artifact.Model, modelID)
	}
	if artifact.Estimator.MessageOverheadTokens != normalizeSummaryDialogOverhead(messageOverhead) {
		return fmt.Errorf("resume message overhead mismatch: artifact %d, requested %d", artifact.Estimator.MessageOverheadTokens, messageOverhead)
	}
	if maxTurns > 0 && len(artifact.Turns) > maxTurns {
		return fmt.Errorf("resume artifact has %d turns, script has %d", len(artifact.Turns), maxTurns)
	}
	return validateSummaryDialogTurns(artifact.Turns)
}

func EvaluateSummaryDialogSuitability(turns []SummaryDialogGeneratedTurn) SummaryDialogSuitability {
	thresholds := []int{800, 1200, 1600}
	crossedAt := make(map[string]int, len(thresholds))
	finalTokens := 0
	for _, turn := range turns {
		finalTokens = turn.CumulativeTokens
		for _, threshold := range thresholds {
			key := strconv.Itoa(threshold)
			if _, ok := crossedAt[key]; !ok && turn.CumulativeTokens >= threshold {
				crossedAt[key] = turn.Turn
			}
		}
	}
	result := SummaryDialogSuitability{
		CrossedAt:   crossedAt,
		FinalTokens: finalTokens,
	}
	lastTurn := 0
	for _, threshold := range thresholds {
		key := strconv.Itoa(threshold)
		turn, ok := crossedAt[key]
		if !ok {
			result.Reasons = append(result.Reasons, fmt.Sprintf("threshold %d was not crossed", threshold))
			continue
		}
		if turn <= lastTurn {
			result.Reasons = append(result.Reasons, fmt.Sprintf("threshold %d crossed without a later completed turn", threshold))
		}
		lastTurn = turn
	}
	if finalTokens < 2400 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("final tokens %d below 2400", finalTokens))
	}
	result.Suitable = len(result.Reasons) == 0
	return result
}

func validateSummaryDialogArtifact(artifact SummaryDialogArtifact) error {
	if artifact.SchemaVersion != 1 {
		return fmt.Errorf("unsupported artifact schema_version %d", artifact.SchemaVersion)
	}
	if strings.TrimSpace(artifact.ScenarioID) == "" {
		return fmt.Errorf("artifact scenario_id is required")
	}
	if strings.TrimSpace(artifact.Model) == "" {
		return fmt.Errorf("artifact model is required")
	}
	if artifact.Status != SummaryDialogStatusInProgress && artifact.Status != SummaryDialogStatusComplete {
		return fmt.Errorf("unsupported artifact status %q", artifact.Status)
	}
	return validateSummaryDialogTurns(artifact.Turns)
}

func validateSummaryDialogTurns(turns []SummaryDialogGeneratedTurn) error {
	for i, turn := range turns {
		if turn.Turn != i+1 {
			return fmt.Errorf("artifact turn %d must have sequential number %d", i, i+1)
		}
		if strings.TrimSpace(turn.User) == "" {
			return fmt.Errorf("artifact turn %d user is required", turn.Turn)
		}
		if strings.TrimSpace(turn.Assistant) == "" {
			return fmt.Errorf("artifact turn %d assistant is required", turn.Turn)
		}
	}
	return nil
}

func normalizeSummaryDialogOverhead(overhead int) int {
	if overhead < 0 {
		return 0
	}
	return overhead
}
