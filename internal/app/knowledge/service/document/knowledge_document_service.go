package document

import (
	"context"
	"io"

	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	knowledgeschedule "local/rag-project/internal/app/knowledge/schedule"
)

type UploadKnowledgeDocumentInput struct {
	KnowledgeBaseID string
	SourceType      string
	FileName        string
	ContentType     string
	Size            int64
	Body            io.Reader
	SourceLocation  string
	ScheduleEnabled bool
	ScheduleCron    string
	ProcessMode     string
	ChunkStrategy   string
	ChunkConfig     string
	OperatorID      string
}

type StartChunkKnowledgeDocumentInput struct {
	DocumentID string
	OperatorID string
}

type GetKnowledgeDocumentInput struct {
	DocumentID string
}

type UpdateKnowledgeDocumentInput struct {
	DocumentID      string
	Name            string
	ProcessMode     string
	ChunkStrategy   string
	ChunkConfig     string
	SourceLocation  string
	ScheduleEnabled *bool
	ScheduleCron    string
	OperatorID      string
}

type EnableKnowledgeDocumentInput struct {
	DocumentID string
	Enabled    bool
	OperatorID string
}

type DeleteKnowledgeDocumentInput struct {
	DocumentID string
	OperatorID string
}

type PageKnowledgeDocumentInput struct {
	KnowledgeBaseID string
	Page            int
	PageSize        int
	Status          string
	Query           string
}

type KnowledgeDocumentPageResult struct {
	Items    []domain.KnowledgeDocument
	Total    int
	Page     int
	PageSize int
}

type SearchKnowledgeDocumentsInput struct {
	Query string
	Limit int
}

type KnowledgeDocumentSearchItem struct {
	ID              string
	KnowledgeBaseID string
	Name            string
}

type KnowledgeDocumentChunkLogPageInput struct {
	DocumentID string
	Page       int
	PageSize   int
}

type KnowledgeDocumentChunkLogItem struct {
	Log domain.KnowledgeDocumentChunkLog
}

type KnowledgeDocumentChunkLogPageResult struct {
	Items    []KnowledgeDocumentChunkLogItem
	Total    int
	Page     int
	PageSize int
}

type PageKnowledgeDocumentScheduleExecInput struct {
	DocumentID string
	Page       int
	PageSize   int
	Status     string
}

type KnowledgeDocumentScheduleExecPageResult struct {
	Items    []domain.KnowledgeDocumentScheduleExec
	Total    int
	Page     int
	PageSize int
}

type KnowledgeDocumentService struct {
	baseRepo        port.KnowledgeBaseRepository
	documentRepo    port.KnowledgeDocumentRepository
	chunkLogRepo    port.KnowledgeDocumentChunkLogRepository
	storage         port.FileStorage
	taskQueue       port.TaskQueue
	chunkJobs       port.ChunkJobs
	scheduleService *KnowledgeDocumentScheduleService
	remoteFetcher   remoteDocumentFetcher
	deleteTx        KnowledgeDocumentDeleteTransaction
}

func (s *KnowledgeDocumentService) SetChunkJobs(jobs port.ChunkJobs) { s.chunkJobs = jobs }

type remoteDocumentFetcher interface {
	FetchAndStore(ctx context.Context, rawURL string, storageKey string, fallbackFileName string) (knowledgeschedule.StoredFileDTO, error)
}

type knowledgeDocumentCounter interface {
	Count(ctx context.Context, filter port.KnowledgeDocumentListFilter) (int, error)
}

type knowledgeDocumentChunkLogCounter interface {
	CountByDocumentID(ctx context.Context, documentID string) (int, error)
}

type knowledgeDocumentScheduleExecCounter interface {
	Count(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) (int, error)
}

type knowledgeDocumentScheduleExecLister interface {
	List(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) ([]domain.KnowledgeDocumentScheduleExec, error)
}

func NewKnowledgeDocumentService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
	_ port.KnowledgeChunkRepository, // deprecated: kept for backward compatibility
	chunkLogRepo port.KnowledgeDocumentChunkLogRepository,
	_ port.VectorStore, // deprecated: kept for backward compatibility
	storage port.FileStorage,
	taskQueue port.TaskQueue,
	scheduleService *KnowledgeDocumentScheduleService,
	remoteFetcher remoteDocumentFetcher,
	deleteTx ...KnowledgeDocumentDeleteTransaction,
) *KnowledgeDocumentService {
	var documentDeleteTx KnowledgeDocumentDeleteTransaction
	if len(deleteTx) > 0 {
		documentDeleteTx = deleteTx[0]
	}
	return &KnowledgeDocumentService{
		baseRepo:        baseRepo,
		documentRepo:    documentRepo,
		chunkLogRepo:    chunkLogRepo,
		storage:         storage,
		taskQueue:       taskQueue,
		scheduleService: scheduleService,
		remoteFetcher:   remoteFetcher,
		deleteTx:        documentDeleteTx,
	}
}
