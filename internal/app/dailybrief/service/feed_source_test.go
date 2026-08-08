package service

import (
	"context"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
	"local/rag-project/internal/app/dailybrief/port"
)

func mustFeedSource(t *testing.T, key string) port.SourceProvider {
	t.Helper()
	spec, ok := domain.SourceFeedSpecByKey(key)
	if !ok {
		t.Fatalf("missing feed spec for key %q", key)
	}
	provider, err := NewFeedSource(spec)
	if err != nil {
		t.Fatalf("NewFeedSource(%q): %v", key, err)
	}
	return provider
}

func TestFeedSourceParsesRSSFixture(t *testing.T) {
	t.Parallel()

	spec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyHackerNews)
	source := mustFeedSource(t, domain.SourceKeyHackerNews)
	client := &staticHTTPClient{
		bodies: map[string][]byte{
			spec.URL: loadSourceFixture(t, "hacker-news.rss.xml"),
		},
	}

	candidates, err := source.Fetch(context.Background(), client)
	if err != nil {
		t.Fatalf("expected rss fetch to succeed, got error %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Source != domain.SourceKeyHackerNews {
		t.Fatalf("expected source key %q, got %q", domain.SourceKeyHackerNews, candidates[0].Source)
	}
	if candidates[0].Topic != domain.TopicKeyTechDev {
		t.Fatalf("expected topic %q, got %q", domain.TopicKeyTechDev, candidates[0].Topic)
	}
}

func TestFeedSourceParsesAtomFixture(t *testing.T) {
	t.Parallel()

	spec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyArxivCSAI)
	source := mustFeedSource(t, domain.SourceKeyArxivCSAI)
	client := &staticHTTPClient{
		bodies: map[string][]byte{
			spec.URL: loadSourceFixture(t, "arxiv-cs-ai.atom.xml"),
		},
	}

	candidates, err := source.Fetch(context.Background(), client)
	if err != nil {
		t.Fatalf("expected atom fetch to succeed, got error %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	for _, candidate := range candidates {
		if candidate.Source != domain.SourceKeyArxivCSAI {
			t.Fatalf("expected source key %q, got %q", domain.SourceKeyArxivCSAI, candidate.Source)
		}
		if candidate.Topic != domain.TopicKeyTechAIResearch {
			t.Fatalf("expected topic %q, got %q", domain.TopicKeyTechAIResearch, candidate.Topic)
		}
	}
}

func TestFeedSourceParsesOpenAIBlogFixture(t *testing.T) {
	t.Parallel()

	spec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyOpenAIBlog)
	source := mustFeedSource(t, domain.SourceKeyOpenAIBlog)
	client := &staticHTTPClient{
		bodies: map[string][]byte{
			spec.URL: loadSourceFixture(t, "openai-blog.rss.xml"),
		},
	}

	candidates, err := source.Fetch(context.Background(), client)
	if err != nil {
		t.Fatalf("expected openai blog fetch to succeed, got error %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Source != domain.SourceKeyOpenAIBlog {
		t.Fatalf("expected source key %q, got %q", domain.SourceKeyOpenAIBlog, candidates[0].Source)
	}
}

func TestFeedSourceParsesMetaAIHTMLFixture(t *testing.T) {
	t.Parallel()

	spec, _ := domain.SourceFeedSpecByKey(domain.SourceKeyMetaAIBlog)
	source := mustFeedSource(t, domain.SourceKeyMetaAIBlog)
	client := &staticHTTPClient{
		bodies: map[string][]byte{
			spec.URL: loadSourceFixture(t, "meta-ai-blog.html"),
		},
	}

	candidates, err := source.Fetch(context.Background(), client)
	if err != nil {
		t.Fatalf("expected meta ai fetch to succeed, got error %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 parsed candidates, got %d", len(candidates))
	}
	if candidates[0].Metadata["format"] != "html" {
		t.Fatalf("expected html format metadata, got %#v", candidates[0].Metadata)
	}
}

func TestFeedSourceParsesNewCatalogFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sourceKey string
		fixture   string
		topic     string
	}{
		{domain.SourceKeyGitHubTrending, "github-trending.rss.xml", domain.TopicKeyTechDev},
		{domain.SourceKeyGoogleDeepMind, "google-deepmind-blog.rss.xml", domain.TopicKeyTechAIModels},
		{domain.SourceKeyTheDecoder, "the-decoder.rss.xml", domain.TopicKeyTechStartups},
		{domain.SourceKeyVentureBeatAI, "venturebeat-ai.rss.xml", domain.TopicKeyTechStartups},
		{domain.SourceKeyTechCrunchAI, "techcrunch-ai.rss.xml", domain.TopicKeyTechStartups},
		{domain.SourceKeyPapersWithCode, "huggingface-papers.html", domain.TopicKeyTechAITools},
		{domain.SourceKeyDezeen, "dezeen.rss.xml", domain.TopicKeyArtDesign},
		{domain.SourceKeyHyperallergic, "hyperallergic.rss.xml", domain.TopicKeyArtContemporary},
		{domain.SourceKeyVariety, "variety.rss.xml", domain.TopicKeyArtFilm},
		{domain.SourceKeyPetaPixel, "petapixel.rss.xml", domain.TopicKeyArtPhotography},
		{domain.SourceKeyArchDaily, "archdaily.rss.xml", domain.TopicKeyArtArchitecture},
		{domain.SourceKeyColossal, "colossal.rss.xml", domain.TopicKeyArtDigital},
		{domain.SourceKeyMusicBusinessWorldwide, "music-business-worldwide.rss.xml", domain.TopicKeyMusicIndustry},
		{domain.SourceKeyMixmag, "mixmag.rss.xml", domain.TopicKeyMusicElectronic},
		{domain.SourceKeyPitchfork, "pitchfork.rss.xml", domain.TopicKeyMusicRockPop},
		{domain.SourceKeySlippedDisc, "slipped-disc.rss.xml", domain.TopicKeyMusicClassical},
		{domain.SourceKeyMusicAlly, "music-ally.rss.xml", domain.TopicKeyMusicTech},
		{domain.SourceKeyBillboard, "billboard.rss.xml", domain.TopicKeyMusicLive},
		{domain.SourceKeyBBCWorld, "bbc-world.rss.xml", domain.TopicKeyPoliticsGlobal},
		{domain.SourceKeySCMP, "scmp.rss.xml", domain.TopicKeyPoliticsChina},
		{domain.SourceKeyTechCrunchPolicy, "techcrunch-policy.rss.xml", domain.TopicKeyPoliticsTechPolicy},
		{domain.SourceKeyFTWorldEconomy, "ft-world-economy.rss.xml", domain.TopicKeyPoliticsEconomyPolicy},
		{domain.SourceKeyCarbonBrief, "carbon-brief.rss.xml", domain.TopicKeyPoliticsEnergy},
		{domain.SourceKeyBBCPolitics, "bbc-politics.rss.xml", domain.TopicKeyPoliticsElections},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.sourceKey, func(t *testing.T) {
			t.Parallel()

			spec, _ := domain.SourceFeedSpecByKey(tc.sourceKey)
			source := mustFeedSource(t, tc.sourceKey)
			client := &staticHTTPClient{
				bodies: map[string][]byte{
					spec.URL: loadSourceFixture(t, tc.fixture),
				},
			}

			candidates, err := source.Fetch(context.Background(), client)
			if err != nil {
				t.Fatalf("expected fetch to succeed, got error %v", err)
			}
			if len(candidates) != 2 {
				t.Fatalf("expected 2 candidates, got %d", len(candidates))
			}
			if candidates[0].Source != tc.sourceKey {
				t.Fatalf("expected source key %q, got %q", tc.sourceKey, candidates[0].Source)
			}
			if candidates[0].Topic != tc.topic {
				t.Fatalf("expected topic %q, got %q", tc.topic, candidates[0].Topic)
			}
			if candidates[0].URL == "" || candidates[0].Title == "" {
				t.Fatalf("expected non-empty title and url, got %#v", candidates[0])
			}
		})
	}
}

func TestFeedSourceStubReturnsEmpty(t *testing.T) {
	t.Parallel()

	spec := domain.SourceFeedSpec{
		Key:    domain.SourceKeyHackerNews,
		Topic:  domain.TopicKeyTechDev,
		Format: domain.SourceFeedFormatStub,
	}
	source, err := NewFeedSource(spec)
	if err != nil {
		t.Fatalf("NewFeedSource(stub): %v", err)
	}
	candidates, err := source.Fetch(context.Background(), &staticHTTPClient{})
	if err != nil {
		t.Fatalf("expected stub fetch to succeed, got error %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected stub source to return no candidates, got %d", len(candidates))
	}
}

func TestDefaultSourceRegistryRegistersAllCatalogSources(t *testing.T) {
	t.Parallel()

	registry, err := NewDefaultSourceRegistry()
	if err != nil {
		t.Fatalf("NewDefaultSourceRegistry returned error: %v", err)
	}
	for _, key := range domain.SourceKeys() {
		if _, ok := registry.Get(key); !ok {
			t.Fatalf("expected registry to include source %q", key)
		}
	}
}
