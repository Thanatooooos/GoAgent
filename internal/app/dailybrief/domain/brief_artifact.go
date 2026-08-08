package domain

type BriefSection struct {
	Key   string           `json:"key"`
	Title string           `json:"title"`
	Items []BriefItemDraft `json:"items"`
}

type BriefItemDraft struct {
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	WhyItMatters string `json:"whyItMatters"`
	URL          string `json:"url"`
	Source       string `json:"source"`
	Topic        string `json:"topic"`
}

type BriefArtifact struct {
	Headline   string         `json:"headline"`
	TopSummary string         `json:"topSummary"`
	Sections   []BriefSection `json:"sections"`
}
