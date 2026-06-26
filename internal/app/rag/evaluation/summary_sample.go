package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/app/rag/domain"
)

type SummaryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SummaryInput struct {
	SourceMessages  []SummaryMessage              `json:"source_messages"`
	PreviousSummary *raghistory.StructuredSummary `json:"previous_summary,omitempty"`
}

type SummaryExpectedField struct {
	MustCover    []string `json:"must_cover,omitempty"`
	ShouldCover  []string `json:"should_cover,omitempty"`
	MustNotClaim []string `json:"must_not_claim,omitempty"`
}

type SummaryExpectedSummary struct {
	Goal             SummaryExpectedField `json:"goal,omitempty"`
	UserPreferences  SummaryExpectedField `json:"user_preferences,omitempty"`
	Constraints      SummaryExpectedField `json:"constraints,omitempty"`
	EstablishedFacts SummaryExpectedField `json:"established_facts,omitempty"`
	RecentProgress   SummaryExpectedField `json:"recent_progress,omitempty"`
	OpenQuestions    SummaryExpectedField `json:"open_questions,omitempty"`
}

type SummaryCriticalContract struct {
	CriticalEntities      []string `json:"critical_entities,omitempty"`
	CriticalConstraints   []string `json:"critical_constraints,omitempty"`
	CriticalFacts         []string `json:"critical_facts,omitempty"`
	CriticalProgress      []string `json:"critical_progress,omitempty"`
	CriticalOpenQuestions []string `json:"critical_open_questions,omitempty"`
	CriticalQueries       []string `json:"critical_queries,omitempty"`
	ForbiddenClaims       []string `json:"forbidden_claims,omitempty"`
}

type SummaryNextTurnQuery struct {
	ID                      string   `json:"id"`
	Query                   string   `json:"query"`
	EquivalenceExpectations []string `json:"equivalence_expectations,omitempty"`
}

type SummaryNextTurnEval struct {
	Queries []SummaryNextTurnQuery `json:"queries,omitempty"`
}

type SummaryStrategyCheckpoint struct {
	AfterTurn        int                     `json:"after_turn"`
	ExpectedSummary  SummaryExpectedSummary  `json:"expected_summary"`
	CriticalContract SummaryCriticalContract `json:"critical_contract"`
	NextTurnEval     SummaryNextTurnEval     `json:"next_turn_eval"`
}

type SummaryStrategyEval struct {
	Checkpoints []SummaryStrategyCheckpoint `json:"checkpoints,omitempty"`
	FinalEval   *SummaryStrategyCheckpoint  `json:"final_eval,omitempty"`
}

type SummarySample struct {
	Name             string                  `json:"name"`
	Tags             []string                `json:"tags,omitempty"`
	Input            SummaryInput            `json:"input"`
	ExpectedSummary  SummaryExpectedSummary  `json:"expected_summary"`
	CriticalContract SummaryCriticalContract `json:"critical_contract"`
	NextTurnEval     SummaryNextTurnEval     `json:"next_turn_eval,omitempty"`
	StrategyEval     *SummaryStrategyEval    `json:"strategy_eval,omitempty"`
	Metadata         map[string]any          `json:"metadata,omitempty"`
}

