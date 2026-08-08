package domain

import "time"

type Item struct {
	ID             string
	IssueID        string
	SectionKey     string
	Rank           int
	Title          string
	Summary        string
	WhyItMatters   string
	URL            string
	Source         string
	Topic          string
	PublishedAt    *time.Time
	MetadataJSON   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewItem(id, issueID, sectionKey string, rank int, title string) Item {
	now := time.Now()
	return Item{
		ID:         id,
		IssueID:    issueID,
		SectionKey: sectionKey,
		Rank:       rank,
		Title:      title,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
