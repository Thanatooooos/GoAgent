// Package service is the stable application-facing entry for knowledge services.
// Implementation lives in responsibility-focused subpackages; this root package
// keeps constructor and type aliases for existing callers.
package service

import (
	"context"

	knowledgeschedule "local/rag-project/internal/app/knowledge/schedule"
	"local/rag-project/internal/app/knowledge/port"
	knowledgebase "local/rag-project/internal/app/knowledge/service/base"
	knowledgechunk "local/rag-project/internal/app/knowledge/service/chunk"
	knowledgedocument "local/rag-project/internal/app/knowledge/service/document"
	knowledgeprocess "local/rag-project/internal/app/knowledge/service/process"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type (
	KnowledgeBaseService     = knowledgebase.KnowledgeBaseService
	CreateKnowledgeBaseInput = knowledgebase.CreateKnowledgeBaseInput
	GetKnowledgeBaseInput    = knowledgebase.GetKnowledgeBaseInput
	UpdateKnowledgeBaseInput = knowledgebase.UpdateKnowledgeBaseInput
	DeleteKnowledgeBaseInput = knowledgebase.DeleteKnowledgeBaseInput
	PageKnowledgeBaseInput   = knowledgebase.PageKnowledgeBaseInput
	KnowledgeBasePageResult  = knowledgebase.KnowledgeBasePageResult

	KnowledgeChunkService            = knowledgechunk.KnowledgeChunkService
	CreateKnowledgeChunkInput        = knowledgechunk.CreateKnowledgeChunkInput
	UpdateKnowledgeChunkInput        = knowledgechunk.UpdateKnowledgeChunkInput
	DeleteKnowledgeChunkInput        = knowledgechunk.DeleteKnowledgeChunkInput
	EnableKnowledgeChunkInput        = knowledgechunk.EnableKnowledgeChunkInput
	BatchToggleKnowledgeChunksInput  = knowledgechunk.BatchToggleKnowledgeChunksInput
	PageKnowledgeChunkInput          = knowledgechunk.PageKnowledgeChunkInput
	KnowledgeChunkPageResult         = knowledgechunk.KnowledgeChunkPageResult
	KnowledgeChunkMutationTransaction = knowledgechunk.KnowledgeChunkMutationTransaction

	KnowledgeDocumentService                    = knowledgedocument.KnowledgeDocumentService
	UploadKnowledgeDocumentInput                = knowledgedocument.UploadKnowledgeDocumentInput
	StartChunkKnowledgeDocumentInput            = knowledgedocument.StartChunkKnowledgeDocumentInput
	GetKnowledgeDocumentInput                   = knowledgedocument.GetKnowledgeDocumentInput
	UpdateKnowledgeDocumentInput                = knowledgedocument.UpdateKnowledgeDocumentInput
	EnableKnowledgeDocumentInput                = knowledgedocument.EnableKnowledgeDocumentInput
	DeleteKnowledgeDocumentInput                = knowledgedocument.DeleteKnowledgeDocumentInput
	PageKnowledgeDocumentInput                  = knowledgedocument.PageKnowledgeDocumentInput
	KnowledgeDocumentPageResult                 = knowledgedocument.KnowledgeDocumentPageResult
	SearchKnowledgeDocumentsInput               = knowledgedocument.SearchKnowledgeDocumentsInput
	KnowledgeDocumentSearchItem                 = knowledgedocument.KnowledgeDocumentSearchItem
	KnowledgeDocumentChunkLogPageInput          = knowledgedocument.KnowledgeDocumentChunkLogPageInput
	KnowledgeDocumentChunkLogItem               = knowledgedocument.KnowledgeDocumentChunkLogItem
	KnowledgeDocumentChunkLogPageResult         = knowledgedocument.KnowledgeDocumentChunkLogPageResult
	PageKnowledgeDocumentScheduleExecInput      = knowledgedocument.PageKnowledgeDocumentScheduleExecInput
	KnowledgeDocumentScheduleExecPageResult     = knowledgedocument.KnowledgeDocumentScheduleExecPageResult
	KnowledgeDocumentDeleteTransaction          = knowledgedocument.KnowledgeDocumentDeleteTransaction
	CreateKnowledgePipelineTaskInput            = knowledgedocument.CreateKnowledgePipelineTaskInput
	KnowledgeDocumentIngestionTaskCompletedInput = knowledgedocument.KnowledgeDocumentIngestionTaskCompletedInput
	IngestionTaskCreator                        = knowledgedocument.IngestionTaskCreator
	IngestionTaskReader                         = knowledgedocument.IngestionTaskReader
	KnowledgeDocumentIngestionReconcileEvent    = knowledgedocument.KnowledgeDocumentIngestionReconcileEvent
	IngestionReconcileRecorder                  = knowledgedocument.IngestionReconcileRecorder

	KnowledgeDocumentScheduleService     = knowledgedocument.KnowledgeDocumentScheduleService
	KnowledgeDocumentScheduleTransaction = knowledgedocument.KnowledgeDocumentScheduleTransaction
	Sourcetype                           = knowledgedocument.Sourcetype

	DocumentProcessService            = knowledgeprocess.DocumentProcessService
	DocumentProcessServiceOptions     = knowledgeprocess.DocumentProcessServiceOptions
	ExecuteChunkInput                 = knowledgeprocess.ExecuteChunkInput
	DocumentChunkPersistenceTransaction = knowledgeprocess.DocumentChunkPersistenceTransaction
)

const (
	FILE = knowledgedocument.FILE
	URL  = knowledgedocument.URL
)

func NewKnowledgeBaseService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
) *KnowledgeBaseService {
	return knowledgebase.NewKnowledgeBaseService(baseRepo, documentRepo)
}

