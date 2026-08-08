package domain

const (
	SourceFeedFormatRSS  = "rss"
	SourceFeedFormatAtom = "atom"
	SourceFeedFormatHTML = "html"
	SourceFeedFormatStub = "stub"
)

// SourceFeedSpec describes how one curated source is fetched and normalized.
// URLs stay code-owned so additions remain reviewed changes.
type SourceFeedSpec struct {
	Key        string
	Topic      string
	Format     string
	URL        string
	HTMLParser string
}

func DefaultSourceFeedSpecs() []SourceFeedSpec {
	return []SourceFeedSpec{
		{Key: SourceKeyHackerNews, Topic: TopicKeyTechDev, Format: SourceFeedFormatRSS, URL: "https://news.ycombinator.com/rss"},
		{Key: SourceKeyGitHubTrending, Topic: TopicKeyTechDev, Format: SourceFeedFormatRSS, URL: "https://cdn.jsdelivr.net/gh/isboyjc/github-trending-api/data/daily/all.xml"},
		{Key: SourceKeyArxivCSAI, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.AI"},
		{Key: SourceKeyArxivCSCL, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.CL"},
		{Key: SourceKeyArxivCSLG, Topic: TopicKeyTechAIResearch, Format: SourceFeedFormatAtom, URL: "https://export.arxiv.org/rss/cs.LG"},
		{Key: SourceKeyPapersWithCode, Topic: TopicKeyTechAITools, Format: SourceFeedFormatHTML, URL: "https://huggingface.co/papers", HTMLParser: "huggingface-papers"},
		{Key: SourceKeyOpenAIBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://openai.com/blog/rss.xml"},
		{Key: SourceKeyAnthropicBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://www.anthropic.com/rss.xml"},
		{Key: SourceKeyGoogleDeepMind, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatRSS, URL: "https://deepmind.google/blog/rss.xml"},
		{Key: SourceKeyMetaAIBlog, Topic: TopicKeyTechAIModels, Format: SourceFeedFormatHTML, URL: "https://ai.meta.com/blog/", HTMLParser: "meta-ai"},
		{Key: SourceKeyTheDecoder, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://the-decoder.com/feed/"},
		{Key: SourceKeyVentureBeatAI, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://venturebeat.com/category/ai/feed"},
		{Key: SourceKeyTechCrunchAI, Topic: TopicKeyTechStartups, Format: SourceFeedFormatRSS, URL: "https://techcrunch.com/category/artificial-intelligence/feed/"},

		{Key: SourceKeyDezeen, Topic: TopicKeyArtDesign, Format: SourceFeedFormatRSS, URL: "https://www.dezeen.com/feed/"},
		{Key: SourceKeyHyperallergic, Topic: TopicKeyArtContemporary, Format: SourceFeedFormatRSS, URL: "https://hyperallergic.com/feed/"},
		{Key: SourceKeyVariety, Topic: TopicKeyArtFilm, Format: SourceFeedFormatRSS, URL: "https://variety.com/feed/"},
		{Key: SourceKeyPetaPixel, Topic: TopicKeyArtPhotography, Format: SourceFeedFormatRSS, URL: "https://petapixel.com/feed/"},
		{Key: SourceKeyArchDaily, Topic: TopicKeyArtArchitecture, Format: SourceFeedFormatRSS, URL: "https://www.archdaily.com/feed"},
		{Key: SourceKeyColossal, Topic: TopicKeyArtDigital, Format: SourceFeedFormatRSS, URL: "https://www.thisiscolossal.com/feed/"},

		{Key: SourceKeyMusicBusinessWorldwide, Topic: TopicKeyMusicIndustry, Format: SourceFeedFormatRSS, URL: "https://www.musicbusinessworldwide.com/feed/"},
		{Key: SourceKeyMixmag, Topic: TopicKeyMusicElectronic, Format: SourceFeedFormatRSS, URL: "https://mixmag.net/rss.xml"},
		{Key: SourceKeyPitchfork, Topic: TopicKeyMusicRockPop, Format: SourceFeedFormatRSS, URL: "https://pitchfork.com/rss/news/"},
		{Key: SourceKeySlippedDisc, Topic: TopicKeyMusicClassical, Format: SourceFeedFormatRSS, URL: "https://slippedisc.com/feed/"},
		{Key: SourceKeyMusicAlly, Topic: TopicKeyMusicTech, Format: SourceFeedFormatRSS, URL: "https://musically.com/feed/"},
		{Key: SourceKeyBillboard, Topic: TopicKeyMusicLive, Format: SourceFeedFormatRSS, URL: "https://www.billboard.com/feed/"},

		{Key: SourceKeyBBCWorld, Topic: TopicKeyPoliticsGlobal, Format: SourceFeedFormatRSS, URL: "https://feeds.bbci.co.uk/news/world/rss.xml"},
		{Key: SourceKeySCMP, Topic: TopicKeyPoliticsChina, Format: SourceFeedFormatRSS, URL: "https://www.scmp.com/rss/91/feed"},
		{Key: SourceKeyTechCrunchPolicy, Topic: TopicKeyPoliticsTechPolicy, Format: SourceFeedFormatRSS, URL: "https://techcrunch.com/tag/government-policy/feed/"},
		{Key: SourceKeyFTWorldEconomy, Topic: TopicKeyPoliticsEconomyPolicy, Format: SourceFeedFormatRSS, URL: "https://www.ft.com/world-economy?format=rss"},
		{Key: SourceKeyCarbonBrief, Topic: TopicKeyPoliticsEnergy, Format: SourceFeedFormatRSS, URL: "https://www.carbonbrief.org/feed"},
		{Key: SourceKeyBBCPolitics, Topic: TopicKeyPoliticsElections, Format: SourceFeedFormatRSS, URL: "https://feeds.bbci.co.uk/news/politics/rss.xml"},
	}
}

func SourceFeedSpecByKey(key string) (SourceFeedSpec, bool) {
	for _, spec := range DefaultSourceFeedSpecs() {
		if spec.Key == key {
			return spec, true
		}
	}
	return SourceFeedSpec{}, false
}
