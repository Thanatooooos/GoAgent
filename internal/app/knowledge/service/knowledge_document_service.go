package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
	knowledgeschedule "local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
	"local/rag-project/internal/framework/log"
	"local/rag-project/internal/framework/paging"
)

const knowledgeDocumentCleanupTimeout = 30 * time.Second

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
	PipelineID      string
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
	PipelineID      string
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
	Log            domain.KnowledgeDocumentChunkLog
	IngestionTask  *ingestiondomain.Task
	IngestionNodes []ingestiondomain.TaskNode
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
	baseRepo             port.KnowledgeBaseRepository
	documentRepo         port.KnowledgeDocumentRepository
	chunkLogRepo         port.KnowledgeDocumentChunkLogRepository
	storage              port.FileStorage
	taskQueue            port.TaskQueue
	scheduleService      *KnowledgeDocumentScheduleService
	remoteFetcher        remoteDocumentFetcher
	deleteTx             KnowledgeDocumentDeleteTransaction
	ingestionTaskCreator IngestionTaskCreator
	ingestionTaskReader  IngestionTaskReader
	reconcileRecorder    IngestionReconcileRecorder
}

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

func (s *KnowledgeDocumentService) Upload(ctx context.Context, input UploadKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document service is required", nil)
	}
	if s.baseRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge base repository is required", nil)
	}
	if s.documentRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.storage == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("file storage is required", nil)
	}

	knowledgeBaseID := strings.TrimSpace(input.KnowledgeBaseID)
	if knowledgeBaseID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge base id is required", nil)
	}

	fileName := sanitizeDocumentFileName(input.FileName)
	if fileName == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("file name is required", nil)
	}
	if input.Body == nil {
		return domain.KnowledgeDocument{}, exception.NewClientException("file body is required", nil)
	}
	if input.Size < 0 {
		return domain.KnowledgeDocument{}, exception.NewClientException("file size is invalid", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("operator id is required", nil)
	}

	sourceType := normalizeKnowledgeDocumentSourceType(input.SourceType)
	if sourceType == "" {
		sourceType = domain.KnowledgeDocumentSourceFile
	}

	processMode, err := normalizeKnowledgeDocumentProcessMode(input.ProcessMode)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	chunkStrategy, err := normalizeKnowledgeDocumentChunkStrategy(input.ChunkStrategy)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	pipelineID := strings.TrimSpace(input.PipelineID)
	if err := validateKnowledgeDocumentProcessingConfig(processMode, chunkStrategy, pipelineID, true); err != nil {
		return domain.KnowledgeDocument{}, err
	}
	input.ProcessMode = processMode
	input.ChunkStrategy = chunkStrategy
	if processMode == domain.KnowledgeDocumentProcessModeChunk {
		input.PipelineID = ""
	} else {
		input.PipelineID = pipelineID
	}

	knowledgeBase, err := s.baseRepo.GetByID(ctx, knowledgeBaseID)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to get knowledge base", err)
	}
	if knowledgeBase.ID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge base not found", nil)
	}

	id, err := distributedid.NextID()
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to generate knowledge document id", err)
	}
	documentID := fmt.Sprintf("%d", id)
	document, cleanup, err := s.buildKnowledgeDocumentForUpload(ctx, knowledgeBase, documentID, sourceType, input, operatorID)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}

	created, err := s.documentRepo.Create(ctx, document)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to create knowledge document", err)
	}

	if created.IsRemote() && s.scheduleService != nil {
		if err := s.scheduleService.SyncSchedule(ctx, &created, true); err != nil {
			_ = s.documentRepo.Delete(ctx, created.ID)
			if cleanup != nil {
				cleanup()
			}
			return domain.KnowledgeDocument{}, exception.NewServiceException("failed to create knowledge document schedule", err)
		}
	}

	return created, nil
}

