package assembly

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	postgresingestion "local/rag-project/internal/adapter/repository/postgres/ingestion"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	raginvgraph "local/rag-project/internal/app/rag/tool/invokers/graph"
	raginvmeta "local/rag-project/internal/app/rag/tool/invokers/meta"
	raginvsystem "local/rag-project/internal/app/rag/tool/invokers/system"
	raginvtrace "local/rag-project/internal/app/rag/tool/invokers/trace"
	raginvweb "local/rag-project/internal/app/rag/tool/invokers/web"
	graphmod "local/rag-project/internal/app/rag/tool/modules/graph"
	metamod "local/rag-project/internal/app/rag/tool/modules/meta"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	tracemod "local/rag-project/internal/app/rag/tool/modules/trace"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
	"local/rag-project/internal/app/rag/tool/planner"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/log"
	aichat "local/rag-project/internal/infra-ai/chat"
	inframcp "local/rag-project/internal/infra-mcp"
)

type knowledgePipelineTaskReader struct {
	taskService *ingestionservice.TaskService
}

func (r knowledgePipelineTaskReader) GetKnowledgePipelineTask(ctx context.Context, taskID string) (ingestiondomain.Task, error) {
	return r.taskService.Get(ctx, taskID)
}

func (r knowledgePipelineTaskReader) ListKnowledgePipelineTaskNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error) {
	return r.taskService.ListNodes(ctx, taskID)
}

// buildServices constructs the shared service layer from the database connection.
func buildServices(db *gorm.DB) (*knowledgeservice.KnowledgeDocumentService, *ingestionservice.TaskService) {
	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	documentRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	chunkLogRepo := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(db)
	documentService := knowledgeservice.NewKnowledgeDocumentService(
		baseRepo, documentRepo, nil, chunkLogRepo,
		nil, nil, nil, nil, nil, nil,
	)

	pipelineRepo := postgresingestion.NewPipelineRepository(db)
	taskRepo := postgresingestion.NewTaskRepository(db)
	taskNodeRepo := postgresingestion.NewTaskNodeRepository(db)
	taskService := ingestionservice.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, nil)
	documentService.SetIngestionTaskReader(knowledgePipelineTaskReader{taskService: taskService})

	return documentService, taskService
}

func registerMetaTools(registry *ragcore.Registry) {
	registerModule(registry,
		raginvmeta.NewThinkTool(),
		ragcore.ToolSpec{
			Capability:          ragcore.CapabilityGeneral,
			ExecutionMode:       ragcore.ExecutionModeReadOnly,
			RiskLevel:           ragcore.RiskLevelLow,
			ApprovalRequirement: ragcore.ApprovalRequirementNone,
			ReadOnly:            true,
			Family:              "system",
		},
		metamod.ThinkBehavior(),
	)
}

func registerWebTools(registry *ragcore.Registry, cfg *config.Config, mcpManager inframcp.ToolClient) {
	searchProvider := buildSearchProvider(cfg, mcpManager)
	sourcePolicy := buildSourcePolicyEngine(cfg)

	registerModule(registry,
		raginvweb.NewWebSearchTool(searchProvider, sourcePolicy),
		ragcore.ToolSpec{
			Capability:          ragcore.CapabilitySearch,
			EvidenceSources:     []string{ragcore.EvidenceSourceExternalWeb},
			ExecutionMode:       ragcore.ExecutionModeReadOnly,
			RiskLevel:           ragcore.RiskLevelLow,
			ApprovalRequirement: ragcore.ApprovalRequirementNone,
			ReadOnly:            true,
			Family:              "web",
		},
		webmod.WebSearchBehavior(),
	)
	registerModule(registry,
		raginvweb.NewWebFetchTool(),
		ragcore.ToolSpec{
			Capability:          ragcore.CapabilitySearch,
			EvidenceSources:     []string{ragcore.EvidenceSourceExternalWeb},
			ExecutionMode:       ragcore.ExecutionModeReadOnly,
			RiskLevel:           ragcore.RiskLevelLow,
			ApprovalRequirement: ragcore.ApprovalRequirementNone,
			ReadOnly:            true,
			Family:              "web",
			After:               []string{"web_search"},
		},
		webmod.WebFetchBehavior(),
	)
}

