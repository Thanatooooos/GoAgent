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
	Status string
	Query  string
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
	Status     string
	ListOptions
}

type KnowledgeBaseRepository interface {
	Create(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	Update(ctx context.Context, knowledgeBase domain.KnowledgeBase) (domain.KnowledgeBase, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeBase, error)
	GetByName(ctx context.Context, name string) (int, error)
	Count(ctx context.Context, filter KnowledgeBaseListFilter) (int, error)
	List(ctx context.Context, filter KnowledgeBaseListFilter) ([]domain.KnowledgeBase, error)
}

type KnowledgeDocumentRepository interface {
	Create(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error)
	Update(ctx context.Context, document domain.KnowledgeDocument) (domain.KnowledgeDocument, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocument, error)
	CountByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error)
	CountChunkedByKnowledgeBaseID(ctx context.Context, knowledgeBaseID string) (int, error)
	List(ctx context.Context, filter KnowledgeDocumentListFilter) ([]domain.KnowledgeDocument, error)
}

type KnowledgeChunkRepository interface {
	CreateBatch(ctx context.Context, chunks []domain.KnowledgeChunk) error
	Update(ctx context.Context, chunk domain.KnowledgeChunk) (domain.KnowledgeChunk, error)
	DeleteByDocumentID(ctx context.Context, documentID string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeChunk, error)
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
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.KnowledgeDocumentSchedule, error)
	ListDue(ctx context.Context, before time.Time, limit int) ([]domain.KnowledgeDocumentSchedule, error)
}
