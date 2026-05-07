package builtin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragdomain "local/rag-project/internal/app/rag/domain"
	ragport "local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type documentGetterStub struct {
	document knowledgedomain.KnowledgeDocument
	err      error
}

func (s *documentGetterStub) Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return s.document, s.err
}

func (s *documentGetterStub) PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	if s.err != nil {
		return knowledgeservice.KnowledgeDocumentChunkLogPageResult{}, s.err
	}
	return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
		Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
			{
				Log: knowledgedomain.KnowledgeDocumentChunkLog{
					ID:            "task-1",
					DocumentID:    input.DocumentID,
					Status:        knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
					ProcessMode:   knowledgedomain.KnowledgeDocumentProcessModePipeline,
					PipelineID:    "pipe-1",
					ChunkCount:    0,
					ErrorMessage:  "indexer failed",
					TotalDuration: 3200,
				},
				IngestionTask: &ingestiondomain.Task{
					ID:         "task-1",
					PipelineID: "pipe-1",
					Status:     ingestiondomain.TaskStatusFailed,
				},
				IngestionNodes: []ingestiondomain.TaskNode{
					{NodeID: "fetcher", NodeType: "fetcher", Status: ingestiondomain.TaskStatusSuccess},
					{NodeID: "indexer", NodeType: "indexer", Status: ingestiondomain.TaskStatusFailed, ErrorMessage: "connection refused"},
				},
			},
		},
		Total:    1,
		Page:     1,
		PageSize: 3,
	}, nil
}

type ingestionTaskGetterStub struct {
	task    ingestiondomain.Task
	nodes   []ingestiondomain.TaskNode
	node    ingestiondomain.TaskNode
	err     error
	nodeErr error
}

func (s *ingestionTaskGetterStub) Get(ctx context.Context, id string) (ingestiondomain.Task, error) {
	return s.task, s.err
}

func (s *ingestionTaskGetterStub) ListNodes(ctx context.Context, taskID string) ([]ingestiondomain.TaskNode, error) {
	return s.nodes, s.err
}

func (s *ingestionTaskGetterStub) GetNode(ctx context.Context, taskID string, nodeID string) (ingestiondomain.TaskNode, error) {
	return s.node, s.nodeErr
}

type traceRunRepoStub struct {
	run ragdomain.RagTraceRun
	err error
}

func (s *traceRunRepoStub) Create(ctx context.Context, run ragdomain.RagTraceRun) (ragdomain.RagTraceRun, error) {
	return ragdomain.RagTraceRun{}, nil
}
func (s *traceRunRepoStub) UpdateByTraceID(ctx context.Context, traceID string, run ragdomain.RagTraceRun) error {
	return nil
}
func (s *traceRunRepoStub) UpdateWhere(ctx context.Context, cond ragport.RagTraceRunConditions, patch ragport.RagTraceRunPatch) (int64, error) {
	return 0, nil
}
func (s *traceRunRepoStub) GetByTraceID(ctx context.Context, traceID string) (ragdomain.RagTraceRun, error) {
	return s.run, s.err
}
func (s *traceRunRepoStub) Count(ctx context.Context, filter ragport.RagTraceRunListFilter) (int, error) {
	return 0, nil
}
func (s *traceRunRepoStub) List(ctx context.Context, filter ragport.RagTraceRunListFilter) ([]ragdomain.RagTraceRun, error) {
	return nil, nil
}

type traceNodeRepoStub struct {
	nodes []ragdomain.RagTraceNode
	err   error
}

func (s *traceNodeRepoStub) Create(ctx context.Context, node ragdomain.RagTraceNode) (ragdomain.RagTraceNode, error) {
	return ragdomain.RagTraceNode{}, nil
}
func (s *traceNodeRepoStub) UpdateByTraceIDAndNodeID(ctx context.Context, traceID string, nodeID string, node ragdomain.RagTraceNode) error {
	return nil
}
func (s *traceNodeRepoStub) UpdateWhere(ctx context.Context, cond ragport.RagTraceNodeConditions, patch ragport.RagTraceNodePatch) (int64, error) {
	return 0, nil
}
func (s *traceNodeRepoStub) ListByTraceID(ctx context.Context, traceID string) ([]ragdomain.RagTraceNode, error) {
	return s.nodes, s.err
}

