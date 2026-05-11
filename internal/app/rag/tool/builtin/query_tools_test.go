package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionservice "local/rag-project/internal/app/ingestion/service"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
	ragdomain "local/rag-project/internal/app/rag/domain"
	ragport "local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool"
)

type documentGetterStub struct {
	document   knowledgedomain.KnowledgeDocument
	pageResult knowledgeservice.KnowledgeDocumentPageResult
	err        error
}

func (s *documentGetterStub) Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return s.document, s.err
}

func (s *documentGetterStub) Page(ctx context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error) {
	if s.err != nil {
		return knowledgeservice.KnowledgeDocumentPageResult{}, s.err
	}
	return s.pageResult, nil
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
	task       ingestiondomain.Task
	nodes      []ingestiondomain.TaskNode
	node       ingestiondomain.TaskNode
	pageResult ingestionservice.TaskPageResult
	err        error
	nodeErr    error
}

func (s *ingestionTaskGetterStub) Page(ctx context.Context, input ingestionservice.PageTasksInput) (ingestionservice.TaskPageResult, error) {
	if s.err != nil {
		return ingestionservice.TaskPageResult{}, s.err
	}
	return s.pageResult, nil
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
	facts, _ := result.Data["facts"].([]string)
	if len(facts) == 0 {
		t.Fatal("expected diagnosis facts")
	}
	if strings.Contains(facts[0], "document.status=") {
		t.Fatalf("expected humanized facts, got %q", facts[0])
	}
	rawEvidence, _ := result.Data["rawEvidence"].([]string)
	if len(rawEvidence) == 0 || !strings.Contains(rawEvidence[0], "document.status=") {
		t.Fatalf("expected rawEvidence to preserve raw fields, got %+v", rawEvidence)
	}
	if scope, _ := result.Data["diagnosisScope"].(string); scope != "document_ingestion" {
		t.Fatalf("unexpected diagnosis scope: %q", scope)
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
	if !strings.Contains(result.Summary, "interestingNodes=[indexer(status=running,type=indexer)]") {
		t.Fatalf("expected interesting node summary, got %q", result.Summary)
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
	facts, _ := result.Data["facts"].([]string)
	if len(facts) == 0 {
		t.Fatal("expected humanized facts")
	}
	if strings.Contains(facts[0], "task.status=") {
		t.Fatalf("expected humanized task facts, got %q", facts[0])
	}
	if scope, _ := result.Data["diagnosisScope"].(string); scope != "task_ingestion" {
		t.Fatalf("unexpected diagnosis scope: %q", scope)
	}
	if nextActions, _ := result.Data["nextActions"].([]string); len(nextActions) == 0 {
		t.Fatal("expected nextActions")
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
	if scope, _ := result.Data["diagnosisScope"].(string); scope != "trace_retrieval" {
		t.Fatalf("unexpected diagnosis scope: %q", scope)
	}
}

func TestDiagnoseDocumentIngestionDetectsStateInconsistency(t *testing.T) {
	conclusion, confidence, evidence, _, _, _, _, _, _ := diagnoseDocumentIngestion(context.Background(),
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
	nil)
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
	if riskHints, _ := result.Data["riskHints"].([]string); len(riskHints) == 0 {
		t.Fatal("expected risk hints for degraded tool workflow")
	}
}

func TestTraceRetrievalDiagnoseToolReportsWeakTopScore(t *testing.T) {
	tool := NewTraceRetrievalDiagnoseTool(
		&traceRunRepoStub{
			run: ragdomain.RagTraceRun{
				TraceID:        "trace-3",
				Status:         "success",
				ConversationID: "conv-3",
			},
		},
		&traceNodeRepoStub{
			nodes: []ragdomain.RagTraceNode{
				{NodeID: "rewrite", NodeType: "rewrite", Status: "success"},
				{NodeID: "retrieve", NodeType: "retrieve", Status: "success", ExtraData: `{"chunkCount":4,"searchMode":"hybrid","topScore":0.21}`},
				{NodeID: "prompt", NodeType: "prompt", Status: "success"},
			},
		},
	)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "trace_retrieval_diagnose",
		Arguments: map[string]any{
			"traceId": "trace-3",
		},
	})
	if err != nil {
		t.Fatalf("invoke trace retrieval diagnose: %v", err)
	}
	conclusion, _ := result.Data["conclusion"].(string)
	if !strings.Contains(conclusion, "top retrieval score is weak") {
		t.Fatalf("unexpected conclusion: %q", conclusion)
	}
	if confidence, _ := result.Data["confidence"].(string); confidence != diagnosisConfidenceMedium {
		t.Fatalf("unexpected confidence: %q", confidence)
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

func TestDocumentListToolInvokeWithStatusFilter(t *testing.T) {
	stub := &documentGetterStub{
		pageResult: knowledgeservice.KnowledgeDocumentPageResult{
			Items: []knowledgedomain.KnowledgeDocument{
				{
					ID:         "doc-1",
					Name:       "test_doc_1.md",
					Status:     knowledgedomain.KnowledgeDocumentStatusFailed,
					ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				},
				{
					ID:         "doc-2",
					Name:       "test_doc_2.md",
					Status:     knowledgedomain.KnowledgeDocumentStatusFailed,
					ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
				},
			},
			Total:    2,
			Page:     1,
			PageSize: 20,
		},
	}
	tool := NewDocumentListTool(stub)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "document_list",
		Arguments: map[string]any{
			"knowledgeBaseId": "kb-1",
			"status":          "failed",
		},
	})
	if err != nil {
		t.Fatalf("invoke document_list: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("expected success, got %q", result.Status)
	}
	if !strings.Contains(result.Summary, "found 2 documents") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	failedCount, _ := result.Data["failedCount"].(int)
	if failedCount != 2 {
		t.Fatalf("expected failedCount=2, got %d", failedCount)
	}
}

func TestDocumentListToolInvokeEmptyResult(t *testing.T) {
	stub := &documentGetterStub{
		pageResult: knowledgeservice.KnowledgeDocumentPageResult{Total: 0},
	}
	tool := NewDocumentListTool(stub)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name:      "document_list",
		Arguments: map[string]any{"status": "running", "knowledgeBaseId": "kb-1"},
	})
	if err != nil {
		t.Fatalf("invoke document_list: %v", err)
	}
	if !strings.Contains(result.Summary, "no documents found") {
		t.Fatalf("expected empty summary, got %q", result.Summary)
	}
}

