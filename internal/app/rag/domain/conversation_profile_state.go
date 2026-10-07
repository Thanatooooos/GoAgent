package domain

import "time"

const (
	ProfileObservationPending    = "pending"
	ProfileObservationProcessing = "processing"
	ProfileObservationCompleted  = "completed"
	ProfileObservationFailed     = "failed"
)

type ConversationProfileState struct {
	ConversationID        string
	UserID                string
	LastObservedMessageID string
	ProcessingToMessageID string
	LastObservedAt        *time.Time
	Status                string
	NextRunAt             time.Time
	Attempts              int
	CreateTime            time.Time
	UpdateTime            time.Time
}
