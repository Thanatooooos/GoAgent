package domain

import "time"

type MessageFeedback struct {
	ID             string
	MessageID      string
	ConversationID string
	UserID         string
	Vote           int
	Reason         string
	Comment        string
	CreateTime     time.Time
	UpdateTime     time.Time
}