func TestDocumentQueryToolInvoke(t *testing.T) {
	tool := NewDocumentQueryTool(&documentGetterStub{
		document: knowledgedomain.KnowledgeDocument{
			ID:              "doc-1",
			Name:            "Manual",
			KnowledgeBaseID: "kb-1",
			Status:          knowledgedomain.KnowledgeDocumentStatusSuccess,
			Enabled:         true,
			ProcessMode:     knowledgedomain.KnowledgeDocumentProcessModePipeline,
			PipelineID:      "pipe-1",
			ChunkCount:      12,
			SourceType:      knowledgedomain.KnowledgeDocumentSourceFile,
		},
	})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "document_query",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke document query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "status=success") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}

func TestDocumentChunkLogQueryToolInvoke(t *testing.T) {
	tool := NewDocumentChunkLogQueryTool(&documentGetterStub{})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "document_chunk_log_query",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke document chunk log query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "latestStatus=failed") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "failed=[task-1[indexer(connection refused)]]") {
		t.Fatalf("unexpected failed summary: %q", result.Summary)
	}
}

func TestDocumentIngestionDiagnoseToolInvoke(t *testing.T) {
	tool := NewDocumentIngestionDiagnoseTool(&documentGetterStub{
		document: knowledgedomain.KnowledgeDocument{
			ID:          "doc-1",
			Name:        "Manual",
			Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
			ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
			PipelineID:  "pipe-1",
		},
	})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "document_ingestion_diagnose",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke document ingestion diagnose: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "confidence=high") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "node=indexer") {
		t.Fatalf("expected indexer node in summary: %q", result.Summary)
	}
	conclusion, _ := result.Data["conclusion"].(string)
	if !strings.Contains(conclusion, "failed at node indexer") {
		t.Fatalf("unexpected conclusion: %q", conclusion)
	}
	evidence, _ := result.Data["evidence"].([]string)
	if len(evidence) == 0 {
		t.Fatal("expected diagnosis evidence")
	}
}

