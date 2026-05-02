package domain

import "time"

type RagTraceNode struct {
	ID           string
	TraceID      string
	NodeID       string
	ParentNodeID string
	Depth        int
	NodeType     string
	NodeName     string
	ClassName    string
	MethodName   string
	Status       string
	ErrorMessage string
	StartTime    *time.Time
	EndTime      *time.Time
	DurationMs   *int64
	ExtraData    string
	CreateTime   time.Time
	UpdateTime   time.Time
}