func registerSystemTools(
	registry *ragcore.Registry,
	documentService *knowledgeservice.KnowledgeDocumentService,
	taskService *ingestionservice.TaskService,
) {
	spec := ragcore.ToolSpec{
		Capability:          ragcore.CapabilityDiagnosis,
		EvidenceSources:     []string{ragcore.EvidenceSourceSystemRecords},
		ExecutionMode:       ragcore.ExecutionModeReadOnly,
		RiskLevel:           ragcore.RiskLevelLow,
		ApprovalRequirement: ragcore.ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "system",
	}

	registerModule(registry, raginvsystem.NewDocumentListTool(documentService), spec, systemmod.DocumentListBehavior())
	registerModule(registry, raginvsystem.NewTaskListTool(taskService), spec, systemmod.TaskListBehavior())
	registerModule(registry, raginvsystem.NewDocumentQueryTool(documentService), spec, systemmod.DocumentQueryBehavior())

	diagnoseSpec := spec
	diagnoseSpec.After = []string{"document_query", "document_chunk_log_query"}
	documentDiagnoseTool := raginvsystem.NewDocumentIngestionDiagnoseTool(documentService)
	documentDiagnoseTool.SetTaskNodeReader(taskService)
	registerModule(registry, documentDiagnoseTool, diagnoseSpec, systemmod.DocumentIngestionDiagnoseBehavior())

	registerModule(registry, raginvsystem.NewDocumentChunkLogQueryTool(documentService), spec, systemmod.DocumentChunkLogQueryBehavior())
	registerModule(registry, raginvsystem.NewTaskIngestionDiagnoseTool(taskService), spec, systemmod.TaskIngestionDiagnoseBehavior())

	taskQuerySpec := spec
	taskQuerySpec.After = []string{"document_ingestion_diagnose", "document_chunk_log_query", "task_ingestion_diagnose"}
	registerModule(registry, raginvsystem.NewIngestionTaskQueryTool(taskService), taskQuerySpec, systemmod.IngestionTaskQueryBehavior())

	nodeQuerySpec := spec
	nodeQuerySpec.After = []string{"ingestion_task_query", "document_ingestion_diagnose", "task_ingestion_diagnose"}
	registerModule(registry, raginvsystem.NewIngestionTaskNodeQueryTool(taskService), nodeQuerySpec, systemmod.IngestionTaskNodeQueryBehavior())
}

func registerTraceTools(
	registry *ragcore.Registry,
	traceRunRepo *postgresrag.RagTraceRunRepository,
	traceNodeRepo *postgresrag.RagTraceNodeRepository,
) {
	spec := ragcore.ToolSpec{
		Capability:          ragcore.CapabilityDiagnosis,
		EvidenceSources:     []string{ragcore.EvidenceSourceRAGTrace},
		ExecutionMode:       ragcore.ExecutionModeReadOnly,
		RiskLevel:           ragcore.RiskLevelLow,
		ApprovalRequirement: ragcore.ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "trace",
	}

	registerModule(registry, raginvtrace.NewTraceNodeQueryTool(traceRunRepo, traceNodeRepo), spec, tracemod.TraceNodeQueryBehavior())
	registerModule(registry, raginvtrace.NewTraceRetrievalDiagnoseTool(traceRunRepo, traceNodeRepo), spec, tracemod.TraceRetrievalDiagnoseBehavior())
}

