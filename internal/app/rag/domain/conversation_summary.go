package domain

import "time"

type ConversationSummary struct {
	ID             string
	ConversationID string
	UserID         string
	Content        string
	LastMessageID  string
	CreateTime     time.Time
	UpdateTime     time.Time
}
