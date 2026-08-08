package domain

import "time"

// Candidate is the normalized pre-LLM source item shape.
type Candidate struct {
	ID             string
	Title          string
	URL            string
	Source         string
	Topic          string
	PublishedAt    time.Time
	ExternalID     string
	SummarySnippet string
	Metadata       map[string]string
	CollectedAt    time.Time
}

func NewCandidate(id, source, title, url string) Candidate {
	now := time.Now()
	return Candidate{
		ID:          id,
		Source:      source,
		Title:       title,
		URL:         url,
		CollectedAt: now,
	}
}
