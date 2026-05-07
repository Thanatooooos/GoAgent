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
	ingestionmodels "local/rag-project/internal/adapter/repository/postgres/ingestion/models"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	corechunk "local/rag-project/internal/app/core/chunk"
	coreparser "local/rag-project/internal/app/core/parser"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	"local/rag-project/internal/framework/config"
)

// TestPipelineFetcherNotFound 验证 fetcher 读取不到来源文件时，
// task / task_node / chunk_log / document 四个实体一致失败。
func TestPipelineFetcherNotFound(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_PIPELINE") != "1" {
		t.Skip("set RAG_INTEGRATION_PIPELINE=1 to run knowledge/ingestion pipeline failure integration test")
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

	if err := db.AutoMigrate(
		&ingestionmodels.PipelineModel{},
		&ingestionmodels.TaskModel{},
		&ingestionmodels.TaskNodeModel{},
	); err != nil {
		t.Fatalf("auto migrate ingestion tables: %v", err)
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

	// fetcher 不提供 storage，os.ReadFile 对不存在的路径会失败，
	// 随后 fallback 到 fetchStorageObject 也会因为 storage=nil 而失败。
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
		fmt.Sprintf("pipeline-fail-fetch-%d", suffix),
		"integration-embedding",
		fmt.Sprintf("pipeline_fail_fetch_%d", suffix%1000000),
		"integration",
	)
	createdBase, err := baseRepo.Create(ctx, base)
	if err != nil {
		t.Fatalf("create knowledge base: %v", err)
	}

	pipelineService := ingestionservice.NewPipelineService(pipelineRepo)
	createdPipeline, err := pipelineService.Create(ctx, ingestionservice.CreatePipelineInput{
		Name:        fmt.Sprintf("pipeline-fail-fetch-%d", suffix),
		Description: "fetcher failure integration test pipeline",
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

	// 构造一个不存在的文件路径，确保 fetcher 读取失败。
	nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist.md")

	documentID := fmt.Sprintf("%d", (suffix+1)%1000000000000000000)
	document := knowledgedomain.NewUploadedKnowledgeDocument(
		documentID,
		createdBase.ID,
		"missing-file.md",
		nonExistentPath,
		"text/markdown",
		"integration",
		0,
	)
	document.ProcessMode = knowledgedomain.KnowledgeDocumentProcessModePipeline
	document.PipelineID = createdPipeline.ID
	document.SourceType = knowledgedomain.KnowledgeDocumentSourceFile
	document.SourceLocation = nonExistentPath

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

	task, err := waitForPipelineTaskFinal(ctx, taskService, taskID)
	if err != nil {
		t.Fatalf("wait for pipeline task final: %v", err)
	}
	if task.Status != ingestiondomain.TaskStatusFailed {
		t.Fatalf("expected task status failed, got %q (errorMessage=%s)", task.Status, task.ErrorMessage)
	}
	if task.ErrorMessage == "" {
		t.Fatal("expected task error message to be non-empty")
	}

	// 验证 fetcher 节点失败，后续节点未启动。
	taskNodes, err := taskService.ListNodes(ctx, taskID)
	if err != nil {
		t.Fatalf("list task nodes: %v", err)
	}
	if len(taskNodes) < 1 {
		t.Fatalf("expected at least 1 task node, got %d", len(taskNodes))
	}
	fetcherNode := taskNodes[0]
	if fetcherNode.Status != ingestiondomain.TaskStatusFailed {
		t.Fatalf("expected fetcher node status failed, got %q", fetcherNode.Status)
	}
	if fetcherNode.ErrorMessage == "" {
		t.Fatal("expected fetcher node error message to be non-empty")
	}
	for i, node := range taskNodes {
		if i == 0 {
			continue
		}
		if node.Status == ingestiondomain.TaskStatusRunning {
			t.Fatalf("expected node %s to not be running after task failure", node.NodeID)
		}
	}

	// 验证 document 已回写为 failed。
	persistedDocument, err := documentRepo.GetByID(ctx, createdDocument.ID)
	if err != nil {
		t.Fatalf("reload knowledge document: %v", err)
	}
	if persistedDocument.Status != knowledgedomain.KnowledgeDocumentStatusFailed {
		t.Fatalf("expected knowledge document status failed, got %q", persistedDocument.Status)
	}

	// 验证 chunk_log 已回写为 failed。
	chunkLog, err := chunkLogRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get chunk log by task id: %v", err)
	}
	if chunkLog.Status != knowledgedomain.KnowledgeDocumentChunkLogStatusFailed {
		t.Fatalf("expected chunk log status failed, got %q (errorMessage=%s)", chunkLog.Status, chunkLog.ErrorMessage)
	}
	if chunkLog.ErrorMessage == "" {
		t.Fatal("expected chunk log error message to be non-empty")
	}
}

// TestPipelineIndexerMissingDependency 验证 indexer 缺少下游依赖时，
// task / task_node / chunk_log / document 四个实体一致失败。
func TestPipelineIndexerMissingDependency(t *testing.T) {
	if os.Getenv("RAG_INTEGRATION_PIPELINE") != "1" {
		t.Skip("set RAG_INTEGRATION_PIPELINE=1 to run knowledge/ingestion pipeline failure integration test")
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

	if err := db.AutoMigrate(
		&ingestionmodels.PipelineModel{},
		&ingestionmodels.TaskModel{},
		&ingestionmodels.TaskNodeModel{},
	); err != nil {
		t.Fatalf("auto migrate ingestion tables: %v", err)
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

	// indexer 不注入 vectorStore 与 embedding，让它在执行时直接失败。
	nodeRunners := ingestionservice.NewNodeRunnerRegistry(
		ingestionservice.NewFetcherNodeRunner(nil, nil),
		ingestionservice.NewParserNodeRunner(coreparser.NewDefaultSelector(nil)),
		ingestionservice.NewChunkerNodeRunner(corechunk.NewDefaultSelector()),
		ingestionservice.NewIndexerNodeRunner(baseRepo, nil, nil, nil),
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
		fmt.Sprintf("pipeline-fail-idx-%d", suffix),
		"integration-embedding",
		fmt.Sprintf("pipeline_fail_idx_%d", suffix%1000000),
		"integration",
	)
	createdBase, err := baseRepo.Create(ctx, base)
	if err != nil {
		t.Fatalf("create knowledge base: %v", err)
	}

	pipelineService := ingestionservice.NewPipelineService(pipelineRepo)
	createdPipeline, err := pipelineService.Create(ctx, ingestionservice.CreatePipelineInput{
		Name:        fmt.Sprintf("pipeline-fail-idx-%d", suffix),
		Description: "indexer failure integration test pipeline",
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
				NodeID:     "chunk-content",
				NodeType:   ingestiondomain.PipelineNodeTypeChunker,
				NextNodeID: "index-content",
				Settings: map[string]any{
					"strategy":    "fixed_size",
					"chunkSize":   80,
					"overlapSize": 0,
				},
			},
			{
				NodeID:   "index-content",
				NodeType: ingestiondomain.PipelineNodeTypeIndexer,
				Settings: map[string]any{
					"target": "knowledge",
				},
			},
		},
		CreatedBy: "integration",
	})
	if err != nil {
		t.Fatalf("create ingestion pipeline: %v", err)
	}

	// 写入真实源文件，确保 fetcher / parser / chunker 都能成功。
	content := []byte("# Indexer Failure Test\n\nThis document should be fetched, parsed, and chunked but the indexer will fail.")
	sourcePath := filepath.Join(t.TempDir(), "indexer-failure.md")
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	documentID := fmt.Sprintf("%d", (suffix+1)%1000000000000000000)
	document := knowledgedomain.NewUploadedKnowledgeDocument(
		documentID,
		createdBase.ID,
		"indexer-failure.md",
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

	task, err := waitForPipelineTaskFinal(ctx, taskService, taskID)
	if err != nil {
		t.Fatalf("wait for pipeline task final: %v", err)
	}
	if task.Status != ingestiondomain.TaskStatusFailed {
		t.Fatalf("expected task status failed, got %q (errorMessage=%s)", task.Status, task.ErrorMessage)
	}

	// 验证前三个节点成功（fetcher / parser / chunker），最后一个节点失败（indexer）。
	taskNodes, err := taskService.ListNodes(ctx, taskID)
	if err != nil {
		t.Fatalf("list task nodes: %v", err)
	}
	if len(taskNodes) < 4 {
		t.Fatalf("expected at least 4 task nodes, got %d", len(taskNodes))
	}
	for i, node := range taskNodes {
		switch node.NodeType {
		case ingestiondomain.PipelineNodeTypeFetcher, ingestiondomain.PipelineNodeTypeParser, ingestiondomain.PipelineNodeTypeChunker:
			if node.Status != ingestiondomain.TaskStatusSuccess {
				t.Fatalf("expected node %s (type=%s order=%d) to succeed, got status=%q error=%s",
					node.NodeID, node.NodeType, i, node.Status, node.ErrorMessage)
			}
		case ingestiondomain.PipelineNodeTypeIndexer:
			if node.Status != ingestiondomain.TaskStatusFailed {
				t.Fatalf("expected indexer node %s to fail, got status=%q", node.NodeID, node.Status)
			}
			if node.ErrorMessage == "" {
				t.Fatal("expected indexer node error message to be non-empty")
			}
		}
	}

	// 验证 document 已回写为 failed。
	persistedDocument, err := documentRepo.GetByID(ctx, createdDocument.ID)
	if err != nil {
		t.Fatalf("reload knowledge document: %v", err)
	}
	if persistedDocument.Status != knowledgedomain.KnowledgeDocumentStatusFailed {
		t.Fatalf("expected knowledge document status failed, got %q", persistedDocument.Status)
	}

	// 验证 chunk_log 已回写为 failed。
	chunkLog, err := chunkLogRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		t.Fatalf("get chunk log by task id: %v", err)
	}
	if chunkLog.Status != knowledgedomain.KnowledgeDocumentChunkLogStatusFailed {
		t.Fatalf("expected chunk log status failed, got %q (errorMessage=%s)", chunkLog.Status, chunkLog.ErrorMessage)
	}
}

// waitForPipelineTaskFinal 等待 task 进入最终状态（success 或 failed）。
func waitForPipelineTaskFinal(
	ctx context.Context,
	taskService *ingestionservice.TaskService,
	taskID string,
) (ingestiondomain.Task, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		task, err := taskService.Get(ctx, taskID)
		if err == nil {
			switch task.Status {
			case ingestiondomain.TaskStatusSuccess, ingestiondomain.TaskStatusFailed:
				return task, nil
			}
		}

		select {
		case <-ctx.Done():
			return ingestiondomain.Task{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
