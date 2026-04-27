package domain

import "time"

type KnowledgeDocumentSchedule struct {
	ID              string
	DocumentID      string
	KnowledgeBaseID string
	CronExpr        string
	Enabled         bool
	NextRunTime     *time.Time
	LastRunTime     *time.Time
	LastSuccessTime *time.Time
	LastStatus      string
	LastError       string
	LastETag        string
	LastModified    string
	LastContentHash string
	LockOwner       string
	LockUntil       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewKnowledgeDocumentSchedule(id, documentID, knowledgeBaseID, cronExpr string) KnowledgeDocumentSchedule {
	now := time.Now()
	return KnowledgeDocumentSchedule{
		ID:              id,
		DocumentID:      documentID,
		KnowledgeBaseID: knowledgeBaseID,
		CronExpr:        cronExpr,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (s KnowledgeDocumentSchedule) IsEnabled() bool {
	return s.Enabled
}