func registerGraphTools(registry *ragcore.Registry, executor *ragruntime.Executor, chatService aichat.LLMService) {
	graphTool, err := raginvgraph.NewDiagnosisGraphTool(executor)
	if err != nil {
		log.Warnf("skip graph tool document_root_cause_diagnosis: %v", err)
	} else {
		registry.MustRegisterModule(ragcore.NewLegacyToolAdapterWithBehavior(
			graphTool,
			ragcore.ToolSpec{
				Capability:          ragcore.CapabilityDiagnosis,
				EvidenceSources:     []string{ragcore.EvidenceSourceSystemRecords},
				ExecutionMode:       ragcore.ExecutionModeReadOnly,
				RiskLevel:           ragcore.RiskLevelLow,
				ApprovalRequirement: ragcore.ApprovalRequirementNone,
				ReadOnly:            true,
				Family:              "graph",
			},
			graphmod.DocumentRootCauseDiagnosisBehavior(),
		).Module())
	}

	searchGraphTool, err := raginvgraph.NewDiagnoseSearchGraphTool(executor)
	if err != nil {
		log.Warnf("skip graph tool document_diagnose_with_search: %v", err)
	} else {
		registry.MustRegisterModule(ragcore.NewLegacyToolAdapterWithBehavior(
			searchGraphTool,
			ragcore.ToolSpec{
				Capability:          ragcore.CapabilitySearch,
				EvidenceSources:     []string{ragcore.EvidenceSourceExternalWeb, ragcore.EvidenceSourceSystemRecords},
				ExecutionMode:       ragcore.ExecutionModeReadOnly,
				RiskLevel:           ragcore.RiskLevelLow,
				ApprovalRequirement: ragcore.ApprovalRequirementNone,
				ReadOnly:            true,
				Family:              "graph",
			},
			graphmod.DocumentDiagnoseWithSearchBehavior(),
		).Module())
	}

	externalWorkflowTool, err := raginvgraph.NewExternalEvidenceWorkflowTool(executor, chatService)
	if err != nil {
		log.Warnf("skip graph tool external_evidence_workflow: %v", err)
	} else {
		registry.MustRegisterModule(ragcore.NewLegacyToolAdapterWithBehavior(
			externalWorkflowTool,
			ragcore.ToolSpec{
				Capability:          ragcore.CapabilitySearch,
				EvidenceSources:     []string{ragcore.EvidenceSourceExternalWeb},
				ExecutionMode:       ragcore.ExecutionModeReadOnly,
				RiskLevel:           ragcore.RiskLevelLow,
				ApprovalRequirement: ragcore.ApprovalRequirementNone,
				ReadOnly:            true,
				Family:              "web",
			},
			webmod.ExternalEvidenceWorkflowBehavior(),
		).Module())
	}
}

func buildAgentLoop(executor *ragruntime.Executor, cfg *config.Config, chatService aichat.LLMService) ragcore.Workflow {
	executor.SetMiddlewares(
		&ragruntime.TimeoutMiddleware{Timeout: 60 * time.Second},
		&ragruntime.RetryMiddleware{MaxRetries: 2, BaseDelay: 500 * time.Millisecond},
	)

	wf := ragruntime.NewAgentLoop(executor)
	if cfg != nil {
		wf.SetMaxIterations(cfg.Rag.Agent.MaxIterations)
		wf.SetParallelToolCalls(
			cfg.Rag.Agent.ParallelToolCalls.Enabled,
			cfg.Rag.Agent.ParallelToolCalls.MaxConcurrency,
		)
	}
	if chatService != nil {
		wf.SetPlanner(planner.NewLLMPlanner(chatService))
		wf.SetObserver(ragruntime.NewLLMObserver(chatService))
	}
	return wf
}

func BuildLocalWorkflow(
	db *gorm.DB,
	traceRunRepo *postgresrag.RagTraceRunRepository,
	traceNodeRepo *postgresrag.RagTraceNodeRepository,
	cfg *config.Config,
	mcpManager inframcp.ToolClient,
	chatService aichat.LLMService,
) ragcore.Workflow {
	if db == nil {
		return nil
	}

	documentService, taskService := buildServices(db)

	registry := ragcore.NewRegistry()

	registerMetaTools(registry)
	registerWebTools(registry, cfg, mcpManager)
	registerSystemTools(registry, documentService, taskService)
	registerTraceTools(registry, traceRunRepo, traceNodeRepo)

	executor := ragruntime.NewExecutor(registry)
	registerGraphTools(registry, executor, chatService)

	return buildAgentLoop(executor, cfg, chatService)
}

func registerModule(
	registry *ragcore.Registry,
	tool ragcore.Tool,
	spec ragcore.ToolSpec,
	behavior ragcore.ToolBehavior,
) {
	registry.MustRegisterModule(ragcore.NewLegacyToolAdapterWithBehavior(tool, spec, behavior).Module())
}

func buildSearchProvider(cfg *config.Config, mcpManager inframcp.ToolClient) searchprovider.SearchProvider {
	return searchprovider.BuildProvider(cfg, mcpManager)
}

func fallbackSearchProvider(name string, tavilyAPIKey string) searchprovider.SearchProvider {
	return searchprovider.BuildProvider(&config.Config{
		Rag: config.RagConfig{
			Search: config.RagSearchConfig{
				WebSearch: config.RagWebSearchConfig{
					Provider:         strings.TrimSpace(name),
					FallbackProvider: "",
					ApiKey:           strings.TrimSpace(tavilyAPIKey),
				},
			},
		},
	}, nil)
}

func buildSourcePolicyEngine(cfg *config.Config) *searchprovider.SourcePolicyEngine {
	return searchprovider.BuildSourcePolicy(cfg)
}
