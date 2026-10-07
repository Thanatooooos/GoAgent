package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestValidateBriefArtifactAcceptsValidPayload(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Daily AI Brief",
		TopSummary: "Top stories across models and research.",
		Sections: []domain.BriefSection{
			{
				Key:   "tech.ai.models",
				Title: "AI Models",
				Items: []domain.BriefItemDraft{
					{
						Title:        "New model release",
						Summary:      "A provider shipped a new model.",
						WhyItMatters: "It changes the cost curve for production apps.",
						URL:          "https://example.com/model",
						Source:       domain.SourceKeyOpenAIBlog,
						Topic:        domain.TopicKeyTechAIModels,
					},
				},
			},
		},
	}
	if err := ValidateBriefArtifact(artifact, 5); err != nil {
		t.Fatalf("expected valid artifact, got %v", err)
	}
}

func TestValidateBriefArtifactRejectsMissingItems(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Daily AI Brief",
		TopSummary: "Top stories.",
		Sections: []domain.BriefSection{
			{Key: "tech.ai.models", Title: "AI Models", Items: []domain.BriefItemDraft{}},
		},
	}
	if err := ValidateBriefArtifact(artifact, 5); err == nil {
		t.Fatal("expected empty section items to be rejected")
	}
}

func TestTruncateBriefArtifactPerTopicLimitsEachSection(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Brief",
		TopSummary: "Summary",
		Sections: []domain.BriefSection{
			{
				Key:   domain.TopicKeyTechStartups,
				Title: "Startups",
				Items: []domain.BriefItemDraft{
					{Title: "s1", Summary: "a", WhyItMatters: "b", URL: "https://example.com/s1", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
					{Title: "s2", Summary: "a", WhyItMatters: "b", URL: "https://example.com/s2", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
					{Title: "s3", Summary: "a", WhyItMatters: "b", URL: "https://example.com/s3", Source: domain.SourceKeyTechCrunchAI, Topic: domain.TopicKeyTechStartups},
				},
			},
			{
				Key:   domain.TopicKeyTechDev,
				Title: "Dev",
				Items: []domain.BriefItemDraft{
					{Title: "d1", Summary: "a", WhyItMatters: "b", URL: "https://example.com/d1", Source: domain.SourceKeyHackerNews, Topic: domain.TopicKeyTechDev},
					{Title: "d2", Summary: "a", WhyItMatters: "b", URL: "https://example.com/d2", Source: domain.SourceKeyHackerNews, Topic: domain.TopicKeyTechDev},
				},
			},
		},
	}

	truncated := TruncateBriefArtifactPerTopic(artifact, 2, 4)
	if len(truncated.Sections[0].Items) != 2 {
		t.Fatalf("expected 2 startup items, got %d", len(truncated.Sections[0].Items))
	}
	if len(truncated.Sections[1].Items) != 2 {
		t.Fatalf("expected 2 dev items, got %d", len(truncated.Sections[1].Items))
	}
}

func TestAlignBriefArtifactWithCandidatesFixesSourceAndTopic(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Brief",
		TopSummary: "Summary",
		Sections: []domain.BriefSection{
			{
				Key:   "tech.startups",
				Title: "?????",
				Items: []domain.BriefItemDraft{
					{
						Title:        "Story",
						Summary:      "Longer summary",
						WhyItMatters: "Impact",
						URL:          "https://techcrunch.com/2026/06/29/example",
						Source:       "techcrunch-aidelphi-tech",
						Topic:        "industry",
					},
				},
			},
		},
	}
	candidates := []domain.Candidate{
		{
			URL:    "https://techcrunch.com/2026/06/29/example",
			Source: domain.SourceKeyTechCrunchAI,
			Topic:  domain.TopicKeyTechStartups,
		},
	}

	aligned := AlignBriefArtifactWithCandidates(artifact, candidates)
	item := aligned.Sections[0].Items[0]
	if item.Source != domain.SourceKeyTechCrunchAI {
		t.Fatalf("expected source correction, got %q", item.Source)
	}
	if item.Topic != domain.TopicKeyTechStartups {
		t.Fatalf("expected topic correction, got %q", item.Topic)
	}
}

func TestAlignBriefArtifactRepairsMissingSummaryAndWhyItMattersFromCandidate(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Brief",
		TopSummary: "Summary",
		Sections: []domain.BriefSection{
			{
				Key:   domain.TopicKeyTechAIModels,
				Title: "Models",
				Items: []domain.BriefItemDraft{
					{
						Title: "Story",
						URL:   "https://openai.com/blog/example",
					},
				},
			},
		},
	}
	candidates := []domain.Candidate{
		{
			Title:          "Story",
			URL:            "https://openai.com/blog/example",
			Source:         domain.SourceKeyOpenAIBlog,
			Topic:          domain.TopicKeyTechAIModels,
			SummarySnippet: "A short snippet about the story",
		},
	}

	aligned := AlignBriefArtifactWithCandidates(artifact, candidates)
	item := aligned.Sections[0].Items[0]
	if item.Summary != "A short snippet about the story" {
		t.Fatalf("expected summary to be repaired from candidate snippet, got %q", item.Summary)
	}
	if strings.TrimSpace(item.WhyItMatters) == "" {
		t.Fatalf("expected whyItMatters to be filled with fallback, got %q", item.WhyItMatters)
	}
	if item.Source != domain.SourceKeyOpenAIBlog {
		t.Fatalf("expected source repaired, got %q", item.Source)
	}
	if item.Topic != domain.TopicKeyTechAIModels {
		t.Fatalf("expected topic repaired, got %q", item.Topic)
	}
}

