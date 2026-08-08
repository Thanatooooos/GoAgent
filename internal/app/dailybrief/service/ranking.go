package service

import (
	"sort"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

var sourcePriority = map[string]int{
	domain.SourceKeyArxivCSAI:      18,
	domain.SourceKeyArxivCSCL:      18,
	domain.SourceKeyArxivCSLG:      18,
	domain.SourceKeyOpenAIBlog:     17,
	domain.SourceKeyAnthropicBlog:  17,
	domain.SourceKeyGoogleDeepMind: 17,
	domain.SourceKeyMetaAIBlog:     17,
	domain.SourceKeyHackerNews:     15,
	domain.SourceKeyPapersWithCode: 14,
	domain.SourceKeyGitHubTrending: 13,
	domain.SourceKeyTheDecoder:     12,
	domain.SourceKeyVentureBeatAI:  11,
	domain.SourceKeyTechCrunchAI:   10,
}

func RankCandidates(candidates []domain.Candidate, selectedTopics []string) []domain.Candidate {
	ranked := append([]domain.Candidate(nil), candidates...)
	if len(ranked) <= 1 {
		return ranked
	}

	selected := make(map[string]struct{}, len(selectedTopics))
	for _, topic := range selectedTopics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue
		}
		selected[trimmed] = struct{}{}
	}

	latest := latestPublishedAt(ranked)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := ranked[i]
		right := ranked[j]
		leftScore := candidateScore(left, latest, selected)
		rightScore := candidateScore(right, latest, selected)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if !left.PublishedAt.Equal(right.PublishedAt) {
			return left.PublishedAt.After(right.PublishedAt)
		}
		if sourcePriority[left.Source] != sourcePriority[right.Source] {
			return sourcePriority[left.Source] > sourcePriority[right.Source]
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Title != right.Title {
			return left.Title < right.Title
		}
		return left.URL < right.URL
	})

	return ranked
}

func latestPublishedAt(candidates []domain.Candidate) time.Time {
	latest := time.Time{}
	for _, candidate := range candidates {
		if candidate.PublishedAt.After(latest) {
			latest = candidate.PublishedAt
		}
	}
	return latest
}

func candidateScore(candidate domain.Candidate, latest time.Time, selectedTopics map[string]struct{}) int {
	score := 0

	// Freshness window: favor content published within the latest 72 hours.
	if !candidate.PublishedAt.IsZero() && !latest.IsZero() {
		diff := latest.Sub(candidate.PublishedAt)
		if diff < 0 {
			diff = 0
		}
		hours := int(diff.Hours())
		if hours < 72 {
			score += 72 - hours
		}
	}

	if _, ok := selectedTopics[candidate.Topic]; ok {
		score += 25
	}

	score += sourcePriority[candidate.Source]
	if len(candidate.SummarySnippet) > 0 {
		score += minInt(len(candidate.SummarySnippet)/20, 10)
	}
	if candidate.ExternalID != "" {
		score += 2
	}
	return score
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