func (s *KnowledgeDocumentService) StartChunk(ctx context.Context, input StartChunkKnowledgeDocumentInput) error {
	if s == nil {
		return exception.NewServiceException("knowledge document service is required", nil)
	}
	if s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.taskQueue == nil {
		return exception.NewServiceException("task queue is required", nil)
	}

	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return exception.NewClientException("knowledge document id is required", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return exception.NewClientException("knowledge document not found", nil)
	}
	if !document.Enabled {
		return exception.NewClientException("knowledge document is disabled", nil)
	}
	if !document.CanStartProcessing() {
		return exception.NewClientException("knowledge document cannot start processing", nil)
	}

	rows, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(document.ID),
		port.KnowledgeDocument.Enabled.Eq(true),
		port.KnowledgeDocument.Deleted.Eq(false),
		port.KnowledgeDocument.Status.In(
			domain.KnowledgeDocumentStatusPending,
			domain.KnowledgeDocumentStatusFailed,
			domain.KnowledgeDocumentStatusSuccess,
		),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusRunning),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to mark knowledge document running", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document processing is already running", nil)
	}

	document.Status = domain.KnowledgeDocumentStatusRunning
	document.UpdatedBy = operatorID
	document.UpdatedAt = time.Now()
	if document.IsRemote() {
		if s.scheduleService == nil {
			_ = s.markDocumentFailed(ctx, document.ID, operatorID)
			return exception.NewServiceException("knowledge document schedule service is required", nil)
		}
		if err := s.scheduleService.SyncSchedule(ctx, &document, true); err != nil {
			_ = s.markDocumentFailed(ctx, document.ID, operatorID)
			return exception.NewServiceException("failed to sync knowledge document schedule", err)
		}
	}

	taskID, err := distributedid.NextID()
	if err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return exception.NewServiceException("failed to generate chunk task id", err)
	}
	if err := s.taskQueue.SubmitChunkDocument(ctx, port.ChunkDocumentTask{
		TaskID:      fmt.Sprintf("%d", taskID),
		DocumentID:  document.ID,
		TriggeredBy: operatorID,
	}); err != nil {
		_ = s.markDocumentFailed(ctx, document.ID, operatorID)
		return exception.NewServiceException("failed to submit chunk document task", err)
	}

	return nil
}

func (s *KnowledgeDocumentService) Get(ctx context.Context, input GetKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	if s == nil || s.documentRepo == nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document id is required", nil)
	}
	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to get knowledge document", err)
	}
	if document.ID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("knowledge document not found", nil)
	}
	return document, nil
}

