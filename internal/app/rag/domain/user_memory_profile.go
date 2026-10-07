package domain

import "time"

type UserMemoryProfile struct {
	UserID         string
	ContentMarkdown string
	Version        int64
	LastObservedAt *time.Time
	CreateTime     time.Time
	UpdateTime     time.Time
}
