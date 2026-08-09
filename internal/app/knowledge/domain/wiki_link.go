package domain

import "time"

const (
	WikiLinkTargetTypeWiki     = "wiki"
	WikiLinkTargetTypeExternal = "external"
)

type WikiLink struct {
	ID              string
	KnowledgeBaseID string
	FromPageID      string
	ToPageID        string
	TargetType      string
	Anchor          string
	CreatedAt       time.Time
}

func NewWikiLink(id, kbID, fromPageID, toPageID, targetType, anchor string) WikiLink {
	return WikiLink{
		ID:              id,
		KnowledgeBaseID: kbID,
		FromPageID:      fromPageID,
		ToPageID:        toPageID,
		TargetType:      targetType,
		Anchor:          anchor,
		CreatedAt:       time.Now(),
	}
}
