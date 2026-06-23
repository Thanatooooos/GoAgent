package evaluation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
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

func TestGenerateSummaryDialogCarriesAccumulatedContextAndPersistsEachTurn(t *testing.T) {
	script := validTwentyFourTurnScript()
	chat := &recordingSummaryDialogChat{responses: repeatedAnswers(24)}
	store := &recordingSummaryDialogStore{}

	artifact, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script:                script,
		ModelID:               "model-a",
		Provider:              "configured",
		Chat:                  chat,
		Estimator:             tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4,
		Store:                 store,
		Now:                   fixedSummaryDialogClock,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 24 || len(store.snapshots) != 24 {
		t.Fatalf("requests=%d snapshots=%d", len(chat.requests), len(store.snapshots))
	}
	gotRoles := messageRoles(chat.requests[2].Messages)
	wantRoles := []convention.Role{
		convention.SystemRole,
		convention.UserRole, convention.AssistantRole,
		convention.UserRole, convention.AssistantRole,
		convention.UserRole,
	}
	if !reflect.DeepEqual(gotRoles, wantRoles) {
		t.Fatalf("roles = %v, want %v", gotRoles, wantRoles)
	}
	if artifact.Status != SummaryDialogStatusComplete {
		t.Fatalf("status = %q", artifact.Status)
	}
}

func TestGenerateSummaryDialogResumesWithoutRegeneratingCompletedTurns(t *testing.T) {
	script := validTwentyFourTurnScript()
	existing := SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "scenario",
		Status:        SummaryDialogStatusInProgress,
		Provider:      "configured",
		Model:         "model-a",
		Estimator: SummaryDialogEstimatorMetadata{
			Name:                  "rune",
			MessageOverheadTokens: 4,
		},
	}
	for i := 0; i < 23; i++ {
		existing.Turns = append(existing.Turns, SummaryDialogGeneratedTurn{
			Turn: i + 1, Phase: "phase", Purpose: "purpose",
			User: fmt.Sprintf("q%d", i+1), Assistant: fmt.Sprintf("a%d", i+1),
		})
	}
	chat := &recordingSummaryDialogChat{responses: []string{"a24"}}

	artifact, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script:                script,
		Existing:              &existing,
		ModelID:               "model-a",
		Provider:              "configured",
		Chat:                  chat,
		Estimator:             tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4,
		Store:                 &recordingSummaryDialogStore{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 1 || len(artifact.Turns) != 24 {
		t.Fatalf("requests=%d turns=%d", len(chat.requests), len(artifact.Turns))
	}
}

func TestGenerateSummaryDialogKeepsCheckpointWhenLaterCallFails(t *testing.T) {
	chat := &recordingSummaryDialogChat{
		responses: repeatedAnswers(24),
		errAt:     2,
	}
	store := &recordingSummaryDialogStore{}
	_, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script:                validTwentyFourTurnScript(),
		ModelID:               "model-a",
		Provider:              "configured",
		Chat:                  chat,
		Estimator:             tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4,
		Store:                 store,
	})
	if err == nil || len(store.snapshots) != 1 {
		t.Fatalf("err=%v snapshots=%d", err, len(store.snapshots))
	}
}

func TestGenerateSummaryDialogRejectsEmptyAnswerWithoutAdvancing(t *testing.T) {
	store := &recordingSummaryDialogStore{}
	_, err := GenerateSummaryDialog(context.Background(), SummaryDialogGenerationInput{
		Script:                validTwentyFourTurnScript(),
		ModelID:               "model-a",
		Provider:              "configured",
		Chat:                  &recordingSummaryDialogChat{responses: []string{"   "}},
		Estimator:             tokenbudget.RuneEstimator{},
		MessageOverheadTokens: 4,
		Store:                 store,
	})
	if err == nil || len(store.snapshots) != 0 {
		t.Fatalf("err=%v snapshots=%d", err, len(store.snapshots))
	}
}

type recordingSummaryDialogChat struct {
	responses []string
	requests  []convention.ChatRequest
	errAt     int
}

func (c *recordingSummaryDialogChat) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	index := len(c.requests)
	c.requests = append(c.requests, request)
	if c.errAt > 0 && index+1 == c.errAt {
		return "", errors.New("provider failed")
	}
	if index >= len(c.responses) {
		return "", errors.New("missing fake response")
	}
	return c.responses[index], nil
}

type recordingSummaryDialogStore struct {
	snapshots []SummaryDialogArtifact
}

func (s *recordingSummaryDialogStore) Save(artifact SummaryDialogArtifact) error {
	s.snapshots = append(s.snapshots, artifact)
	return nil
}

func validTwentyFourTurnScript() SummaryDialogScript {
	turns := make([]SummaryDialogTurn, 24)
	for i := range turns {
		turns[i] = SummaryDialogTurn{
			Turn:    i + 1,
			Phase:   "phase",
			Purpose: "purpose",
			User:    fmt.Sprintf("q%d", i+1),
		}
	}
	return SummaryDialogScript{
		SchemaVersion: 1,
		ScenarioID:    "scenario",
		SystemPrompt:  "system",
		Turns:         turns,
	}
}

func repeatedAnswers(count int) []string {
	answers := make([]string, count)
	for i := range answers {
		answers[i] = fmt.Sprintf("a%d", i+1)
	}
	return answers
}

func fixedSummaryDialogClock() time.Time {
	return time.Date(2026, 6, 23, 10, 30, 0, 0, time.UTC)
}

func messageRoles(messages []convention.ChatMessage) []convention.Role {
	roles := make([]convention.Role, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	return roles
}