func NewKnowledgeChunkService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	vectorStore port.VectorStore,
	embedding aiembedding.EmbeddingService,
	transaction ...KnowledgeChunkMutationTransaction,
) *KnowledgeChunkService {
	return knowledgechunk.NewKnowledgeChunkService(baseRepo, documentRepo, chunkRepo, vectorStore, embedding, transaction...)
}

func NewKnowledgeDocumentService(
	baseRepo port.KnowledgeBaseRepository,
	documentRepo port.KnowledgeDocumentRepository,
	chunkRepo port.KnowledgeChunkRepository,
	chunkLogRepo port.KnowledgeDocumentChunkLogRepository,
	vectorStore port.VectorStore,
	storage port.FileStorage,
	taskQueue port.TaskQueue,
	scheduleService *KnowledgeDocumentScheduleService,
	remoteFetcher interface {
		FetchAndStore(ctx context.Context, rawURL string, storageKey string, fallbackFileName string) (knowledgeschedule.StoredFileDTO, error)
	},
	deleteTx ...KnowledgeDocumentDeleteTransaction,
) *KnowledgeDocumentService {
	return knowledgedocument.NewKnowledgeDocumentService(
		baseRepo,
		documentRepo,
		chunkRepo,
		chunkLogRepo,
		vectorStore,
		storage,
		taskQueue,
		scheduleService,
		remoteFetcher,
		deleteTx...,
	)
}

func NewKnowledgeDocumentScheduleService(
	scheduleRepo port.KnowledgeDocumentScheduleRepository,
	scheduleExecRepo port.KnowledgeDocumentScheduleExecRepository,
	scheduleSeconds int64,
	transaction KnowledgeDocumentScheduleTransaction,
) *KnowledgeDocumentScheduleService {
	return knowledgedocument.NewKnowledgeDocumentScheduleService(scheduleRepo, scheduleExecRepo, scheduleSeconds, transaction)
}

func NewDocumentProcessService(options DocumentProcessServiceOptions) *DocumentProcessService {
	return knowledgeprocess.NewDocumentProcessService(options)
}
