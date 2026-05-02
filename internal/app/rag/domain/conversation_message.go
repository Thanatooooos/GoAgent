package domain

import "time"

type ConversationMessage struct {
	ID               string
	ConversationID   string
	UserID           string
	Role             string
	Content          string
	ThinkingContent  string
	ThinkingDuration *int
	CreateTime       time.Time
	UpdateTime       time.Time
}