func (s *KnowledgeDocumentService) Page(ctx context.Context, input PageKnowledgeDocumentInput) (KnowledgeDocumentPageResult, error) {
	if s == nil || s.documentRepo == nil {
		return KnowledgeDocumentPageResult{}, exception.NewServiceException("knowledge document repository is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	baseFilter := port.KnowledgeDocumentListFilter{
		KnowledgeBaseID: strings.TrimSpace(input.KnowledgeBaseID),
		Status:          strings.TrimSpace(input.Status),
		Query:           strings.TrimSpace(input.Query),
	}
	total, err := s.countKnowledgeDocuments(ctx, baseFilter)
	if err != nil {
		return KnowledgeDocumentPageResult{}, err
	}

	items, err := s.documentRepo.List(ctx, port.KnowledgeDocumentListFilter{
		KnowledgeBaseID: strings.TrimSpace(input.KnowledgeBaseID),
		Status:          strings.TrimSpace(input.Status),
		Query:           strings.TrimSpace(input.Query),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeDocumentPageResult{}, exception.NewServiceException("failed to page knowledge documents", err)
	}

	return KnowledgeDocumentPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) Search(ctx context.Context, input SearchKnowledgeDocumentsInput) ([]KnowledgeDocumentSearchItem, error) {
	if s == nil || s.documentRepo == nil {
		return nil, exception.NewServiceException("knowledge document repository is required", nil)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 50 {
		limit = 50
	}

	items, err := s.documentRepo.List(ctx, port.KnowledgeDocumentListFilter{
		Query: strings.TrimSpace(input.Query),
		ListOptions: port.ListOptions{
			Limit: limit,
		},
	})
	if err != nil {
		return nil, exception.NewServiceException("failed to search knowledge documents", err)
	}

	result := make([]KnowledgeDocumentSearchItem, 0, len(items))
	for _, item := range items {
		result = append(result, KnowledgeDocumentSearchItem{
			ID:              item.ID,
			KnowledgeBaseID: item.KnowledgeBaseID,
			Name:            item.Name,
		})
	}
	return result, nil
}

func (s *KnowledgeDocumentService) Update(ctx context.Context, input UpdateKnowledgeDocumentInput) (domain.KnowledgeDocument, error) {
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return domain.KnowledgeDocument{}, exception.NewClientException("operator id is required", nil)
	}

	nextProcessMode, err := effectiveKnowledgeDocumentProcessMode(document.ProcessMode, input.ProcessMode)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	nextChunkStrategy, err := effectiveKnowledgeDocumentChunkStrategy(document.ChunkStrategy, input.ChunkStrategy)
	if err != nil {
		return domain.KnowledgeDocument{}, err
	}
	nextPipelineID := strings.TrimSpace(document.PipelineID)
	if strings.TrimSpace(input.PipelineID) != "" {
		nextPipelineID = strings.TrimSpace(input.PipelineID)
	}
	if err := validateKnowledgeDocumentProcessingConfig(
		nextProcessMode,
		nextChunkStrategy,
		nextPipelineID,
		strings.TrimSpace(input.ProcessMode) != "" || strings.TrimSpace(input.ChunkStrategy) != "" || strings.TrimSpace(input.PipelineID) != "",
	); err != nil {
		return domain.KnowledgeDocument{}, err
	}

	if name := strings.TrimSpace(input.Name); name != "" {
		document.Name = name
	}
	document.ProcessMode = nextProcessMode
	if document.ProcessMode == domain.KnowledgeDocumentProcessModeChunk {
		document.ChunkStrategy = nextChunkStrategy
		document.PipelineID = ""
	} else {
		document.ChunkStrategy = ""
		document.PipelineID = nextPipelineID
	}
	if input.ChunkConfig != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			return domain.KnowledgeDocument{}, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	if location := strings.TrimSpace(input.SourceLocation); location != "" {
		document.SourceLocation = location
	}
	if input.ScheduleEnabled != nil {
		document.ScheduleEnabled = *input.ScheduleEnabled
	}
	if cron := strings.TrimSpace(input.ScheduleCron); cron != "" || (input.ScheduleEnabled != nil && !*input.ScheduleEnabled) {
		document.ScheduleCron = cron
	}
	document.UpdatedBy = operatorID
	document.UpdatedAt = time.Now()

	updated, err := s.documentRepo.Update(ctx, document)
	if err != nil {
		return domain.KnowledgeDocument{}, exception.NewServiceException("failed to update knowledge document", err)
	}
	if updated.IsRemote() && s.scheduleService != nil {
		if err := s.scheduleService.SyncSchedule(ctx, &updated, true); err != nil {
			return domain.KnowledgeDocument{}, exception.NewServiceException("failed to update knowledge document schedule", err)
		}
	}
	return updated, nil
}

func (s *KnowledgeDocumentService) Enable(ctx context.Context, input EnableKnowledgeDocumentInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return err
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	rows, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(document.ID),
		port.KnowledgeDocument.Deleted.Eq(false),
	), port.Set(
		port.KnowledgeDocument.Enabled.To(input.Enabled),
		port.KnowledgeDocument.UpdatedBy.To(operatorID),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	if err != nil {
		return exception.NewServiceException("failed to update knowledge document enabled status", err)
	}
	if rows == 0 {
		return exception.NewClientException("knowledge document not found", nil)
	}

	if document.IsRemote() && s.scheduleService != nil {
		document.Enabled = input.Enabled
		document.UpdatedBy = operatorID
		document.UpdatedAt = time.Now()
		if err := s.scheduleService.SyncSchedule(ctx, &document, true); err != nil {
			return exception.NewServiceException("failed to sync knowledge document schedule", err)
		}
	}
	return nil
}

func (s *KnowledgeDocumentService) Delete(ctx context.Context, input DeleteKnowledgeDocumentInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}
	if s.deleteTx == nil {
		return exception.NewServiceException("knowledge document delete transaction is required", nil)
	}
	document, err := s.Get(ctx, GetKnowledgeDocumentInput{DocumentID: input.DocumentID})
	if err != nil {
		return err
	}
	if !document.CanDelete() {
		return exception.NewClientException("knowledge document cannot be deleted in current status", nil)
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		return exception.NewClientException("operator id is required", nil)
	}

	if err := s.deleteTx(ctx, func(
		txCtx context.Context,
		documentRepo port.KnowledgeDocumentRepository,
		chunkRepo port.KnowledgeChunkRepository,
		vectorStore port.VectorStore,
		scheduleRepo port.KnowledgeDocumentScheduleRepository,
		scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
	) error {
		rows, err := documentRepo.UpdateFields(txCtx, port.Where(
			port.KnowledgeDocument.ID.Eq(document.ID),
			port.KnowledgeDocument.Deleted.Eq(false),
			port.KnowledgeDocument.Status.In(
				domain.KnowledgeDocumentStatusPending,
				domain.KnowledgeDocumentStatusFailed,
				domain.KnowledgeDocumentStatusSuccess,
			),
		), port.Set(
			port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusDeleting),
			port.KnowledgeDocument.UpdatedBy.To(operatorID),
			port.KnowledgeDocument.UpdatedAt.To(time.Now()),
		))
		if err != nil {
			return exception.NewServiceException("failed to mark knowledge document deleting", err)
		}
		if rows == 0 {
			return exception.NewClientException("knowledge document cannot be deleted in current status", nil)
		}

		// Hard delete cross-table / externalized runtime data first so we do not leave
		// executable schedule state or searchable vectors behind after the document is deleted.
		if document.IsRemote() {
			if scheduleExecRepo != nil {
				if err := scheduleExecRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
					return exception.NewServiceException("failed to delete knowledge document schedule execs", err)
				}
			}
			if scheduleRepo != nil {
				if err := scheduleRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
					return exception.NewServiceException("failed to delete knowledge document schedule", err)
				}
			}
		}
		if vectorStore != nil {
			if err := vectorStore.DeleteByDocumentID(txCtx, document.ID); err != nil {
				return exception.NewServiceException("failed to delete knowledge document vectors", err)
			}
		}

		// Soft delete chunk rows and the document row itself. Both repositories are backed by
		// GORM soft_delete models, so the business data remains recoverable/auditable in DB.
		if chunkRepo != nil {
			if err := chunkRepo.DeleteByDocumentID(txCtx, document.ID); err != nil {
				return exception.NewServiceException("failed to delete knowledge document chunks", err)
			}
		}
		if err := documentRepo.Delete(txCtx, document.ID); err != nil {
			return exception.NewServiceException("failed to delete knowledge document", err)
		}
		return nil
	}); err != nil {
		return err
	}
	if s.storage != nil && strings.TrimSpace(document.FileURL) != "" {
		if err := s.storage.Delete(ctx, document.FileURL); err != nil {
			log.Errorf("knowledge document file cleanup failed after delete commit: documentId=%s operatorId=%s fileUrl=%s err=%v",
				document.ID, operatorID, document.FileURL, err)
		}
	}
	return nil
}

