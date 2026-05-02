package domain

import "time"

type RagTraceRun struct {
	ID             string
	TraceID        string
	TraceName      string
	EntryMethod    string
	ConversationID string
	TaskID         string
	UserID         string
	Status         string
	ErrorMessage   string
	StartTime      *time.Time
	EndTime        *time.Time
	DurationMs     *int64
	ExtraData      string
	CreateTime     time.Time
	UpdateTime     time.Time
}
