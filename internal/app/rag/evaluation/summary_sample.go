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
	ID                     string   `json:"id"`
	Query                  string   `json:"query"`
	EquivalenceExpectations []string `json:"equivalence_expectations,omitempty"`
}

type SummaryNextTurnEval struct {
	Queries []SummaryNextTurnQuery `json:"queries,omitempty"`
}

type SummarySample struct {
	Name             string                  `json:"name"`
	Tags             []string                `json:"tags,omitempty"`
	Input            SummaryInput            `json:"input"`
	ExpectedSummary  SummaryExpectedSummary  `json:"expected_summary"`
	CriticalContract SummaryCriticalContract `json:"critical_contract"`
	NextTurnEval     SummaryNextTurnEval     `json:"next_turn_eval,omitempty"`
	Metadata         map[string]any          `json:"metadata,omitempty"`
}

func ParseSummarySamples(rawSamples []json.RawMessage) ([]SummarySample, error) {
	samples := make([]SummarySample, 0, len(rawSamples))
	for i, raw := range rawSamples {
		var sample SummarySample
		if err := json.Unmarshal(raw, &sample); err != nil {
			return nil, fmt.Errorf("decode summary sample %d: %w", i, err)
		}
		if err := sample.Validate(); err != nil {
			return nil, fmt.Errorf("validate summary sample %d: %w", i, err)
		}
		sample.Normalize()
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