func (s *KnowledgeDocumentService) PageChunkLogs(ctx context.Context, input KnowledgeDocumentChunkLogPageInput) (KnowledgeDocumentChunkLogPageResult, error) {
	if s == nil || s.chunkLogRepo == nil {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewServiceException("knowledge document chunk log repository is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)
	total, err := s.countKnowledgeDocumentChunkLogs(ctx, documentID)
	if err != nil {
		return KnowledgeDocumentChunkLogPageResult{}, err
	}
	items, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{
		Offset: (page - 1) * pageSize,
		Limit:  pageSize,
	})
	if err != nil {
		return KnowledgeDocumentChunkLogPageResult{}, exception.NewServiceException("failed to page knowledge document chunk logs", err)
	}
	mapped := make([]KnowledgeDocumentChunkLogItem, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, KnowledgeDocumentChunkLogItem{Log: item})
	}
	return KnowledgeDocumentChunkLogPageResult{
		Items:    mapped,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) PageScheduleExecs(ctx context.Context, input PageKnowledgeDocumentScheduleExecInput) (KnowledgeDocumentScheduleExecPageResult, error) {
	if s == nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("knowledge document service is required", nil)
	}
	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewClientException("knowledge document id is required", nil)
	}
	if s.scheduleService == nil || s.scheduleService.scheduleExecRepo == nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}

	page, pageSize := paging.Normalize(input.Page, input.PageSize, defaultKnowledgePageSize, maxKnowledgePageSize)

	filter := port.KnowledgeDocumentScheduleExecListFilter{
		DocumentID: documentID,
		Status:     strings.TrimSpace(input.Status),
	}
	total, err := s.countKnowledgeDocumentScheduleExecs(ctx, filter)
	if err != nil {
		return KnowledgeDocumentScheduleExecPageResult{}, err
	}

	items, err := s.scheduleService.scheduleExecRepo.List(ctx, port.KnowledgeDocumentScheduleExecListFilter{
		DocumentID: documentID,
		Status:     strings.TrimSpace(input.Status),
		ListOptions: port.ListOptions{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
	})
	if err != nil {
		return KnowledgeDocumentScheduleExecPageResult{}, exception.NewServiceException("failed to page knowledge document schedule execs", err)
	}

	return KnowledgeDocumentScheduleExecPageResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *KnowledgeDocumentService) markDocumentFailed(ctx context.Context, documentID, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(strings.TrimSpace(documentID)),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusFailed),
		port.KnowledgeDocument.UpdatedBy.To(strings.TrimSpace(operatorID)),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (s *KnowledgeDocumentService) countKnowledgeDocuments(ctx context.Context, filter port.KnowledgeDocumentListFilter) (int, error) {
	if counter, ok := s.documentRepo.(knowledgeDocumentCounter); ok {
		total, err := counter.Count(ctx, filter)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge documents", err)
		}
		return total, nil
	}
	allItems, err := s.documentRepo.List(ctx, filter)
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge documents", err)
	}
	return len(allItems), nil
}

