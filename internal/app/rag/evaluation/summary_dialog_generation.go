package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
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

type SummaryDialogChat interface {
	ChatWithModel(convention.ChatRequest, string) (string, error)
}

type SummaryDialogArtifactStore interface {
	Save(SummaryDialogArtifact) error
}

type SummaryDialogGenerationInput struct {
	Script                SummaryDialogScript
	Existing              *SummaryDialogArtifact
	ModelID               string
	Provider              string
	Chat                  SummaryDialogChat
	Estimator             tokenbudget.Estimator
	MessageOverheadTokens int
	Store                 SummaryDialogArtifactStore
	Now                   func() time.Time
}

func ParseSummaryDialogScript(raw []byte) (SummaryDialogScript, error) {
	var script SummaryDialogScript
	if err := json.Unmarshal(raw, &script); err != nil {
		return SummaryDialogScript{}, fmt.Errorf("decode summary dialog script: %w", err)
	}
	if err := normalizeAndValidateSummaryDialogScript(&script); err != nil {
		return SummaryDialogScript{}, err
	}
	return script, nil
}

func GenerateSummaryDialog(ctx context.Context, input SummaryDialogGenerationInput) (SummaryDialogArtifact, error) {
	if err := normalizeAndValidateSummaryDialogScript(&input.Script); err != nil {
		return SummaryDialogArtifact{}, err
	}
	modelID := strings.TrimSpace(input.ModelID)
	if modelID == "" {
		return SummaryDialogArtifact{}, fmt.Errorf("model id is required")
	}
	provider := strings.TrimSpace(input.Provider)
	if provider == "" {
		provider = "configured"
	}
	if input.Chat == nil {
		return SummaryDialogArtifact{}, fmt.Errorf("summary dialog chat is required")
	}
	if input.Store == nil {
		return SummaryDialogArtifact{}, fmt.Errorf("summary dialog store is required")
	}
	estimator := input.Estimator
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	estimatorName, estimatorVersion := tokenbudget.DescribeEstimator(estimator)
	overhead := normalizeSummaryDialogOverhead(input.MessageOverheadTokens)
	now := input.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	artifact := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    input.Script.ScenarioID,
		Status:        SummaryDialogStatusInProgress,
		Provider:      provider,
		Model:         modelID,
		Estimator: SummaryDialogEstimatorMetadata{
			Name:                  estimatorName,
			Version:               estimatorVersion,
			MessageOverheadTokens: overhead,
		},
	}
	if input.Existing != nil {
		artifact = *input.Existing
		if err := ValidateSummaryDialogResume(
			artifact, input.Script.ScenarioID, provider, modelID, overhead, len(input.Script.Turns),
		); err != nil {
			return SummaryDialogArtifact{}, err
		}
		artifact.Status = SummaryDialogStatusInProgress
		artifact.Suitability = EvaluateSummaryDialogSuitability(artifact.Turns)
	}

	for i := len(artifact.Turns); i < len(input.Script.Turns); i++ {
		if err := ctx.Err(); err != nil {
			return artifact, err
		}
		scriptTurn := input.Script.Turns[i]
		request := buildSummaryDialogRequest(input.Script, artifact.Turns, scriptTurn)
		answer, err := input.Chat.ChatWithModel(request, modelID)
		if err != nil {
			return artifact, fmt.Errorf("generate summary dialog turn %d: %w", scriptTurn.Turn, err)
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return artifact, fmt.Errorf("generate summary dialog turn %d: empty answer", scriptTurn.Turn)
		}

		artifact.Turns = append(artifact.Turns, SummaryDialogGeneratedTurn{
			Turn:        scriptTurn.Turn,
			Phase:       scriptTurn.Phase,
			Purpose:     scriptTurn.Purpose,
			User:        scriptTurn.User,
			Assistant:   answer,
			GeneratedAt: now().UTC(),
		})
		artifact.Turns[len(artifact.Turns)-1].CumulativeTokens = estimateGeneratedDialogTokens(artifact.Turns, estimator, overhead)
		if len(artifact.Turns) == len(input.Script.Turns) {
			artifact.Status = SummaryDialogStatusComplete
		}
		artifact.Suitability = EvaluateSummaryDialogSuitability(artifact.Turns)
		if err := input.Store.Save(artifact); err != nil {
			return artifact, fmt.Errorf("save summary dialog turn %d: %w", scriptTurn.Turn, err)
		}
	}
	return artifact, nil
}

func buildSummaryDialogRequest(
	script SummaryDialogScript,
	completed []SummaryDialogGeneratedTurn,
	next SummaryDialogTurn,
) convention.ChatRequest {
	messages := make([]convention.ChatMessage, 0, 2+len(completed)*2)
	messages = append(messages, convention.SystemMessage(script.SystemPrompt))
	for _, turn := range completed {
		messages = append(messages,
			convention.UserMessage(turn.User),
			convention.AssistantMessage(turn.Assistant),
		)
	}
	messages = append(messages, convention.UserMessage(next.User))
	temperature := 0.4
	maxTokens := 800
	thinking := false
	return convention.ChatRequest{
		Messages:    messages,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Thinking:    &thinking,
	}
}

func estimateGeneratedDialogTokens(
	turns []SummaryDialogGeneratedTurn,
	estimator tokenbudget.Estimator,
	overhead int,
) int {
	messages := make([]convention.ChatMessage, 0, len(turns)*2)
	for _, turn := range turns {
		messages = append(messages,
			convention.UserMessage(turn.User),
			convention.AssistantMessage(turn.Assistant),
		)
	}
	return tokenbudget.EstimateMessages(messages, estimator, overhead)
}

func normalizeAndValidateSummaryDialogScript(script *SummaryDialogScript) error {
	if script == nil {
		return fmt.Errorf("summary dialog script is required")
	}
	script.ScenarioID = strings.TrimSpace(script.ScenarioID)
	script.SystemPrompt = strings.TrimSpace(script.SystemPrompt)
	if script.SchemaVersion != 1 {
		return fmt.Errorf("unsupported script schema_version %d", script.SchemaVersion)
	}
	if script.ScenarioID == "" {
		return fmt.Errorf("scenario_id is required")
	}
	if script.SystemPrompt == "" {
		return fmt.Errorf("system_prompt is required")
	}
	if len(script.Turns) != 24 {
		return fmt.Errorf("script requires exactly 24 turns, got %d", len(script.Turns))
	}
	for i := range script.Turns {
		turn := &script.Turns[i]
		turn.Phase = strings.TrimSpace(turn.Phase)
		turn.Purpose = strings.TrimSpace(turn.Purpose)
		turn.User = strings.TrimSpace(turn.User)
		if turn.Turn != i+1 {
			return fmt.Errorf("turn %d must have sequential number %d", i, i+1)
		}
		if turn.Phase == "" || turn.Purpose == "" || turn.User == "" {
			return fmt.Errorf("turn %d requires phase, purpose, and user", turn.Turn)
		}
	}
	return nil
}
