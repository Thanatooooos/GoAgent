// Package service is the stable application-facing entry for ingestion services.
// Implementation lives in responsibility-focused subpackages; this root package
// keeps constructor and type aliases for existing callers.
package service

import (
	"net/http"

	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	"local/rag-project/internal/app/ingestion/port"
	ingestionexecutor "local/rag-project/internal/app/ingestion/service/executor"
	ingestionobserver "local/rag-project/internal/app/ingestion/service/observer"
	ingestionpipeline "local/rag-project/internal/app/ingestion/service/pipeline"
	ingestionrunner "local/rag-project/internal/app/ingestion/service/runner"
	ingestiontask "local/rag-project/internal/app/ingestion/service/task"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	aiembedding "local/rag-project/internal/infra-ai/embedding"
)

type (
	PipelineService     = ingestionpipeline.PipelineService
	PagePipelinesInput  = ingestionpipeline.PagePipelinesInput
	PipelinePageResult  = ingestionpipeline.PipelinePageResult
	CreatePipelineInput = ingestionpipeline.CreatePipelineInput
	UpdatePipelineInput = ingestionpipeline.UpdatePipelineInput

	TaskService     = ingestiontask.TaskService
	PageTasksInput  = ingestiontask.PageTasksInput
	TaskPageResult  = ingestiontask.TaskPageResult
	CreateTaskInput = ingestiontask.CreateTaskInput

	ExecutorService        = ingestionexecutor.ExecutorService
	ExecutorServiceOptions = ingestionexecutor.ExecutorServiceOptions

	MetricsService           = ingestionobserver.MetricsService
	MetricsSnapshot          = ingestionobserver.MetricsSnapshot
	MetricsTotalsSnapshot    = ingestionobserver.MetricsTotalsSnapshot
	MetricsRatesSnapshot     = ingestionobserver.MetricsRatesSnapshot
	ReconcileMetricsSnapshot = ingestionobserver.ReconcileMetricsSnapshot
	ReconcileFailureSnapshot = ingestionobserver.ReconcileFailureSnapshot
	ReconcileMetricsEvent    = ingestionobserver.ReconcileMetricsEvent
	NodeMetricsSnapshot      = ingestionobserver.NodeMetricsSnapshot
	TaskObserver             = ingestionobserver.TaskObserver
	MultiTaskObserver        = ingestionobserver.MultiTaskObserver
	RepositoryTaskObserver     = ingestionobserver.RepositoryTaskObserver
	MetricsObserver            = ingestionobserver.MetricsObserver

	NodeRunnerRegistry = ingestionrunner.NodeRunnerRegistry
	NodeRunner         = ingestionrunner.NodeRunner
	FetcherNodeRunner  = ingestionrunner.FetcherNodeRunner
	ParserNodeRunner   = ingestionrunner.ParserNodeRunner
	EnhancerNodeRunner = ingestionrunner.EnhancerNodeRunner
	ChunkerNodeRunner  = ingestionrunner.ChunkerNodeRunner
	EnricherNodeRunner = ingestionrunner.EnricherNodeRunner
	IndexerNodeRunner  = ingestionrunner.IndexerNodeRunner

	WorkflowBuilder          = ingestionworkflow.WorkflowBuilder
	EinoGraphWorkflowBuilder = ingestionworkflow.EinoGraphWorkflowBuilder
	ExecutionState           = ingestionworkflow.ExecutionState
	SourcePayload            = ingestionworkflow.SourcePayload
	ParsedDocument           = ingestionworkflow.ParsedDocument
	ChunkPayload             = ingestionworkflow.ChunkPayload
	IndexResult              = ingestionworkflow.IndexResult
	WorkflowSpec             = ingestionworkflow.WorkflowSpec
	WorkflowNodeSpec         = ingestionworkflow.WorkflowNodeSpec
	WorkflowEdgeSpec         = ingestionworkflow.WorkflowEdgeSpec
	NodeIOContract           = ingestionworkflow.NodeIOContract
	NodeInputRequirement     = ingestionworkflow.NodeInputRequirement
)

func NewPipelineService(repo port.PipelineRepository, nodeRunners ...*NodeRunnerRegistry) *PipelineService {
	return ingestionpipeline.NewPipelineService(repo, nodeRunners...)
}

func NewTaskService(
	pipelineRepo port.PipelineRepository,
	taskRepo port.TaskRepository,
	taskNodeRepo port.TaskNodeRepository,
	executor port.TaskExecutor,
) *TaskService {
	return ingestiontask.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, executor)
}

func NewExecutorService(options ExecutorServiceOptions) *ExecutorService {
	return ingestionexecutor.NewExecutorService(options)
}

func NewMetricsService(maxConcurrent int) *MetricsService {
	return ingestionobserver.NewMetricsService(maxConcurrent)
}

func NewMultiTaskObserver(observers ...TaskObserver) *MultiTaskObserver {
	return ingestionobserver.NewMultiTaskObserver(observers...)
}

func NewRepositoryTaskObserver(taskRepo port.TaskRepository, taskNodeRepo port.TaskNodeRepository) *RepositoryTaskObserver {
	return ingestionobserver.NewRepositoryTaskObserver(taskRepo, taskNodeRepo)
}

func NewMetricsObserver(service *MetricsService) *MetricsObserver {
	return ingestionobserver.NewMetricsObserver(service)
}

func NewNodeRunnerRegistry(runners ...NodeRunner) *NodeRunnerRegistry {
	return ingestionrunner.NewNodeRunnerRegistry(runners...)
}

func NewFetcherNodeRunner(storage knowledgeport.FileStorage, httpClient *http.Client) *FetcherNodeRunner {
	return ingestionrunner.NewFetcherNodeRunner(storage, httpClient)
}

func NewParserNodeRunner(selector *coreparser.Selector) *ParserNodeRunner {
	return ingestionrunner.NewParserNodeRunner(selector)
}

func NewEnhancerNodeRunner() *EnhancerNodeRunner {
	return ingestionrunner.NewEnhancerNodeRunner()
}

func NewChunkerNodeRunner(selector *corechunk.Selector) *ChunkerNodeRunner {
	return ingestionrunner.NewChunkerNodeRunner(selector)
}

func NewEnricherNodeRunner() *EnricherNodeRunner {
	return ingestionrunner.NewEnricherNodeRunner()
}

func NewIndexerNodeRunner(
	baseRepo knowledgeport.KnowledgeBaseRepository,
	chunkRepo knowledgeport.KnowledgeChunkRepository,
	vectorStore knowledgeport.VectorStore,
	embedding aiembedding.EmbeddingService,
) *IndexerNodeRunner {
	return ingestionrunner.NewIndexerNodeRunner(baseRepo, chunkRepo, vectorStore, embedding)
}

func NewEinoGraphWorkflowBuilder() *EinoGraphWorkflowBuilder {
	return ingestionworkflow.NewEinoGraphWorkflowBuilder()
}

func ListNodeIOContracts() []NodeIOContract {
	return ingestionworkflow.ListNodeIOContracts()
}
