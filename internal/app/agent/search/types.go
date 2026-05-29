package search

type SearchResultItem struct {
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Snippet       string   `json:"snippet"`
	Domain        string   `json:"domain"`
	SourceType    string   `json:"source_type"`
	Policy        string   `json:"policy"`
	RiskFlags     []string `json:"risk_flags,omitempty"`
	Reasons       []string `json:"reasons,omitempty"`
	ProviderScore *float64 `json:"provider_score,omitempty"`
}

type SearchOutput struct {
	Query                string             `json:"query"`
	Provider             string             `json:"provider"`
	ProviderActual       string             `json:"provider_actual,omitempty"`
	ProviderFallbackUsed bool               `json:"provider_fallback_used"`
	ResultCount          int                `json:"result_count"`
	AllowedCount         int                `json:"allowed_count"`
	NeutralCount         int                `json:"neutral_count"`
	DeniedCount          int                `json:"denied_count"`
	URLs                 []string           `json:"urls,omitempty"`
	Results              []SearchResultItem `json:"results"`
	Summary              string             `json:"summary"`
	Degraded             bool               `json:"degraded,omitempty"`
	DegradeReason        string             `json:"degrade_reason,omitempty"`
	ErrorMessage         string             `json:"error_message,omitempty"`
}
