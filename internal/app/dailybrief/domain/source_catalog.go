package domain

const (
	SourceKeyHackerNews     = "hacker-news"
	SourceKeyGitHubTrending = "github-trending"
	SourceKeyArxivCSAI      = "arxiv-cs-ai"
	SourceKeyArxivCSCL      = "arxiv-cs-cl"
	SourceKeyArxivCSLG      = "arxiv-cs-lg"
	SourceKeyPapersWithCode = "papers-with-code"
	SourceKeyOpenAIBlog     = "openai-blog"
	SourceKeyAnthropicBlog  = "anthropic-blog"
	SourceKeyGoogleDeepMind = "google-deepmind-blog"
	SourceKeyMetaAIBlog     = "meta-ai-blog"
	SourceKeyTheDecoder     = "the-decoder"
	SourceKeyVentureBeatAI  = "venturebeat-ai"
	SourceKeyTechCrunchAI   = "techcrunch-ai"

	SourceKeyDezeen                 = "dezeen"
	SourceKeyHyperallergic          = "hyperallergic"
	SourceKeyVariety                = "variety"
	SourceKeyPetaPixel              = "petapixel"
	SourceKeyArchDaily              = "archdaily"
	SourceKeyColossal               = "colossal"
	SourceKeyMusicBusinessWorldwide = "music-business-worldwide"
	SourceKeyMixmag                 = "mixmag"
	SourceKeyPitchfork              = "pitchfork"
	SourceKeySlippedDisc            = "slipped-disc"
	SourceKeyMusicAlly              = "music-ally"
	SourceKeyBillboard              = "billboard"
	SourceKeyBBCWorld               = "bbc-world"
	SourceKeySCMP                   = "scmp"
	SourceKeyTechCrunchPolicy       = "techcrunch-policy"
	SourceKeyFTWorldEconomy         = "ft-world-economy"
	SourceKeyCarbonBrief            = "carbon-brief"
	SourceKeyBBCPolitics            = "bbc-politics"
)

var sourceKeys = []string{
	SourceKeyHackerNews,
	SourceKeyGitHubTrending,
	SourceKeyArxivCSAI,
	SourceKeyArxivCSCL,
	SourceKeyArxivCSLG,
	SourceKeyPapersWithCode,
	SourceKeyOpenAIBlog,
	SourceKeyAnthropicBlog,
	SourceKeyGoogleDeepMind,
	SourceKeyMetaAIBlog,
	SourceKeyTheDecoder,
	SourceKeyVentureBeatAI,
	SourceKeyTechCrunchAI,
	SourceKeyDezeen,
	SourceKeyHyperallergic,
	SourceKeyVariety,
	SourceKeyPetaPixel,
	SourceKeyArchDaily,
	SourceKeyColossal,
	SourceKeyMusicBusinessWorldwide,
	SourceKeyMixmag,
	SourceKeyPitchfork,
	SourceKeySlippedDisc,
	SourceKeyMusicAlly,
	SourceKeyBillboard,
	SourceKeyBBCWorld,
	SourceKeySCMP,
	SourceKeyTechCrunchPolicy,
	SourceKeyFTWorldEconomy,
	SourceKeyCarbonBrief,
	SourceKeyBBCPolitics,
}

var sourceKeySet = newStringSet(sourceKeys)

func SourceKeys() []string {
	return append([]string(nil), sourceKeys...)
}

func IsSourceKeySupported(key string) bool {
	_, ok := sourceKeySet[key]
	return ok
}
