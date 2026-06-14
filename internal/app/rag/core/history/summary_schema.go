package history

import (
	"encoding/json"
	"fmt"
	"strings"
)

type StructuredSummary struct {
	SchemaVersion    int      `json:"schema_version"`
	Goal             string   `json:"goal"`
	UserPreferences  []string `json:"user_preferences,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	EstablishedFacts []string `json:"established_facts,omitempty"`
	RecentProgress   []string `json:"recent_progress,omitempty"`
	OpenQuestions    []string `json:"open_questions,omitempty"`
}

func ParseStructuredSummary(raw string) (StructuredSummary, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()

	var summary StructuredSummary
	if err := decoder.Decode(&summary); err != nil {
		return StructuredSummary{}, fmt.Errorf("decode structured summary: %w", err)
	}
	summary.Normalize()
	return summary, nil
}

func (s *StructuredSummary) Normalize() {
	if s == nil {
		return
	}
	if s.SchemaVersion <= 0 {
		s.SchemaVersion = 1
	}
	s.Goal = strings.TrimSpace(s.Goal)
	s.UserPreferences = normalizeSummaryItems(s.UserPreferences)
	s.Constraints = normalizeSummaryItems(s.Constraints)
	s.EstablishedFacts = normalizeSummaryItems(s.EstablishedFacts)
	s.RecentProgress = normalizeSummaryItems(s.RecentProgress)
	s.OpenQuestions = normalizeSummaryItems(s.OpenQuestions)
}

func normalizeSummaryItems(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
