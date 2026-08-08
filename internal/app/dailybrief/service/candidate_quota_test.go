package service

import (
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestLimitCandidatesPerTopicRoundRobin(t *testing.T) {
	t.Parallel()

	ranked := []domain.Candidate{
		{Title: "s1", URL: "https://example.com/s1", Source: "techcrunch-ai", Topic: domain.TopicKeyTechStartups},
		{Title: "s2", URL: "https://example.com/s2", Source: "techcrunch-ai", Topic: domain.TopicKeyTechStartups},
		{Title: "s3", URL: "https://example.com/s3", Source: "techcrunch-ai", Topic: domain.TopicKeyTechStartups},
		{Title: "d1", URL: "https://example.com/d1", Source: "hacker-news", Topic: domain.TopicKeyTechDev},
		{Title: "d2", URL: "https://example.com/d2", Source: "hacker-news", Topic: domain.TopicKeyTechDev},
		{Title: "r1", URL: "https://example.com/r1", Source: "arxiv-cs-ai", Topic: domain.TopicKeyTechAIResearch},
	}

	selected := []string{
		domain.TopicKeyTechAIResearch,
		domain.TopicKeyTechDev,
		domain.TopicKeyTechStartups,
	}
	limited := LimitCandidatesPerTopic(ranked, selected, 2, 6)

	if len(limited) != 5 {
		t.Fatalf("expected 5 candidates (1 research + 2 dev + 2 startups), got %d", len(limited))
	}
	counts := map[string]int{}
	for _, candidate := range limited {
		counts[candidate.Topic]++
	}
	if counts[domain.TopicKeyTechAIResearch] != 1 {
		t.Fatalf("expected 1 research candidate, got %d", counts[domain.TopicKeyTechAIResearch])
	}
	for _, topic := range []string{domain.TopicKeyTechDev, domain.TopicKeyTechStartups} {
		if counts[topic] != 2 {
			t.Fatalf("expected 2 candidates for %s, got %d (%v)", topic, counts[topic], counts)
		}
	}
	if limited[0].Topic != domain.TopicKeyTechAIResearch {
		t.Fatalf("expected first pick from first subscribed topic, got %q", limited[0].Topic)
	}
	if limited[1].Topic != domain.TopicKeyTechDev {
		t.Fatalf("expected second pick from second topic, got %q", limited[1].Topic)
	}
}

func TestLimitCandidatesPerTopicRespectsGlobalMax(t *testing.T) {
	t.Parallel()

	ranked := []domain.Candidate{
		{Title: "a1", URL: "https://example.com/a1", Source: "openai-blog", Topic: domain.TopicKeyTechAIModels},
		{Title: "a2", URL: "https://example.com/a2", Source: "openai-blog", Topic: domain.TopicKeyTechAIModels},
		{Title: "b1", URL: "https://example.com/b1", Source: "hacker-news", Topic: domain.TopicKeyTechDev},
		{Title: "b2", URL: "https://example.com/b2", Source: "hacker-news", Topic: domain.TopicKeyTechDev},
	}
	limited := LimitCandidatesPerTopic(ranked, []string{domain.TopicKeyTechAIModels, domain.TopicKeyTechDev}, 2, 3)
	if len(limited) != 3 {
		t.Fatalf("expected global cap of 3, got %d", len(limited))
	}
}

func TestEffectiveMaxItems(t *testing.T) {
	t.Parallel()

	if got := EffectiveMaxItems(23, 46, 2); got != 46 {
		t.Fatalf("expected 46, got %d", got)
	}
	if got := EffectiveMaxItems(23, 100, 2); got != 46 {
		t.Fatalf("expected per-topic total to cap global max, got %d", got)
	}
	if got := EffectiveMaxItems(5, 0, 2); got != 10 {
		t.Fatalf("expected 10 when global max unset, got %d", got)
	}
}
