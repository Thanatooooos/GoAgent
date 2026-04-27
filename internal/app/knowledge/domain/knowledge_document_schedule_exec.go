package domain

import "time"

type KnowledgeDocumentScheduleExec struct {
	ID              string
	ScheduleID      string
	DocumentID      string
	KnowledgeBaseID string
	Status          string
	Message         string
	StartTime       *time.Time
	EndTime         *time.Time
	FileName        string
	FileSize        *int64
	ContentHash     string
	ETag            string
	LastModified    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewKnowledgeDocumentScheduleExec(id, scheduleID, documentID, knowledgeBaseID string, startTime time.Time) KnowledgeDocumentScheduleExec {
	now := time.Now()
	return KnowledgeDocumentScheduleExec{
		ID:              id,
		ScheduleID:      scheduleID,
		DocumentID:      documentID,
		KnowledgeBaseID: knowledgeBaseID,
		Status:          KnowledgeDocumentScheduleRunStatusRunning,
		StartTime:       &startTime,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (e KnowledgeDocumentScheduleExec) IsFinished() bool {
	return e.Status == KnowledgeDocumentScheduleRunStatusSuccess ||
		e.Status == KnowledgeDocumentScheduleRunStatusFailed ||
		e.Status == KnowledgeDocumentScheduleRunStatusSkipped
}