func (s *KnowledgeDocumentService) countKnowledgeDocumentChunkLogs(ctx context.Context, documentID string) (int, error) {
	if counter, ok := s.chunkLogRepo.(knowledgeDocumentChunkLogCounter); ok {
		total, err := counter.CountByDocumentID(ctx, documentID)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge document chunk logs", err)
		}
		return total, nil
	}
	allItems, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{})
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge document chunk logs", err)
	}
	return len(allItems), nil
}

func (s *KnowledgeDocumentService) countKnowledgeDocumentScheduleExecs(ctx context.Context, filter port.KnowledgeDocumentScheduleExecListFilter) (int, error) {
	if s == nil || s.scheduleService == nil || s.scheduleService.scheduleExecRepo == nil {
		return 0, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}
	if counter, ok := s.scheduleService.scheduleExecRepo.(knowledgeDocumentScheduleExecCounter); ok {
		total, err := counter.Count(ctx, filter)
		if err != nil {
			return 0, exception.NewServiceException("failed to count knowledge document schedule execs", err)
		}
		return total, nil
	}
	lister, ok := s.scheduleService.scheduleExecRepo.(knowledgeDocumentScheduleExecLister)
	if !ok {
		return 0, exception.NewServiceException("knowledge document schedule exec repository is required", nil)
	}
	allItems, err := lister.List(ctx, filter)
	if err != nil {
		return 0, exception.NewServiceException("failed to list knowledge document schedule execs", err)
	}
	return len(allItems), nil
}

func (s *KnowledgeDocumentService) buildKnowledgeDocumentForUpload(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	sourceType string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	switch sourceType {
	case domain.KnowledgeDocumentSourceURL:
		return s.buildRemoteKnowledgeDocument(ctx, knowledgeBase, documentID, input, operatorID)
	default:
		return s.buildUploadedKnowledgeDocument(ctx, knowledgeBase, documentID, input, operatorID)
	}
}

func (s *KnowledgeDocumentService) buildUploadedKnowledgeDocument(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	fileName := sanitizeDocumentFileName(input.FileName)
	if fileName == "" {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file name is required", nil)
	}
	if input.Body == nil {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file body is required", nil)
	}
	if input.Size < 0 {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("file size is invalid", nil)
	}

	storageKey := buildKnowledgeDocumentStorageKey(knowledgeBase.CollectionName, documentID, fileName)
	contentType := strings.TrimSpace(input.ContentType)
	stored, err := s.storage.Upload(ctx, port.FileUpload{
		Key:         storageKey,
		FileName:    fileName,
		ContentType: contentType,
		Size:        input.Size,
		Body:        input.Body,
	})
	if err != nil {
		return domain.KnowledgeDocument{}, nil, exception.NewServiceException("failed to upload knowledge document file", err)
	}
	stored = normalizeStoredKnowledgeDocumentFile(stored, storageKey, fileName, contentType, input.Size)

	document := domain.NewUploadedKnowledgeDocument(
		documentID,
		knowledgeBase.ID,
		stored.FileName,
		stored.Key,
		resolveKnowledgeDocumentFileType(stored.FileName, stored.ContentType),
		operatorID,
		stored.Size,
	)
	document.ProcessMode = normalizeKnowledgeDocumentProcessModeValue(input.ProcessMode)
	document.ChunkStrategy = strings.TrimSpace(input.ChunkStrategy)
	if strings.TrimSpace(input.ChunkConfig) != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			_ = s.storage.Delete(ctx, stored.Key)
			return domain.KnowledgeDocument{}, nil, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	document.PipelineID = strings.TrimSpace(input.PipelineID)
	return document, func() { _ = s.storage.Delete(newCleanupContext(ctx), stored.Key) }, nil
}