func TestAlignBriefArtifactMatchesCandidateByTitleWhenURLMissing(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Brief",
		TopSummary: "Summary",
		Sections: []domain.BriefSection{
			{
				Key:   domain.TopicKeyTechDev,
				Title: "Dev",
				Items: []domain.BriefItemDraft{
					{
						Title:   "Hacker News Story",
						Summary: "existing summary",
					},
				},
			},
		},
	}
	candidates := []domain.Candidate{
		{
			Title:          "Hacker News Story",
			URL:            "https://news.ycombinator.com/item?id=1",
			Source:         domain.SourceKeyHackerNews,
			Topic:          domain.TopicKeyTechDev,
			SummarySnippet: "snippet",
		},
	}

	aligned := AlignBriefArtifactWithCandidates(artifact, candidates)
	item := aligned.Sections[0].Items[0]
	if item.URL != "https://news.ycombinator.com/item?id=1" {
		t.Fatalf("expected url repaired by title match, got %q", item.URL)
	}
	if item.Source != domain.SourceKeyHackerNews {
		t.Fatalf("expected source repaired, got %q", item.Source)
	}
	if item.Topic != domain.TopicKeyTechDev {
		t.Fatalf("expected topic repaired, got %q", item.Topic)
	}
	if item.Summary != "existing summary" {
		t.Fatalf("expected existing summary preserved, got %q", item.Summary)
	}
}

func TestAlignBriefArtifactKeepsCompleteItemUntouched(t *testing.T) {
	t.Parallel()

	artifact := domain.BriefArtifact{
		Headline:   "Brief",
		TopSummary: "Summary",
		Sections: []domain.BriefSection{
			{
				Key:   domain.TopicKeyTechAIModels,
				Title: "Models",
				Items: []domain.BriefItemDraft{
					{
						Title:        "Story",
						Summary:      "Full summary",
						WhyItMatters: "Impact",
						URL:          "https://openai.com/blog/example",
						Source:       domain.SourceKeyOpenAIBlog,
						Topic:        domain.TopicKeyTechAIModels,
					},
				},
			},
		},
	}
	candidates := []domain.Candidate{
		{
			Title:          "Story",
			URL:            "https://openai.com/blog/example",
			Source:         domain.SourceKeyOpenAIBlog,
			Topic:          domain.TopicKeyTechAIModels,
			SummarySnippet: "snippet",
		},
	}

	aligned := AlignBriefArtifactWithCandidates(artifact, candidates)
	item := aligned.Sections[0].Items[0]
	if item.Summary != "Full summary" || item.WhyItMatters != "Impact" {
		t.Fatalf("expected complete item unchanged, got %+v", item)
	}
	if item.Source != domain.SourceKeyOpenAIBlog || item.Topic != domain.TopicKeyTechAIModels {
		t.Fatalf("expected unchanged source/topic, got %+v", item)
	}
}

func TestParseBriefArtifactExtractsJSONObject(t *testing.T) {
	t.Parallel()

	raw := "Here is the brief:\n{\"headline\":\"Daily AI Brief\",\"topSummary\":\"Summary\",\"sections\":[{\"key\":\"tech.ai.models\",\"title\":\"AI Models\",\"items\":[{\"title\":\"Story\",\"summary\":\"S\",\"whyItMatters\":\"W\",\"url\":\"https://example.com\",\"source\":\"openai-blog\",\"topic\":\"tech.ai.models\"}]}]}"
	artifact, err := ParseBriefArtifact(raw)
	if err != nil {
		t.Fatalf("ParseBriefArtifact returned error: %v", err)
	}
	if artifact.Headline != "Daily AI Brief" {
		t.Fatalf("unexpected headline: %q", artifact.Headline)
	}
	if len(artifact.Sections) != 1 || len(artifact.Sections[0].Items) != 1 {
		t.Fatalf("unexpected sections: %+v", artifact.Sections)
	}
}

func TestParseBriefArtifactFixtureValidatesSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "..", "testdata", "dailybrief", "generation", "valid-artifact.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	artifact, err := ParseBriefArtifact(string(raw))
	if err != nil {
		t.Fatalf("ParseBriefArtifact returned error: %v", err)
	}
	if err := ValidateBriefArtifact(artifact, 5); err != nil {
		t.Fatalf("expected fixture to validate, got %v", err)
	}
}