func TestTaskListToolInvokeWithStatusFilter(t *testing.T) {
	stub := &ingestionTaskGetterStub{
		pageResult: ingestionservice.TaskPageResult{
			Items: []ingestiondomain.Task{
				{
					ID:             "task-1",
					PipelineID:     "pipe-1",
					Status:         ingestiondomain.TaskStatusRunning,
					SourceFileName: "test.md",
					ChunkCount:     120,
				},
				{
					ID:             "task-2",
					PipelineID:     "pipe-1",
					Status:         ingestiondomain.TaskStatusRunning,
					SourceFileName: "test2.md",
					ChunkCount:     45,
				},
			},
			Total:    2,
			Page:     1,
			PageSize: 20,
		},
	}
	tool := NewTaskListTool(stub)

	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name: "task_list",
		Arguments: map[string]any{
			"status": "running",
		},
	})
	if err != nil {
		t.Fatalf("invoke task_list: %v", err)
	}
	if result.Status != ragtool.CallStatusSuccess {
		t.Fatalf("expected success, got %q", result.Status)
	}
	if !strings.Contains(result.Summary, "found 2 tasks") {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	runningCount, _ := result.Data["runningCount"].(int)
	if runningCount != 2 {
		t.Fatalf("expected runningCount=2, got %d", runningCount)
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

func TestWebSearchToolParsesResultsFromJSON(t *testing.T) {
	sample := `{
		"AbstractText": "Vector databases can experience connection refused errors when the service is not running.",
		"AbstractURL": "https://example.com/vector-db-errors",
		"Heading": "Vector Database Errors",
		"RelatedTopics": [
			{
				"FirstURL": "https://example.com/troubleshooting",
				"Text": "Connection Refused Troubleshooting - Common causes include port blocking, firewall rules, and service not started."
			},
			{
				"FirstURL": "https://example.com/monitoring",
				"Text": "Monitoring Vector Store Health - Set up health checks to detect connection issues early."
			}
		]
	}`

	var parsed duckDuckGoResponse
	if err := json.Unmarshal([]byte(sample), &parsed); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	results := parsed.extractResults()
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if !strings.Contains(results[0].Snippet, "connection refused") {
		t.Fatalf("expected abstract in first result, got %q", results[0].Snippet)
	}
	if !strings.Contains(results[1].Title, "Connection Refused") {
		t.Fatalf("expected topic title in second result, got %q", results[1].Title)
	}
}

func TestWebSearchToolRequiresQuery(t *testing.T) {
	tool := NewWebSearchTool()
	result, err := tool.Invoke(context.Background(), ragtool.Call{
		Name:      "web_search",
		Arguments: map[string]any{"query": ""},
	})
	if err != nil {
		t.Fatalf("invoke without query: %v", err)
	}
	if result.Status != ragtool.CallStatusFailed {
		t.Fatalf("expected failed status without query, got %q", result.Status)
	}
}
