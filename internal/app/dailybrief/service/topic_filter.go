package service

import (
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
)

func FilterCandidatesByTopics(candidates []domain.Candidate, topics []string) []domain.Candidate {
	allowed := make(map[string]struct{}, len(topics))
	for _, topic := range topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return append([]domain.Candidate(nil), candidates...)
	}

	filtered := make([]domain.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := allowed[candidate.Topic]; ok {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}
