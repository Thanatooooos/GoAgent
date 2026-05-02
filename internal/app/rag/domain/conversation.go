package domain

import "time"

type Conversation struct {
	ID             string
	ConversationID string
	UserID         string
	Title          string
	LastTime       *time.Time
	CreateTime     time.Time
	UpdateTime     time.Time
}
