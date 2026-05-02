package port

import (
	"context"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
)

type ListOptions struct {
	Offset int
	Limit  int
}

type KnowledgeBaseListFilter struct {
	Query string
	ListOptions
}

type KnowledgeDocumentListFilter struct {
	KnowledgeBaseID string
	SourceType      string
	Status          string
	Enabled         *bool
	Query           string
	ListOptions
}

type KnowledgeChunkListFilter struct {
	DocumentID string
	Enabled    *bool
	ListOptions
}

type KnowledgeDocumentScheduleExecListFilter struct {
	ScheduleID string
	DocumentID string
	Status     string
	ListOptions
}

type KnowledgeBaseRepository interface {
	Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	UpdateWhere(ctx context.Context, cond KnowledgeBaseConditions, patch KnowledgeBasePatch) (int64, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error)
	GetByName(ctx context.Context, name string) (int, error)
	Count(ctx context.Context, filter KnowledgeBaseListFilter) (int, error)
	List(ctx context.Context, filter KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error)
}

type KnowledgeDocumentRepository interface {
	Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error)
	Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error)
	UpdateWhere(ctx context.Context, cond KnowledgeDocumentConditions, patch KnowledgeDocumentPatch) (int64, error)
	UpdateFields(ctx context.Context, where UpdatePredicates, set UpdateAssignments) (int64, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error)
	CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error)
	CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error)
	List(ctx context.Context, filter KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error)
}

type KnowledgeChunkRepository interface {
	Create(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error)
	CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error
	Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error)
	Delete(ctx context.Context, id string) error
	DeleteByDocumentID(ctx context.Context, documentID string) error
	UpdateEnabledByDocumentID(ctx context.Context, documentID string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error)
	UpdateEnabledByIDs(ctx context.Context, documentID string, chunkIDs []string, enabled bool, updatedBy string, updatedAt time.Time) (int64, error)
	GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error)
	CountByDocumentID(ctx context.Context, documentID string, enabled *bool) (int, error)
	List(ctx context.Context, filter KnowledgeChunkListFilter) ([]domain.KnowledgeChunk, error)
}

type KnowledgeDocumentChunkLogRepository interface {
	Create(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error)
	Update(ctx context.Context, log domain.KnowledgeDocumentChunkLog) (domain.KnowledgeDocumentChunkLog, error)
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentChunkLog, error)
	GetByTaskID(ctx context.Context, taskID string) (domain.KnowledgeDocumentChunkLog, error)
	ListByDocumentID(ctx context.Context, documentID string, options ListOptions) ([]domain.KnowledgeDocumentChunkLog, error)
}

type KnowledgeDocumentScheduleRepository interface {
	Create(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error)
	Update(ctx context.Context, schedule domain.KnowledgeDocumentSchedule) (domain.KnowledgeDocumentSchedule, error)
	UpdateWhere(ctx context.Context, cond KnowledgeDocumentScheduleConditions, patch KnowledgeDocumentSchedulePatch) (int64, error)
	Delete(ctx context.Context, id string) error
	DeleteByDocumentID(ctx context.Context, documentID string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error)
	GetByDocumentID(ctx context.Context, documentID string) (domain.KnowledgeDocumentSchedule, error)
	ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error)
	TryAcquireLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time, now time.Time) (bool, error)
	RenewLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease, lockUntil time.Time) (bool, error)
	ReleaseLock(ctx context.Context, lease domain.KnowledgeDocumentScheduleLockLease) (bool, error)
}

type KnowledgeDocumentScheduleExecRepository interface {
	Create(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error)
	Update(ctx context.Context, exec domain.KnowledgeDocumentScheduleExec) (domain.KnowledgeDocumentScheduleExec, error)
	UpdateWhere(ctx context.Context, cond KnowledgeDocumentScheduleExecConditions, patch KnowledgeDocumentScheduleExecPatch) (int64, error)
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentScheduleExec, error)
	DeleteByDocumentID(ctx context.Context, documentID string) error
	List(ctx context.Context, filter KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error)
}
