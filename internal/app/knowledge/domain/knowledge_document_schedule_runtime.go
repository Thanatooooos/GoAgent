package domain

import "time"

type KnowledgeDocumentScheduleLockLease struct {
	ScheduleID string
	LockToken  string
}

func NewKnowledgeDocumentScheduleLockLease(scheduleID, lockToken string) KnowledgeDocumentScheduleLockLease {
	return KnowledgeDocumentScheduleLockLease{
		ScheduleID: scheduleID,
		LockToken:  lockToken,
	}
}

type KnowledgeDocumentScheduleStateContext struct {
	ScheduleID  string
	ExecID      string
	CronExpr    string
	StartTime   time.Time
	NextRunTime *time.Time
}
