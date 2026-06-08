package external_evidence

import agentsearch "local/rag-project/internal/app/agent/search"

const (
	readinessReady        = "ready"
	readinessPartial      = "partial"
	readinessInsufficient = "insufficient"

	defaultMaxReviewedSources = 2
)

// ReviewInput is the deterministic source-review input used inside the
// external-evidence workflow.
type ReviewInput struct {
	Query         string                       `json:"query"`
	Results       []agentsearch.SearchResultItem `json:"results"`
	MaxCandidates int                          `json:"max_candidates,omitempty"`
}

// ReviewResult captures which sources survived review and how ready the
// resulting evidence set is for answer generation.
type ReviewResult struct {
	Selected            []SelectedSource `json:"selected,omitempty"`
	Rejected            []RejectedSource `json:"rejected,omitempty"`
	Readiness           string           `json:"readiness,omitempty"`
	ReadinessConfidence float64          `json:"readiness_confidence,omitempty"`
	ReadinessReasoning  string           `json:"readiness_reasoning,omitempty"`
	CitedURLs           []string         `json:"cited_urls,omitempty"`
}

// SelectedSource is one search result accepted by source review.
type SelectedSource struct {
	URL      string `json:"url"`
	Title    string `json:"title,omitempty"`
	Policy   string `json:"policy,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// RejectedSource is one search result excluded by source review.
type RejectedSource struct {
	URL    string `json:"url,omitempty"`
	Title  string `json:"title,omitempty"`
	Reason string `json:"reason,omitempty"`
}
