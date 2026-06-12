package domain

import "time"

type ConversationSummary struct {
	ID                   string
	ConversationID       string
	UserID               string
	Content              string
	LastMessageID        string
	SummaryVersion       int
	CoveredFromMessageID string
	CoveredToMessageID   string
	SourceMessageCount   int
	QualityStatus        string
	LastRebuildReason    string
	CreateTime           time.Time
	UpdateTime           time.Time
}
