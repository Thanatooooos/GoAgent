package service_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresingestion "local/rag-project/internal/adapter/repository/postgres/ingestion"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/config"
)

func TestKnowledgeDocumentPipelineIntegration(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_PIPELINE") != "1" {
		t.Skip("set RAG_INTEGRATION_PIPELINE=1 to run knowledge/ingestion pipeline integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := postgresrepo.NewGormDB(config.DataSourceConfig{
		Url:      getenvDefault("POSTGRES_URL", "jdbc:postgresql://postgres:5432/ragent"),
		Username: getenvDefault("POSTGRES_USER", "postgres"),
		Password: getenvDefault("POSTGRES_PASSWORD", "postgres"),
	})
	if err != nil {
		t.Fatalf("new postgres db: %v", err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pipelineRepo := postgresingestion.NewPipelineRepository(db)
	taskRepo := postgresingestion.NewTaskRepository(db)
	taskNodeRepo := postgresingestion.NewTaskNodeRepository(db)
	baseRepo := postgresknowledge.NewKnowledgeBaseRepository(db)
	documentRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	chunkLogRepo := postgresknowledge.NewKnowledgeDocumentChunkLogRepository(db)

	documentService := knowledgeservice.NewKnowledgeDocumentService(
		baseRepo,
		documentRepo,
		nil,
		chunkLogRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	nodeRunners := ingestionservice.NewNodeRunnerRegistry(
		ingestionservice.NewFetcherNodeRunner(nil, nil),
		ingestionservice.NewParserNodeRunner(coreparser.NewDefaultSelector(nil)),
		ingestionservice.NewChunkerNodeRunner(corechunk.NewDefaultSelector()),
	)
	executor := ingestionservice.NewExecutorService(ingestionservice.ExecutorServiceOptions{
		TaskRepo:        taskRepo,
		TaskNodeRepo:    taskNodeRepo,
		WorkflowBuilder: ingestionservice.NewLinearWorkflowBuilder(),
		NodeRunners:     nodeRunners,
		TaskObserver: ingestionservice.NewMultiTaskObserver(
			ingestionservice.NewRepositoryTaskObserver(taskRepo, taskNodeRepo),
			knowledgebootstrap.NewIngestionTaskObserver(documentService),
		),
		MaxConcurrent: 1,
	})
	t.Cleanup(executor.Close)

	taskService := ingestionservice.NewTaskService(pipelineRepo, taskRepo, taskNodeRepo, executor)
	documentService.SetIngestionTaskCreator(knowledgebootstrap.NewIngestionTaskCreator(taskService))
	documentService.SetIngestionTaskReader(knowledgebootstrap.NewIngestionTaskReader(taskService))

	suffix := time.Now().UnixNano()
	base := knowledgedomain.NewKnowledgeBase(
		fmt.Sprintf("%d", suffix%1000000000000000000),
		fmt.Sprintf("pipeline-it-%d", suffix),
		"integration-embedding",
		fmt.Sprintf("pipeline_it_%d", suffix%1000000),
		"integration",
	)
	createdBase, err := baseRepo.Create(ctx, base)
	if err != nil {
		t.Fatalf("create knowledge base: %v", err)
	}

	pipelineService := ingestionservice.NewPipelineService(pipelineRepo)
	createdPipeline, err := pipelineService.Create(ctx, ingestionservice.CreatePipelineInput{
		Name:        fmt.Sprintf("pipeline-it-%d", suffix),
		Description: "knowledge -> ingestion integration pipeline",
		Nodes: []ingestiondomain.PipelineNode{
			{
				NodeID:     "fetch-source",
				NodeType:   ingestiondomain.PipelineNodeTypeFetcher,
				NextNodeID: "parse-markdown",
			},
			{
				NodeID:     "parse-markdown",
				NodeType:   ingestiondomain.PipelineNodeTypeParser,
				NextNodeID: "chunk-content",
			},
			{
				NodeID:   "chunk-content",
				NodeType: ingestiondomain.PipelineNodeTypeChunker,
				Settings: map[string]any{
					"strategy":    "fixed_size",
					"chunkSize":   80,
					"overlapSize": 0,
				},
			},
		},
		CreatedBy: "integration",
	})
	if err != nil {
		t.Fatalf("create ingestion pipeline: %v", err)
	}

	content := []byte("# Pipeline Integration\n\nThis document should be fetched, parsed, chunked, and written back to knowledge execution logs.\n\nIt contains enough text to produce at least one chunk.")
	sourcePath := filepath.Join(t.TempDir(), "pipeline-integration.md")
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	documentID := fmt.Sprintf("%d", (suffix+1)%1000000000000000000)
	document := knowledgedomain.NewUploadedKnowledgeDocument(
		documentID,
		createdBase.ID,
		"pipeline-integration.md",
		sourcePath,
		"text/markdown",
		"integration",
		int64(len(content)),
	)
	document.ProcessMode = knowledgedomain.KnowledgeDocumentProcessModePipeline
	document.PipelineID = createdPipeline.ID
	document.SourceType = knowledgedomain.KnowledgeDocumentSourceFile
	document.SourceLocation = sourcePath

	createdDocument, err := documentRepo.Create(ctx, document)
	if err != nil {
		t.Fatalf("create knowledge document: %v", err)
	}

	var taskID string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if taskID != "" {
			_ = db.WithContext(cleanupCtx).Exec("delete from t_ingestion_task_node where task_id = ?", taskID).Error
			_ = db.WithContext(cleanupCtx).Exec("delete from t_ingestion_task where id = ?", taskID).Error
		}
		_ = db.WithContext(cleanupCtx).Exec("delete from t_knowledge_document_chunk_log where document_id = ?", createdDocument.ID).Error
		_ = db.WithContext(cleanupCtx).Exec("delete from t_knowledge_document where id = ?", createdDocument.ID).Error
		_ = db.WithContext(cleanupCtx).Exec("delete from t_ingestion_pipeline where id = ?", createdPipeline.ID).Error
		_ = db.WithContext(cleanupCtx).Exec("delete from t_knowledge_base where id = ?", createdBase.ID).Error
	})

	if err := documentService.StartChunk(ctx, knowledgeservice.StartChunkKnowledgeDocumentInput{
		DocumentID: createdDocument.ID,
		OperatorID: "integration",
	}); err != nil {
		t.Fatalf("start pipeline chunk: %v", err)
	}

	taskID, err = waitForPipelineTaskID(ctx, chunkLogRepo, createdDocument.ID)
	if err != nil {
		t.Fatalf("wait for pipeline task id: %v", err)
	}

	task, err := waitForPipelineTaskCompletion(ctx, taskService, chunkLogRepo, taskID)
	if err != nil {
		t.Fatalf("wait for pipeline task completion: %v", err)
	}
	if task.Status != ingestiondomain.TaskStatusSuccess {
		t.Fatalf("unexpected task status: %q", task.Status)
	}
	if task.Metadata["documentId"] != createdDocument.ID {
		t.Fatalf("unexpected task document metadata: %+v", task.Metadata)
	}
	if task.Metadata["knowledgeBaseId"] != createdBase.ID {
		t.Fatalf("unexpected task kb metadata: %+v", task.Metadata)
	}

	persistedDocument, err := documentRepo.GetByID(ctx, createdDocument.ID)
	if err != nil {
		t.Fatalf("reload knowledge document: %v", err)
	}
	if persistedDocument.Status != knowledgedomain.KnowledgeDocumentStatusSuccess {
		t.Fatalf("unexpected knowledge document status: %q", persistedDocument.Status)
	}
	if persistedDocument.ChunkCount <= 0 {
		t.Fatalf("expected knowledge document chunk count > 0, got %d", persistedDocument.ChunkCount)
	}

	taskNodes, err := taskService.ListNodes(ctx, taskID)
	if err != nil {
		t.Fatalf("list task nodes: %v", err)
	}
	if len(taskNodes) != 3 {
		t.Fatalf("expected 3 task nodes, got %d", len(taskNodes))
	}
	for _, node := range taskNodes {
		if node.Status != ingestiondomain.TaskStatusSuccess {
			t.Fatalf("unexpected node %s status: %q", node.NodeID, node.Status)
		}
	}

	chunkLog, err := chunkLogRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get chunk log by task id: %v", err)
	}
	if chunkLog.Status != knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess {
		t.Fatalf("unexpected chunk log status: %q", chunkLog.Status)
	}
	if chunkLog.ProcessMode != knowledgedomain.KnowledgeDocumentProcessModePipeline {
		t.Fatalf("unexpected chunk log process mode: %q", chunkLog.ProcessMode)
	}
	if chunkLog.ChunkCount != persistedDocument.ChunkCount {
		t.Fatalf("chunk count mismatch: log=%d doc=%d", chunkLog.ChunkCount, persistedDocument.ChunkCount)
	}

	pageResult, err := documentService.PageChunkLogs(ctx, knowledgeservice.KnowledgeDocumentChunkLogPageInput{
		DocumentID: createdDocument.ID,
		Page:       1,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("page chunk logs: %v", err)
	}
	if len(pageResult.Items) != 1 {
		t.Fatalf("expected 1 chunk log item, got %d", len(pageResult.Items))
	}
	logItem := pageResult.Items[0]
	if logItem.IngestionTask == nil || logItem.IngestionTask.ID != taskID {
		t.Fatalf("expected chunk log ingestion task to be populated, got %+v", logItem.IngestionTask)
	}
	if len(logItem.IngestionNodes) != len(taskNodes) {
		t.Fatalf("expected %d ingestion nodes, got %d", len(taskNodes), len(logItem.IngestionNodes))
	}
	if logItem.Log.Status != knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess {
		t.Fatalf("unexpected enriched chunk log status: %q", logItem.Log.Status)
	}
}

func waitForPipelineTaskID(
	ctx context.Context,
	chunkLogRepo knowledgeport.KnowledgeDocumentChunkLogRepository,
	documentID string,
) (string, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		logs, err := chunkLogRepo.ListByDocumentID(ctx, documentID, knowledgeport.ListOptions{Offset: 0, Limit: 10})
		if err != nil {
			return "", err
		}
		if len(logs) > 0 && logs[0].ID != "" {
			return logs[0].ID, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForPipelineTaskCompletion(
	ctx context.Context,
	taskService *ingestionservice.TaskService,
	chunkLogRepo knowledgeport.KnowledgeDocumentChunkLogRepository,
	taskID string,
) (ingestiondomain.Task, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := taskService.Get(ctx, taskID)
		if err == nil {
			switch task.Status {
			case ingestiondomain.TaskStatusSuccess:
				return task, nil
			case ingestiondomain.TaskStatusFailed:
				chunkLog, _ := chunkLogRepo.GetByTaskID(ctx, taskID)
				return task, fmt.Errorf("task failed: %s (chunkLog=%s)", task.ErrorMessage, chunkLog.ErrorMessage)
			}
		}

		select {
		case <-ctx.Done():
			return ingestiondomain.Task{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