func ParseSummarySamples(rawSamples []json.RawMessage) ([]SummarySample, error) {
	samples := make([]SummarySample, 0, len(rawSamples))
	for i, raw := range rawSamples {
		var sample SummarySample
		if err := json.Unmarshal(raw, &sample); err != nil {
			return nil, fmt.Errorf("decode summary sample %d: %w", i, err)
		}
		sample.Normalize()
		if err := sample.Validate(); err != nil {
			return nil, fmt.Errorf("validate summary sample %d: %w", i, err)
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

func (s *SummarySample) Normalize() {
	if s == nil {
		return
	}
	s.Name = strings.TrimSpace(s.Name)
	s.Tags = normalizeTags(s.Tags)
	for i := range s.Input.SourceMessages {
		s.Input.SourceMessages[i].Role = strings.TrimSpace(s.Input.SourceMessages[i].Role)
		s.Input.SourceMessages[i].Content = strings.TrimSpace(s.Input.SourceMessages[i].Content)
	}
	normalizeSummaryExpectedField(&s.ExpectedSummary.Goal)
	normalizeSummaryExpectedField(&s.ExpectedSummary.UserPreferences)
	normalizeSummaryExpectedField(&s.ExpectedSummary.Constraints)
	normalizeSummaryExpectedField(&s.ExpectedSummary.EstablishedFacts)
	normalizeSummaryExpectedField(&s.ExpectedSummary.RecentProgress)
	normalizeSummaryExpectedField(&s.ExpectedSummary.OpenQuestions)
	s.CriticalContract.CriticalEntities = normalizeSummaryValues(s.CriticalContract.CriticalEntities)
	s.CriticalContract.CriticalConstraints = normalizeSummaryValues(s.CriticalContract.CriticalConstraints)
	s.CriticalContract.CriticalFacts = normalizeSummaryValues(s.CriticalContract.CriticalFacts)
	s.CriticalContract.CriticalProgress = normalizeSummaryValues(s.CriticalContract.CriticalProgress)
	s.CriticalContract.CriticalOpenQuestions = normalizeSummaryValues(s.CriticalContract.CriticalOpenQuestions)
	s.CriticalContract.CriticalQueries = normalizeSummaryValues(s.CriticalContract.CriticalQueries)
	s.CriticalContract.ForbiddenClaims = normalizeSummaryValues(s.CriticalContract.ForbiddenClaims)
	for i := range s.NextTurnEval.Queries {
		s.NextTurnEval.Queries[i].ID = strings.TrimSpace(s.NextTurnEval.Queries[i].ID)
		s.NextTurnEval.Queries[i].Query = strings.TrimSpace(s.NextTurnEval.Queries[i].Query)
		s.NextTurnEval.Queries[i].EquivalenceExpectations = normalizeSummaryValues(s.NextTurnEval.Queries[i].EquivalenceExpectations)
	}
	if s.StrategyEval != nil {
		turnCount := countSummaryTurns(s.Input.SourceMessages)
		for i := range s.StrategyEval.Checkpoints {
			normalizeSummaryStrategyCheckpoint(&s.StrategyEval.Checkpoints[i])
		}
		if s.StrategyEval.FinalEval != nil {
			if s.StrategyEval.FinalEval.AfterTurn <= 0 {
				s.StrategyEval.FinalEval.AfterTurn = turnCount
			}
			normalizeSummaryStrategyCheckpoint(s.StrategyEval.FinalEval)
		}
	}
}

func (s SummarySample) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("sample name is required")
	}
	if len(s.Input.SourceMessages) == 0 {
		return fmt.Errorf("sample %q source_messages is required", s.Name)
	}
	for _, msg := range s.Input.SourceMessages {
		if strings.TrimSpace(msg.Content) == "" {
			return fmt.Errorf("sample %q source_messages content is required", s.Name)
		}
	}
	if s.StrategyEval != nil {
		turnCount := countSummaryTurns(s.Input.SourceMessages)
		for i, checkpoint := range s.StrategyEval.Checkpoints {
			if err := validateSummaryStrategyCheckpoint(turnCount, checkpoint); err != nil {
				return fmt.Errorf("sample %q checkpoint %d: %w", s.Name, i, err)
			}
		}
		if s.StrategyEval.FinalEval != nil {
			if err := validateSummaryStrategyCheckpoint(turnCount, *s.StrategyEval.FinalEval); err != nil {
				return fmt.Errorf("sample %q final_eval: %w", s.Name, err)
			}
		}
	}
	return nil
}

func (s SummarySample) ToDomainMessages() []domain.ConversationMessage {
	if len(s.Input.SourceMessages) == 0 {
		return nil
	}
	messages := make([]domain.ConversationMessage, 0, len(s.Input.SourceMessages))
	for _, msg := range s.Input.SourceMessages {
		content := strings.TrimSpace(msg.Content)
		role := strings.TrimSpace(msg.Role)
		if content == "" || role == "" {
			continue
		}
		messages = append(messages, domain.ConversationMessage{
			Role:    role,
			Content: content,
		})
	}
	return messages
}

func countSummaryTurns(messages []SummaryMessage) int {
	turns := 0
	for i := 0; i+1 < len(messages); i += 2 {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") && strings.EqualFold(strings.TrimSpace(messages[i+1].Role), "assistant") {
			turns++
		}
	}
	return turns
}

func validateSummaryStrategyCheckpoint(turnCount int, checkpoint SummaryStrategyCheckpoint) error {
	if checkpoint.AfterTurn <= 0 {
		return fmt.Errorf("after_turn must be positive")
	}
	if turnCount > 0 && checkpoint.AfterTurn > turnCount {
		return fmt.Errorf("after_turn=%d exceeds conversation turn count %d", checkpoint.AfterTurn, turnCount)
	}
	return nil
}

func normalizeSummaryStrategyCheckpoint(checkpoint *SummaryStrategyCheckpoint) {
	if checkpoint == nil {
		return
	}
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.Goal)
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.UserPreferences)
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.Constraints)
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.EstablishedFacts)
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.RecentProgress)
	normalizeSummaryExpectedField(&checkpoint.ExpectedSummary.OpenQuestions)
	checkpoint.CriticalContract.CriticalEntities = normalizeSummaryValues(checkpoint.CriticalContract.CriticalEntities)
	checkpoint.CriticalContract.CriticalConstraints = normalizeSummaryValues(checkpoint.CriticalContract.CriticalConstraints)
	checkpoint.CriticalContract.CriticalFacts = normalizeSummaryValues(checkpoint.CriticalContract.CriticalFacts)
	checkpoint.CriticalContract.CriticalProgress = normalizeSummaryValues(checkpoint.CriticalContract.CriticalProgress)
	checkpoint.CriticalContract.CriticalOpenQuestions = normalizeSummaryValues(checkpoint.CriticalContract.CriticalOpenQuestions)
	checkpoint.CriticalContract.CriticalQueries = normalizeSummaryValues(checkpoint.CriticalContract.CriticalQueries)
	checkpoint.CriticalContract.ForbiddenClaims = normalizeSummaryValues(checkpoint.CriticalContract.ForbiddenClaims)
	for i := range checkpoint.NextTurnEval.Queries {
		checkpoint.NextTurnEval.Queries[i].ID = strings.TrimSpace(checkpoint.NextTurnEval.Queries[i].ID)
		checkpoint.NextTurnEval.Queries[i].Query = strings.TrimSpace(checkpoint.NextTurnEval.Queries[i].Query)
		checkpoint.NextTurnEval.Queries[i].EquivalenceExpectations = normalizeSummaryValues(checkpoint.NextTurnEval.Queries[i].EquivalenceExpectations)
	}
}

func normalizeSummaryExpectedField(field *SummaryExpectedField) {
	if field == nil {
		return
	}
	field.MustCover = normalizeSummaryValues(field.MustCover)
	field.ShouldCover = normalizeSummaryValues(field.ShouldCover)
	field.MustNotClaim = normalizeSummaryValues(field.MustNotClaim)
}

func normalizeSummaryValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