func (s *KnowledgeDocumentService) buildRemoteKnowledgeDocument(
	ctx context.Context,
	knowledgeBase domain.KnowledgeBase,
	documentID string,
	input UploadKnowledgeDocumentInput,
	operatorID string,
) (domain.KnowledgeDocument, func(), error) {
	if s.remoteFetcher == nil {
		return domain.KnowledgeDocument{}, nil, exception.NewServiceException("remote file fetcher is required", nil)
	}
	sourceLocation := strings.TrimSpace(input.SourceLocation)
	if sourceLocation == "" {
		return domain.KnowledgeDocument{}, nil, exception.NewClientException("source location is required", nil)
	}
	fallbackName := sanitizeDocumentFileName(input.FileName)
	if fallbackName == "" {
		fallbackName = "remote-file"
	}
	storageKey := buildKnowledgeDocumentStorageKey(knowledgeBase.CollectionName, documentID, fallbackName)
	stored, err := s.remoteFetcher.FetchAndStore(ctx, sourceLocation, storageKey, fallbackName)
	if err != nil {
		return domain.KnowledgeDocument{}, nil, err
	}
	document := domain.NewUploadedKnowledgeDocument(
		documentID,
		knowledgeBase.ID,
		stored.OriginFileName,
		stored.Url,
		resolveKnowledgeDocumentFileType(stored.OriginFileName, stored.DetectedType),
		operatorID,
		stored.Size,
	)
	document.SourceType = domain.KnowledgeDocumentSourceURL
	document.SourceLocation = sourceLocation
	document.ScheduleEnabled = input.ScheduleEnabled
	document.ScheduleCron = strings.TrimSpace(input.ScheduleCron)
	document.ProcessMode = normalizeKnowledgeDocumentProcessModeValue(input.ProcessMode)
	document.ChunkStrategy = strings.TrimSpace(input.ChunkStrategy)
	if strings.TrimSpace(input.ChunkConfig) != "" {
		if !json.Valid([]byte(input.ChunkConfig)) {
			_ = s.storage.Delete(ctx, stored.Url)
			return domain.KnowledgeDocument{}, nil, exception.NewClientException("chunk config must be valid json", nil)
		}
		document.ChunkConfig = []byte(strings.TrimSpace(input.ChunkConfig))
	}
	document.PipelineID = strings.TrimSpace(input.PipelineID)
	return document, func() { _ = s.storage.Delete(newCleanupContext(ctx), stored.Url) }, nil
}

func newCleanupContext(ctx context.Context) context.Context {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	cleanupCtx, _ := context.WithTimeout(base, knowledgeDocumentCleanupTimeout)
	return cleanupCtx
}

func normalizeKnowledgeDocumentSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", domain.KnowledgeDocumentSourceFile:
		return domain.KnowledgeDocumentSourceFile
	case domain.KnowledgeDocumentSourceURL:
		return domain.KnowledgeDocumentSourceURL
	default:
		return ""
	}
}

func normalizeKnowledgeDocumentProcessMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", domain.KnowledgeDocumentProcessModeChunk:
		return domain.KnowledgeDocumentProcessModeChunk, nil
	case domain.KnowledgeDocumentProcessModePipeline:
		return domain.KnowledgeDocumentProcessModePipeline, nil
	default:
		return "", exception.NewClientException("process mode must be chunk or pipeline", nil)
	}
}

func normalizeKnowledgeDocumentProcessModeValue(value string) string {
	mode, err := normalizeKnowledgeDocumentProcessMode(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return mode
}

func effectiveKnowledgeDocumentProcessMode(current string, input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return normalizeKnowledgeDocumentProcessMode(input)
	}
	return normalizeKnowledgeDocumentProcessMode(current)
}

func normalizeKnowledgeDocumentChunkStrategy(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		return "", nil
	case "fixed_size":
		return "fixed_size", nil
	case "markdown", "structure_aware":
		return "structure_aware", nil
	default:
		return "", exception.NewClientException("chunk strategy is invalid", nil)
	}
}

func effectiveKnowledgeDocumentChunkStrategy(current string, input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return normalizeKnowledgeDocumentChunkStrategy(input)
	}
	return normalizeKnowledgeDocumentChunkStrategy(current)
}

