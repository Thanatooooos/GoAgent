package history

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type StructuredSummary struct {
	SchemaVersion    int      `json:"schema_version"`
	Goal             string   `json:"goal"`
	ActivePriorities []string `json:"active_priorities,omitempty"`
	UserPreferences  []string `json:"user_preferences,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	EstablishedFacts []string `json:"established_facts,omitempty"`
	RecentProgress   []string `json:"recent_progress,omitempty"`
	OpenQuestions    []string `json:"open_questions,omitempty"`
	BackgroundIssues []string `json:"background_issues,omitempty"`
}

type structuredSummaryWire struct {
	SchemaVersion    any      `json:"schema_version"`
	Goal             string   `json:"goal"`
	ActivePriorities []string `json:"active_priorities,omitempty"`
	UserPreferences  []string `json:"user_preferences,omitempty"`
	Constraints      []string `json:"constraints,omitempty"`
	EstablishedFacts []string `json:"established_facts,omitempty"`
	RecentProgress   []string `json:"recent_progress,omitempty"`
	OpenQuestions    []string `json:"open_questions,omitempty"`
	BackgroundIssues []string `json:"background_issues,omitempty"`
}

func ParseStructuredSummary(raw string) (StructuredSummary, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()

	var wire structuredSummaryWire
	if err := decoder.Decode(&wire); err != nil {
		return StructuredSummary{}, fmt.Errorf("decode structured summary: %w", err)
	}

	summary := StructuredSummary{
		Goal:             wire.Goal,
		ActivePriorities: wire.ActivePriorities,
		UserPreferences:  wire.UserPreferences,
		Constraints:      wire.Constraints,
		EstablishedFacts: wire.EstablishedFacts,
		RecentProgress:   wire.RecentProgress,
		OpenQuestions:    wire.OpenQuestions,
		BackgroundIssues: wire.BackgroundIssues,
	}
	if wire.SchemaVersion != nil {
		version, err := parseSchemaVersion(wire.SchemaVersion)
		if err != nil {
			return StructuredSummary{}, err
		}
		summary.SchemaVersion = version
	}

	summary.Normalize()
	return summary, nil
}

func parseSchemaVersion(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		return normalizeSchemaVersionNumber(typed), nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, fmt.Errorf("decode structured summary: schema_version must be numeric, got %q", typed)
		}
		return normalizeSchemaVersionNumber(parsed), nil
	default:
		return 0, fmt.Errorf("decode structured summary: schema_version has unsupported type %T", value)
	}
}

func normalizeSchemaVersionNumber(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 1
	}
	if value <= 0 {
		return 1
	}
	if math.Trunc(value) == value {
		return int(value)
	}
	return 1
}

func (s *StructuredSummary) Normalize() {
	if s == nil {
		return
	}
	if s.SchemaVersion <= 0 {
		s.SchemaVersion = 1
	}
	s.Goal = strings.TrimSpace(s.Goal)
	s.ActivePriorities = normalizeSummaryItems(s.ActivePriorities)
	s.UserPreferences = normalizeSummaryItems(s.UserPreferences)
	s.Constraints = normalizeSummaryItems(s.Constraints)
	s.EstablishedFacts = normalizeSummaryItems(s.EstablishedFacts)
	s.RecentProgress = normalizeSummaryItems(s.RecentProgress)
	s.OpenQuestions = normalizeSummaryItems(s.OpenQuestions)
	s.BackgroundIssues = normalizeSummaryItems(s.BackgroundIssues)
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
