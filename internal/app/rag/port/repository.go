package port

import (
	"context"
	"time"

	"local/rag-project/internal/app/rag/domain"
)

type ListOptions struct {
	Offset int
	Limit  int
}

type ConversationMessageOrder string

const (
	ConversationMessageOrderAsc  ConversationMessageOrder = "asc"
	ConversationMessageOrderDesc ConversationMessageOrder = "desc"
)

type ConversationMessageListFilter struct {
	ConversationID string
	UserID         string
	Roles          []string
	AfterID        string
	BeforeID       string
	Order          ConversationMessageOrder
	Limit          int
}

type MemoryItemListFilter struct {
	UserID          string
	ScopeTypes      []string
	ScopeIDs        []string
	Namespaces      []string
	MemoryTypes     []string
	Categories      []string
	CanonicalKeys   []string
	Statuses        []string
	SearchText      string
	SearchTokens    []string
	SourceMessageID string
	SupersedesID    string
	ExpiresBefore   *time.Time
	UpdatedBefore   *time.Time
	ListOptions
}

type ActiveMemoryConflict struct {
	UserID       string
	ScopeType    string
	ScopeID      string
	CanonicalKey string
	ActiveCount  int
}

type RagTraceRunListFilter struct {
	TraceID        string
	ConversationID string
	TaskID         string
	Status         string
	ListOptions
}

type ConversationRepository interface {
	Create(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error)
	Update(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error)
	UpdateWhere(ctx context.Context, cond ConversationConditions, patch ConversationPatch) (int64, error)
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (domain.Conversation, error)
	GetByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.Conversation, error)
	ListByUserID(ctx context.Context, userID string) ([]domain.Conversation, error)
}

type ConversationMessageRepository interface {
	Create(ctx context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error)
	GetByID(ctx context.Context, id string) (domain.ConversationMessage, error)
	List(ctx context.Context, filter ConversationMessageListFilter) ([]domain.ConversationMessage, error)
	CountByConversationIDAndUserIDAndRole(ctx context.Context, conversationID string, userID string, role string) (int64, error)
	FindMaxIDAtOrBefore(ctx context.Context, conversationID string, userID string, at time.Time) (string, error)
	DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error
}

type ConversationSummaryRepository interface {
	Create(ctx context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error)
	FindLatestByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) (domain.ConversationSummary, error)
	DeleteByConversationIDAndUserID(ctx context.Context, conversationID string, userID string) error
}

type MemoryItemRepository interface {
	Create(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error)
	Update(ctx context.Context, item domain.MemoryItem) (domain.MemoryItem, error)
	GetByID(ctx context.Context, id string) (domain.MemoryItem, error)
	List(ctx context.Context, filter MemoryItemListFilter) ([]domain.MemoryItem, error)
	Count(ctx context.Context, filter MemoryItemListFilter) (int64, error)
	ListActiveByCanonicalKey(ctx context.Context, userID string, scopeType string, scopeID string, canonicalKey string) ([]domain.MemoryItem, error)
	ListActiveSingleValueConflicts(ctx context.Context, canonicalKeys []string) ([]ActiveMemoryConflict, error)
	TouchLastUsed(ctx context.Context, userID string, ids []string, at time.Time) error
	ExpireByIDs(ctx context.Context, ids []string, updatedBy string, at time.Time) (int64, error)
	DeleteByStatusesUpdatedBefore(ctx context.Context, statuses []string, updatedBefore time.Time, limit int) (int64, error)
}

type MemoryItemEmbeddingSearchFilter struct {
	UserID      string
	ScopeTypes  []string
	ScopeIDs    []string
	MemoryTypes []string
	Statuses    []string
	TopK        int
}

type MemoryItemEmbeddingRepository interface {
	UpsertBatch(ctx context.Context, embeddings []domain.MemoryItemEmbedding) error
	SearchByVector(ctx context.Context, vector []float32, filter MemoryItemEmbeddingSearchFilter) ([]domain.MemoryItemSearchHit, error)
}

type SessionChunkRepository interface {
	CreateBatch(ctx context.Context, chunks []domain.SessionChunk) error
	ExistsRecallable(ctx context.Context, conversationID string, userID string, excludeMessageID string) (bool, error)
	GetRecallFingerprint(ctx context.Context, conversationID string, userID string, excludeMessageID string) (domain.SessionRecallFingerprint, error)
	SearchRecallableByVector(ctx context.Context, conversationID string, userID string, excludeMessageID string, vector []float32, topK int) ([]domain.SessionChunkSearchHit, error)
}

type SessionChunkEmbeddingRepository interface {
	UpsertBatch(ctx context.Context, embeddings []domain.SessionChunkEmbedding) error
}

type MessageFeedbackRepository interface {
	Create(ctx context.Context, feedback domain.MessageFeedback) (domain.MessageFeedback, error)
	Update(ctx context.Context, feedback domain.MessageFeedback) (domain.MessageFeedback, error)
	UpdateWhere(ctx context.Context, cond MessageFeedbackConditions, patch MessageFeedbackPatch) (int64, error)
	GetByMessageIDAndUserID(ctx context.Context, messageID string, userID string) (domain.MessageFeedback, error)
	ListByUserIDAndMessageIDs(ctx context.Context, userID string, messageIDs []string) ([]domain.MessageFeedback, error)
}

type RagTraceRunRepository interface {
	Create(ctx context.Context, run domain.RagTraceRun) (domain.RagTraceRun, error)
	UpdateByTraceID(ctx context.Context, traceID string, run domain.RagTraceRun) error
	UpdateWhere(ctx context.Context, cond RagTraceRunConditions, patch RagTraceRunPatch) (int64, error)
	GetByTraceID(ctx context.Context, traceID string) (domain.RagTraceRun, error)
	Count(ctx context.Context, filter RagTraceRunListFilter) (int, error)
	List(ctx context.Context, filter RagTraceRunListFilter) ([]domain.RagTraceRun, error)
}

type RagTraceNodeRepository interface {
	Create(ctx context.Context, node domain.RagTraceNode) (domain.RagTraceNode, error)
	UpdateByTraceIDAndNodeID(ctx context.Context, traceID string, nodeID string, node domain.RagTraceNode) error
	UpdateWhere(ctx context.Context, cond RagTraceNodeConditions, patch RagTraceNodePatch) (int64, error)
	ListByTraceID(ctx context.Context, traceID string) ([]domain.RagTraceNode, error)
}