func TestIngestionTaskQueryToolInvokeWithNodes(t *testing.T) {
	startedAt := time.Now()
	tool := NewIngestionTaskQueryTool(&ingestionTaskGetterStub{
		task: ingestiondomain.Task{
			ID:         "task-1",
			PipelineID: "pipe-1",
			Status:     ingestiondomain.TaskStatusRunning,
			SourceType: ingestiondomain.TaskSourceTypeFile,
			StartedAt:  &startedAt,
		},
		nodes: []ingestiondomain.TaskNode{
			{NodeID: "fetcher", NodeType: "fetcher", Status: ingestiondomain.TaskStatusSuccess},
			{NodeID: "indexer", NodeType: "indexer", Status: ingestiondomain.TaskStatusRunning},
		},
	})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "ingestion_task_query",
		Arguments: map[string]any{
			"taskId":       "task-1",
			"includeNodes": true,
		},
	})
	if err != nil {
		t.Fatalf("invoke ingestion task query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "nodes=2") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}

func TestTaskIngestionDiagnoseToolInvoke(t *testing.T) {
	tool := NewTaskIngestionDiagnoseTool(&ingestionTaskGetterStub{
		task: ingestiondomain.Task{
			ID:           "task-1",
			PipelineID:   "pipe-1",
			Status:       ingestiondomain.TaskStatusFailed,
			SourceType:   ingestiondomain.TaskSourceTypeFile,
			ErrorMessage: "indexer failed after retries",
		},
		nodes: []ingestiondomain.TaskNode{
			{NodeID: "fetcher", NodeType: "fetcher", Status: ingestiondomain.TaskStatusSuccess},
			{NodeID: "indexer", NodeType: "indexer", Status: ingestiondomain.TaskStatusFailed, ErrorMessage: "connection refused"},
		},
	})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "task_ingestion_diagnose",
		Arguments: map[string]any{
			"taskId": "task-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke task ingestion diagnose: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "confidence=high") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "node=indexer") {
		t.Fatalf("expected indexer node in summary: %q", result.Summary)
	}
	conclusion, _ := result.Data["conclusion"].(string)
	if !strings.Contains(conclusion, "failed at node indexer") {
		t.Fatalf("unexpected conclusion: %q", conclusion)
	}
	evidence, _ := result.Data["evidence"].([]string)
	if len(evidence) == 0 {
		t.Fatal("expected diagnosis evidence")
	}
}

func TestIngestionTaskNodeQueryToolInvokeAllNodes(t *testing.T) {
	stub := &ingestionTaskGetterStub{
		task: ingestiondomain.Task{
			ID:         "task-1",
			PipelineID: "pipe-1",
			Status:     ingestiondomain.TaskStatusRunning,
		},
		nodes: []ingestiondomain.TaskNode{
			{NodeID: "fetcher", NodeType: "fetcher", NodeOrder: 1, Status: ingestiondomain.TaskStatusSuccess, DurationMs: 1200},
			{NodeID: "parser", NodeType: "parser", NodeOrder: 2, Status: ingestiondomain.TaskStatusSuccess, DurationMs: 800},
			{NodeID: "indexer", NodeType: "indexer", NodeOrder: 3, Status: ingestiondomain.TaskStatusFailed, ErrorMessage: "connection refused"},
		},
	}

	tool := NewIngestionTaskNodeQueryTool(stub)
	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "ingestion_task_node_query",
		Arguments: map[string]any{
			"taskId": "task-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke task node query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "totalNodes=3") {
		t.Fatalf("expected totalNodes=3 in summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "failed=[indexer(connection refused)]") {
		t.Fatalf("expected failed node in summary: %q", result.Summary)
	}
}

func TestIngestionTaskNodeQueryToolInvokeSingleNode(t *testing.T) {
	stub := &ingestionTaskGetterStub{
		node: ingestiondomain.TaskNode{
			NodeID:       "indexer",
			NodeType:     "indexer",
			NodeOrder:    3,
			Status:       ingestiondomain.TaskStatusFailed,
			DurationMs:   5000,
			ErrorMessage: "connection refused",
			Message:      "indexing failed after 3 retries",
		},
	}

	tool := NewIngestionTaskNodeQueryTool(stub)
	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "ingestion_task_node_query",
		Arguments: map[string]any{
			"taskId": "task-1",
			"nodeId": "indexer",
		},
	})
	if err != nil {
		t.Fatalf("invoke task single node query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "type=indexer") {
		t.Fatalf("expected type=indexer in summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "error=connection refused") {
		t.Fatalf("expected error message in summary: %q", result.Summary)
	}
	if !strings.Contains(result.Summary, "duration=5000ms") {
		t.Fatalf("expected duration in summary: %q", result.Summary)
	}
}

func TestIngestionTaskNodeQueryToolSingleNodeNotFound(t *testing.T) {
	stub := &ingestionTaskGetterStub{node: ingestiondomain.TaskNode{}}
	tool := NewIngestionTaskNodeQueryTool(stub)
	_, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "ingestion_task_node_query",
		Arguments: map[string]any{
			"taskId": "task-1",
			"nodeId": "nonexistent",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestTraceNodeQueryToolInvoke(t *testing.T) {
	tool := NewTraceNodeQueryTool(
		&traceRunRepoStub{
			run: ragdomain.RagTraceRun{
				TraceID:        "trace-1",
				Status:         "success",
				ConversationID: "conv-1",
			},
		},
		&traceNodeRepoStub{
			nodes: []ragdomain.RagTraceNode{
				{NodeID: "rewrite", NodeType: "rewrite", NodeName: "query_rewrite", Status: "success"},
				{NodeID: "retrieve", NodeType: "retrieve", NodeName: "vector_retrieve", Status: "success"},
			},
		},
	)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "trace_node_query",
		Arguments: map[string]any{
			"traceId": "trace-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke trace node query: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "nodes=2") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}

func TestTraceRetrievalDiagnoseToolInvoke(t *testing.T) {
	tool := NewTraceRetrievalDiagnoseTool(
		&traceRunRepoStub{
			run: ragdomain.RagTraceRun{
				TraceID:        "trace-1",
				Status:         "success",
				ConversationID: "conv-1",
			},
		},
		&traceNodeRepoStub{
			nodes: []ragdomain.RagTraceNode{
				{NodeID: "rewrite", NodeType: "rewrite", NodeName: "query_rewrite", Status: "success"},
				{NodeID: "retrieve", NodeType: "retrieve", NodeName: "vector_retrieve", Status: "success", ExtraData: `{"chunkCount":0,"searchMode":"hybrid"}`},
				{NodeID: "prompt", NodeType: "prompt", NodeName: "build_messages", Status: "success"},
			},
		},
	)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "trace_retrieval_diagnose",
		Arguments: map[string]any{
			"traceId": "trace-1",
		},
	})
	if err != nil {
		t.Fatalf("invoke trace retrieval diagnose: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if !strings.Contains(result.Summary, "confidence=high") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	conclusion, _ := result.Data["conclusion"].(string)
	if !strings.Contains(conclusion, "returned no chunks") {
		t.Fatalf("unexpected conclusion: %q", conclusion)
	}
}

func TestDiagnoseDocumentIngestionDetectsStateInconsistency(t *testing.T) {
	conclusion, confidence, evidence, _, _, _, _, _, _ := diagnoseDocumentIngestion(
		knowledgedomain.KnowledgeDocument{
			ID:          "doc-3",
			Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
			ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
			PipelineID:  "pipe-1",
		},
		knowledgeservice.KnowledgeDocumentChunkLogPageResult{
			Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{
				{
					Log: knowledgedomain.KnowledgeDocumentChunkLog{
						ID:         "task-9",
						Status:     knowledgedomain.KnowledgeDocumentChunkLogStatusSuccess,
						ChunkCount: 5,
					},
					IngestionTask: &ingestiondomain.Task{
						ID:         "task-9",
						Status:     ingestiondomain.TaskStatusSuccess,
						ChunkCount: 5,
					},
					IngestionNodes: []ingestiondomain.TaskNode{
						{NodeID: "fetcher", Status: ingestiondomain.TaskStatusSuccess},
						{NodeID: "indexer", Status: ingestiondomain.TaskStatusSuccess},
					},
				},
			},
		},
	)
	if !strings.Contains(conclusion, "inconsistent") {
		t.Fatalf("expected inconsistent conclusion, got %q", conclusion)
	}
	if confidence != "medium" {
		t.Fatalf("expected medium confidence, got %q", confidence)
	}
	if len(evidence) == 0 {
		t.Fatal("expected evidence for inconsistent state")
	}
}

func TestTraceRetrievalDiagnoseToolReportsToolWorkflowDegradation(t *testing.T) {
	tool := NewTraceRetrievalDiagnoseTool(
		&traceRunRepoStub{
			run: ragdomain.RagTraceRun{
				TraceID:        "trace-2",
				Status:         "success",
				ConversationID: "conv-2",
			},
		},
		&traceNodeRepoStub{
			nodes: []ragdomain.RagTraceNode{
				{NodeID: "rewrite", NodeType: "rewrite", Status: "success"},
				{NodeID: "retrieve", NodeType: "retrieve", Status: "success", ExtraData: `{"chunkCount":4,"searchMode":"hybrid","topScore":0.92}`},
				{NodeID: "tool_workflow", NodeType: "tool", Status: "success", ExtraData: `{"toolCallCount":2,"degraded":true,"degradeReason":"task lookup failed","toolNames":["document_query","task_ingestion_diagnose"]}`},
				{NodeID: "prompt", NodeType: "prompt", Status: "success"},
			},
		},
	)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "trace_retrieval_diagnose",
		Arguments: map[string]any{
			"traceId": "trace-2",
		},
	})
	if err != nil {
		t.Fatalf("invoke trace retrieval diagnose: %v", err)
	}
	conclusion, _ := result.Data["conclusion"].(string)
	if !strings.Contains(conclusion, "degraded tool calls") {
		t.Fatalf("unexpected conclusion: %q", conclusion)
	}
	if !strings.Contains(result.Summary, "node=tool_workflow") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
}

func TestQueryToolsValidateRequiredArgs(t *testing.T) {
	documentTool := NewDocumentQueryTool(&documentGetterStub{})
	if _, err := documentTool.Invoke(context.Background(), ragtool.Call{Name: "document_query"}); err == nil {
		t.Fatal("expected document query arg validation error")
	}

	documentChunkLogTool := NewDocumentChunkLogQueryTool(&documentGetterStub{})
	if _, err := documentChunkLogTool.Invoke(context.Background(), ragtool.Call{Name: "document_chunk_log_query"}); err == nil {
		t.Fatal("expected document chunk log query arg validation error")
	}

	documentDiagnoseTool := NewDocumentIngestionDiagnoseTool(&documentGetterStub{})
	if _, err := documentDiagnoseTool.Invoke(context.Background(), ragtool.Call{Name: "document_ingestion_diagnose"}); err == nil {
		t.Fatal("expected document ingestion diagnose arg validation error")
	}

	taskTool := NewIngestionTaskQueryTool(&ingestionTaskGetterStub{})
	if _, err := taskTool.Invoke(context.Background(), ragtool.Call{Name: "ingestion_task_query"}); err == nil {
		t.Fatal("expected ingestion task query arg validation error")
	}

	taskDiagnoseTool := NewTaskIngestionDiagnoseTool(&ingestionTaskGetterStub{})
	if _, err := taskDiagnoseTool.Invoke(context.Background(), ragtool.Call{Name: "task_ingestion_diagnose"}); err == nil {
		t.Fatal("expected task ingestion diagnose arg validation error")
	}

	nodeTool := NewIngestionTaskNodeQueryTool(&ingestionTaskGetterStub{})
	if _, err := nodeTool.Invoke(context.Background(), ragtool.Call{Name: "ingestion_task_node_query"}); err == nil {
		t.Fatal("expected ingestion task node query arg validation error")
	}

	traceTool := NewTraceNodeQueryTool(&traceRunRepoStub{}, &traceNodeRepoStub{})
	if _, err := traceTool.Invoke(context.Background(), ragtool.Call{Name: "trace_node_query"}); err == nil {
		t.Fatal("expected trace node query arg validation error")
	}

	traceDiagnoseTool := NewTraceRetrievalDiagnoseTool(&traceRunRepoStub{}, &traceNodeRepoStub{})
	if _, err := traceDiagnoseTool.Invoke(context.Background(), ragtool.Call{Name: "trace_retrieval_diagnose"}); err == nil {
		t.Fatal("expected trace retrieval diagnose arg validation error")
	}
}

func TestQueryToolsPropagateServiceError(t *testing.T) {
	expectedErr := errors.New("backend unavailable")
	tool := NewTraceNodeQueryTool(&traceRunRepoStub{err: expectedErr}, &traceNodeRepoStub{})

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "trace_node_query",
		Arguments: map[string]any{
			"traceId": "trace-1",
		},
	})
	if err == nil {
		t.Fatal("expected service error")
	}
	if result.ErrorMessage != expectedErr.Error() {
		t.Fatalf("unexpected error message: %q", result.ErrorMessage)
	}
}
