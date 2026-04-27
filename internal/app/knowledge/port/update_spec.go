package port

import "time"

type UpdateValue[T any] struct {
	Set   bool
	Value T
}

func ValueOf[T any](value T) UpdateValue[T] {
	return UpdateValue[T]{
		Set:   true,
		Value: value,
	}
}

type KnowledgeBaseConditions struct {
	ID             string
	NameEQ         string
	NameNE         string
	CollectionEQ   string
	Deleted        *bool
}

type KnowledgeBasePatch struct {
	Name           UpdateValue[string]
	EmbeddingModel UpdateValue[string]
	CollectionName UpdateValue[string]
	UpdatedBy      UpdateValue[string]
	UpdatedAt      UpdateValue[time.Time]
}

type KnowledgeDocumentConditions struct {
	ID              string
	KnowledgeBaseID string
	StatusEQ        string
	StatusNE        string
	SourceTypeEQ    string
	Enabled         *bool
	Deleted         *bool
}

type KnowledgeDocumentPatch struct {
	Name            UpdateValue[string]
	Enabled         UpdateValue[bool]
	ChunkCount      UpdateValue[int]
	FileURL         UpdateValue[string]
	FileType        UpdateValue[string]
	FileSize        UpdateValue[int64]
	ProcessMode     UpdateValue[string]
	Status          UpdateValue[string]
	SourceType      UpdateValue[string]
	SourceLocation  UpdateValue[string]
	ScheduleEnabled UpdateValue[*bool]
	ScheduleCron    UpdateValue[string]
	ChunkStrategy   UpdateValue[string]
	ChunkConfig     UpdateValue[[]byte]
	PipelineID      UpdateValue[string]
	UpdatedBy       UpdateValue[string]
	UpdatedAt       UpdateValue[time.Time]
}

type KnowledgeDocumentScheduleConditions struct {
	ID             string
	DocumentID     string
	Enabled        *bool
	LastStatusEQ   string
	LockOwnerEQ    string
	Deleted        *bool
}

type KnowledgeDocumentSchedulePatch struct {
	KnowledgeBaseID UpdateValue[string]
	CronExpr        UpdateValue[string]
	Enabled         UpdateValue[bool]
	NextRunTime     UpdateValue[*time.Time]
	LastRunTime     UpdateValue[*time.Time]
	LastSuccessTime UpdateValue[*time.Time]
	LastStatus      UpdateValue[string]
	LastError       UpdateValue[string]
	LastETag        UpdateValue[string]
	LastModified    UpdateValue[string]
	LastContentHash UpdateValue[string]
	LockOwner       UpdateValue[*string]
	LockUntil       UpdateValue[*time.Time]
	UpdatedAt       UpdateValue[time.Time]
}

type KnowledgeDocumentScheduleExecConditions struct {
	ID          string
	ScheduleID  string
	DocumentID  string
	StatusEQ    string
	StatusNE    string
}

type KnowledgeDocumentScheduleExecPatch struct {
	Status       UpdateValue[string]
	Message      UpdateValue[string]
	StartTime    UpdateValue[*time.Time]
	EndTime      UpdateValue[*time.Time]
	FileName     UpdateValue[string]
	FileSize     UpdateValue[*int64]
	ContentHash  UpdateValue[string]
	ETag         UpdateValue[string]
	LastModified UpdateValue[string]
	UpdatedAt    UpdateValue[time.Time]
}