func validateKnowledgeDocumentProcessingConfig(processMode string, chunkStrategy string, pipelineID string, validateModeSpecificFields bool) error {
	switch processMode {
	case domain.KnowledgeDocumentProcessModeChunk:
		if chunkStrategy == "" {
			chunkStrategy = "fixed_size"
		}
		if validateModeSpecificFields && strings.TrimSpace(pipelineID) != "" {
			return exception.NewClientException("pipeline id is only allowed when process mode is pipeline", nil)
		}
		return nil
	case domain.KnowledgeDocumentProcessModePipeline:
		if strings.TrimSpace(pipelineID) == "" {
			return exception.NewClientException("pipeline id is required when process mode is pipeline", nil)
		}
		if validateModeSpecificFields && strings.TrimSpace(chunkStrategy) != "" {
			return exception.NewClientException("chunk strategy is only allowed when process mode is chunk", nil)
		}
		return nil
	default:
		return exception.NewClientException("process mode must be chunk or pipeline", nil)
	}
}

func buildKnowledgeDocumentStorageKey(collectionName, documentID, fileName string) string {
	collectionName = strings.Trim(strings.TrimSpace(collectionName), "/")
	if collectionName == "" {
		collectionName = "knowledge"
	}
	return fmt.Sprintf("knowledge/%s/documents/%s/%s", collectionName, documentID, fileName)
}

func normalizeStoredKnowledgeDocumentFile(stored port.StoredFile, key, fileName, contentType string, size int64) port.StoredFile {
	if strings.TrimSpace(stored.Key) == "" {
		stored.Key = key
	}
	if strings.TrimSpace(stored.FileName) == "" {
		stored.FileName = fileName
	}
	if strings.TrimSpace(stored.ContentType) == "" {
		stored.ContentType = contentType
	}
	if stored.Size == 0 && size > 0 {
		stored.Size = size
	}
	return stored
}

func sanitizeDocumentFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return ""
	}
	fileName = filepath.Base(fileName)
	if fileName == "." || fileName == string(filepath.Separator) {
		return ""
	}
	return fileName
}

func resolveKnowledgeDocumentFileType(fileName, contentType string) string {
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), "."); ext != "" {
		return truncateKnowledgeDocumentFileType(ext)
	}

	if mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType)); err == nil && mediaType != "" {
		if slash := strings.LastIndex(mediaType, "/"); slash >= 0 && slash < len(mediaType)-1 {
			return truncateKnowledgeDocumentFileType(mediaType[slash+1:])
		}
		return truncateKnowledgeDocumentFileType(mediaType)
	}

	return "unknown"
}

func truncateKnowledgeDocumentFileType(fileType string) string {
	fileType = strings.TrimSpace(strings.ToLower(fileType))
	if fileType == "" {
		return "unknown"
	}
	if len(fileType) > 16 {
		return fileType[:16]
	}
	return fileType
}

type CreateKnowledgePipelineTaskInput struct {
	TaskID          string
	PipelineID      string
	SourceType      string
	SourceLocation  string
	SourceFileName  string
	DocumentID      string
	KnowledgeBaseID string
	DocumentName    string
	OperatorID      string
}

type KnowledgeDocumentIngestionTaskCompletedInput struct {
	TaskID       string
	DocumentID   string
	PipelineID   string
	ChunkCount   int
	StartedAt    *time.Time
	CompletedAt  *time.Time
	OperatorID   string
	ErrorMessage string
}

type IngestionTaskCreator interface {
	CreateKnowledgePipelineTask(ctx context.Context, input CreateKnowledgePipelineTaskInput) (string, error)
}

type IngestionTaskReader interface {
	GetKnowledgePipelineTask(ctx context.Context, taskID string) (ingestiondomain.Task, error)
	ListKnowledgePipelineTaskNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error)
}

type KnowledgeDocumentIngestionReconcileEvent struct {
	Source          string
	TaskID          string
	DocumentID      string
	Skipped         bool
	DocumentUpdated bool
	ChunkLogUpdated bool
	ChunkLogCreated bool
	ErrorMessage    string
}

type IngestionReconcileRecorder interface {
	RecordKnowledgeDocumentIngestionReconcile(event KnowledgeDocumentIngestionReconcileEvent)
}

func (s *KnowledgeDocumentService) SetIngestionTaskCreator(creator IngestionTaskCreator) {
	if s == nil {
		return
	}
	s.ingestionTaskCreator = creator
}

func (s *KnowledgeDocumentService) SetIngestionTaskReader(reader IngestionTaskReader) {
	if s == nil {
		return
	}
	s.ingestionTaskReader = reader
}

func (s *KnowledgeDocumentService) SetIngestionReconcileRecorder(recorder IngestionReconcileRecorder) {
	if s == nil {
		return
	}
	s.reconcileRecorder = recorder
}

