package service

import (
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/framework/config"
)

const defaultMaxCandidates = 20

type CandidatePipeline struct {
	maxCandidates    int
	maxItemsPerTopic int
}

func NewCandidatePipeline(generationConfig config.DailyBriefGenerationConfig) *CandidatePipeline {
	maxCandidates := generationConfig.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = defaultMaxCandidates
	}
	return &CandidatePipeline{
		maxCandidates:    maxCandidates,
		maxItemsPerTopic: generationConfig.MaxItemsPerTopic,
	}
}

func (p *CandidatePipeline) Process(candidates []domain.Candidate, selectedTopics []string) []domain.Candidate {
	normalized := normalizeCandidates(candidates)
	deduped := DeduplicateCandidates(normalized)
	filtered := FilterCandidatesByTopics(deduped, selectedTopics)
	ranked := RankCandidates(filtered, selectedTopics)
	return LimitCandidatesPerTopic(ranked, selectedTopics, p.maxItemsPerTopic, p.maxCandidates)
}

func normalizeCandidates(candidates []domain.Candidate) []domain.Candidate {
	normalized := make([]domain.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Title = strings.TrimSpace(candidate.Title)
		candidate.URL = NormalizeURL(candidate.URL)
		candidate.Source = strings.TrimSpace(candidate.Source)
		candidate.Topic = strings.TrimSpace(candidate.Topic)
		candidate.ExternalID = strings.TrimSpace(candidate.ExternalID)
		candidate.SummarySnippet = strings.Join(strings.Fields(strings.TrimSpace(candidate.SummarySnippet)), " ")
		if candidate.Topic == "" {
			candidate.Topic = inferTopic(candidate.Source)
		}
		if candidate.Metadata == nil {
			candidate.Metadata = map[string]string{}
		}
		if candidate.Title == "" || candidate.URL == "" || candidate.Source == "" {
			continue
		}
		normalized = append(normalized, candidate)
	}
	return normalized
}

func inferTopic(source string) string {
	if topic := domain.TopicForSourceKey(source); topic != "" {
		return topic
	}
	return domain.TopicKeyTechAIResearch
}