func (s *KnowledgeDocumentService) OnIngestionTaskCompleted(ctx context.Context, input KnowledgeDocumentIngestionTaskCompletedInput) error {
	if s == nil || s.documentRepo == nil {
		return exception.NewServiceException("knowledge document repository is required", nil)
	}

	documentID := strings.TrimSpace(input.DocumentID)
	if documentID == "" {
		return nil
	}

	document, err := s.documentRepo.GetByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("get knowledge document: %w", err)
	}
	if document.ID == "" {
		return nil
	}

	operatorID := strings.TrimSpace(input.OperatorID)
	if operatorID == "" {
		operatorID = "system"
	}

	chunkCount := input.ChunkCount
	if input.ErrorMessage != "" {
		return errors.Join(
			s.markDocumentFailed(ctx, documentID, operatorID),
			s.finishPipelineChunkLogWithRecord(ctx, documentID, input.TaskID, chunkCount, input.ErrorMessage, input.StartedAt, input.CompletedAt),
			s.ReconcileIngestionTaskCompletion(ctx, input),
		)
	}

	return errors.Join(
		s.markDocumentSuccess(ctx, documentID, chunkCount, operatorID),
		s.finishPipelineChunkLogWithRecord(ctx, documentID, input.TaskID, chunkCount, "", input.StartedAt, input.CompletedAt),
		s.ReconcileIngestionTaskCompletion(ctx, input),
	)
}

func (s *KnowledgeDocumentService) markDocumentSuccess(ctx context.Context, documentID string, chunkCount int, operatorID string) error {
	_, err := s.documentRepo.UpdateFields(ctx, port.Where(
		port.KnowledgeDocument.ID.Eq(strings.TrimSpace(documentID)),
		port.KnowledgeDocument.Status.Eq(domain.KnowledgeDocumentStatusRunning),
	), port.Set(
		port.KnowledgeDocument.Status.To(domain.KnowledgeDocumentStatusSuccess),
		port.KnowledgeDocument.ChunkCount.To(chunkCount),
		port.KnowledgeDocument.UpdatedBy.To(strings.TrimSpace(operatorID)),
		port.KnowledgeDocument.UpdatedAt.To(time.Now()),
	))
	return err
}

func (s *KnowledgeDocumentService) finishPipelineChunkLogWithRecord(ctx context.Context, documentID, taskID string, chunkCount int, errorMessage string, startedAt, completedAt *time.Time) error {
	if s.chunkLogRepo == nil {
		return nil
	}
	record, err := s.loadCompletablePipelineChunkLog(ctx, documentID, taskID)
	if err != nil || record.ID == "" {
		return err
	}
	record.Status = domain.KnowledgeDocumentChunkLogStatusSuccess
	record.ErrorMessage = ""
	if errorMessage != "" {
		record.Status = domain.KnowledgeDocumentChunkLogStatusFailed
		record.ErrorMessage = errorMessage
	}
	record.ChunkCount = chunkCount
	now := time.Now()
	if startedAt != nil {
		record.StartTime = startedAt
	}
	if completedAt != nil {
		record.EndTime = completedAt
	}
	record.UpdatedAt = now

	_, err = s.chunkLogRepo.Update(ctx, record)
	return err
}

func (s *KnowledgeDocumentService) loadCompletablePipelineChunkLog(ctx context.Context, documentID string, taskID string) (domain.KnowledgeDocumentChunkLog, error) {
	if s.chunkLogRepo == nil {
		return domain.KnowledgeDocumentChunkLog{}, nil
	}
	if strings.TrimSpace(taskID) != "" {
		record, err := s.chunkLogRepo.GetByTaskID(ctx, strings.TrimSpace(taskID))
		if err != nil {
			return domain.KnowledgeDocumentChunkLog{}, err
		}
		if record.ID != "" {
			if strings.TrimSpace(record.DocumentID) != "" && strings.TrimSpace(record.DocumentID) != strings.TrimSpace(documentID) {
				return domain.KnowledgeDocumentChunkLog{}, fmt.Errorf("knowledge document chunk log task %q belongs to document %q, not %q", taskID, record.DocumentID, documentID)
			}
			return record, nil
		}
	}
	items, err := s.chunkLogRepo.ListByDocumentID(ctx, documentID, port.ListOptions{Limit: 1})
	if err != nil {
		return domain.KnowledgeDocumentChunkLog{}, err
	}
	if len(items) == 0 {
		return domain.KnowledgeDocumentChunkLog{}, nil
	}
	return items[0], nil
}
