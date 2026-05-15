package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
)

type toolWorkflowStub struct {
	result ragtool.WorkflowResult
	err    error
	input  ragtool.WorkflowInput
}

func (s *toolWorkflowStub) Run(ctx context.Context, input ragtool.WorkflowInput) (ragtool.WorkflowResult, error) {
	s.input = input
	return s.result, s.err
}

type fallbackSinkStub struct {
	metaCalls       int
	agentThinkCalls int
	fallbackCalls   int
	fallbackReason  string
	finishCalls     int
	errorCalls      int
	doneCalls       int
	toolCalls       int
	toolNames       []string
}

func (s *fallbackSinkStub) SendMeta(meta RagChatMeta) error {
	s.metaCalls++
	return nil
}

func (s *fallbackSinkStub) SendFallback(reason string) error {
	s.fallbackCalls++
	s.fallbackReason = reason
	return nil
}

func (s *fallbackSinkStub) SendAgentThink(message string) error {
	s.agentThinkCalls++
	return nil
}

func (s *fallbackSinkStub) SendThinking(delta string) error { return nil }
func (s *fallbackSinkStub) SendMessage(delta string) error  { return nil }
func (s *fallbackSinkStub) SendToolStart(payload ragtool.ToolCallEvent) error {
	return nil
}
func (s *fallbackSinkStub) SendToolResult(payload ragtool.ToolCallEvent) error {
	return nil
}
func (s *fallbackSinkStub) SendTitle(title string) error { return nil }
func (s *fallbackSinkStub) SendTool(name string, status string, summary string) error {
	s.toolCalls++
	s.toolNames = append(s.toolNames, name)
	return nil
}

func (s *fallbackSinkStub) SendFinish(payload RagChatFinishPayload) error {
	s.finishCalls++
	return nil
}

func (s *fallbackSinkStub) SendCancel(payload RagChatFinishPayload) error { return nil }

func (s *fallbackSinkStub) SendError(err error) error {
	s.errorCalls++
	return nil
}

func (s *fallbackSinkStub) SendDone() error {
	s.doneCalls++
	return nil
}

type traceNodeRepoRecorder struct {
	created []domain.RagTraceNode
}

func (r *traceNodeRepoRecorder) Create(_ context.Context, node domain.RagTraceNode) (domain.RagTraceNode, error) {
	r.created = append(r.created, node)
	return node, nil
}

func (r *traceNodeRepoRecorder) UpdateByTraceIDAndNodeID(context.Context, string, string, domain.RagTraceNode) error {
	return nil
}

func (r *traceNodeRepoRecorder) UpdateWhere(context.Context, port.RagTraceNodeConditions, port.RagTraceNodePatch) (int64, error) {
	return 0, nil
}

func (r *traceNodeRepoRecorder) ListByTraceID(context.Context, string) ([]domain.RagTraceNode, error) {
	return nil, nil
}

func TestTopChunkScore(t *testing.T) {
	if got := topChunkScore(ragretrieve.Result{}); got != 0 {
		t.Fatalf("empty result: expected 0, got %v", got)
	}

	result := ragretrieve.Result{
		Chunks: []convention.RetrievedChunk{
			{ID: "c1", Score: 0.85},
		},
	}
	if got := topChunkScore(result); got != 0.85 {
		t.Fatalf("single chunk: expected 0.85, got %v", got)
	}

	result = ragretrieve.Result{
		Chunks: []convention.RetrievedChunk{
			{ID: "c1", Score: 0.45},
			{ID: "c2", Score: 0.92},
			{ID: "c3", Score: 0.67},
		},
	}
	if got := topChunkScore(result); got != 0.92 {
		t.Fatalf("multi chunk: expected 0.92, got %v", got)
	}
}

func TestBuildFallbackPrompt(t *testing.T) {
	question := "what is the weather today"
	prompt := buildFallbackPrompt(question)

	if !strings.Contains(prompt, question) {
		t.Fatalf("expected question %q in fallback prompt, got: %s", question, prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "general model") {
		t.Fatalf("expected general-model fallback warning in fallback prompt, got: %s", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "respond in chinese") {
		t.Fatalf("expected Chinese-response hint in fallback prompt, got: %s", prompt)
	}
}

func TestRagChatServiceConfidenceThresholdDefaultsOff(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if svc.confidenceThreshold != 0 {
		t.Fatalf("expected confidenceThreshold=0 by default, got %v", svc.confidenceThreshold)
	}
}

func TestRagChatServiceSetConfidenceThreshold(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetConfidenceThreshold(0.6)
	if svc.confidenceThreshold != 0.6 {
		t.Fatalf("expected 0.6, got %v", svc.confidenceThreshold)
	}

	svc.SetConfidenceThreshold(0)
	if svc.confidenceThreshold != 0 {
		t.Fatalf("expected 0 after disabling, got %v", svc.confidenceThreshold)
	}
}

func TestRagChatServiceSetToolWorkflow(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	workflow := &toolWorkflowStub{}
	svc.SetToolWorkflow(workflow)
	if svc.toolWorkflow != workflow {
		t.Fatal("expected tool workflow to be assigned")
	}
}

func TestRagChatServiceValidateDependencies(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.validateDependencies(); err == nil {
		t.Fatal("expected validation error for nil dependencies")
	}
}

func TestRagChatServiceNilSink(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	if err := svc.Chat(nil, RagChatInput{Question: "test", UserID: "u1"}, nil); err == nil {
		t.Fatal("expected error for nil sink")
	}
}

func TestRagChatServiceEmptyQuestion(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	sink := &fallbackSinkStub{}
	if err := svc.Chat(nil, RagChatInput{Question: "", UserID: "u1"}, sink); err == nil {
		t.Fatal("expected error for empty question or missing dependencies")
	}
}

func TestRagChatServiceEmptyUserID(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	sink := &fallbackSinkStub{}
	if err := svc.Chat(nil, RagChatInput{Question: "hello", UserID: ""}, sink); err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestShouldRunRetrieve(t *testing.T) {
	if shouldRunRetrieve(RagChatInput{}, ragrewrite.Result{NeedRetrieval: true}) {
		t.Fatal("expected no retrieve when knowledge base ids are empty")
	}
	if shouldRunRetrieve(RagChatInput{KnowledgeBaseIDs: []string{"kb-1"}}, ragrewrite.Result{NeedRetrieval: false}) {
		t.Fatal("expected no retrieve when rewrite says retrieval is unnecessary")
	}
	if !shouldRunRetrieve(RagChatInput{KnowledgeBaseIDs: []string{"kb-1"}}, ragrewrite.Result{NeedRetrieval: true}) {
		t.Fatal("expected retrieve when knowledge base exists and rewrite requires it")
	}
}

func TestRunToolWorkflowStageSkipsWhenWorkflowUnset(t *testing.T) {
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	result, err := svc.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "q", UserID: "u"},
		nil,
		ragrewrite.Result{},
		ragretrieve.Result{},
		false,
		"trace-1",
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.result.Used {
		t.Fatal("expected empty workflow result when workflow is unset")
	}
}

func TestRunToolWorkflowStageReturnsWorkflowResult(t *testing.T) {
	workflow := &toolWorkflowStub{
		result: ragtool.WorkflowResult{
			Used:    true,
			Context: "tool context",
			Calls: []ragtool.CallSummary{
				{Name: "document_query", Status: ragtool.CallStatusSuccess, Summary: "matched doc-1"},
			},
		},
	}
	svc := NewRagChatService(nil, nil, nil, nil, nil, nil, nil, nil)
	svc.SetToolWorkflow(workflow)

	history := []convention.ChatMessage{convention.UserMessage("previous")}
	rewriteResult := ragrewrite.Result{RewrittenQuestion: "rewritten"}
	retrieveResult := ragretrieve.Result{KnowledgeContext: "knowledge"}
	result, err := svc.runToolWorkflowStage(
		context.Background(),
		RagChatInput{
			ConversationID:   "conv-1",
			UserID:           "user-1",
			Question:         "why failed",
			KnowledgeBaseIDs: []string{"kb-1"},
		},
		history,
		rewriteResult,
		retrieveResult,
		true,
		"trace-1",
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.result.Used {
		t.Fatal("expected workflow result to be used")
	}
	if result.result.Context != "tool context" {
		t.Fatalf("unexpected tool context: %q", result.result.Context)
	}
	if workflow.input.TraceID != "trace-1" {
		t.Fatalf("unexpected trace id: %q", workflow.input.TraceID)
	}
	if workflow.input.Control.ExecutionMode != ragtool.ExecutionModeReadOnly {
		t.Fatalf("unexpected workflow execution mode: %q", workflow.input.Control.ExecutionMode)
	}
	if workflow.input.Control.RiskLevel != ragtool.RiskLevelLow {
		t.Fatalf("unexpected workflow risk level: %q", workflow.input.Control.RiskLevel)
	}
	if workflow.input.Control.ApprovalRequirement != ragtool.ApprovalRequirementNone {
		t.Fatalf("unexpected workflow approval requirement: %q", workflow.input.Control.ApprovalRequirement)
	}
	if len(workflow.input.History) != 1 || workflow.input.History[0].Content != "previous" {
		t.Fatalf("unexpected history: %+v", workflow.input.History)
	}
}

func TestRecordToolCallTraceNodes(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tracer := NewChatTracer(nil, repo)
	_ = NewRagChatService(nil, nil, nil, nil, nil, nil, nil, tracer)
	tracer.now = func() time.Time { return now }

	tracer.recordToolCallTraceNodes(context.Background(), "trace-1", []ragtool.CallSummary{
		{Name: "document_query", Status: ragtool.CallStatusSuccess, Summary: "matched doc-1", DurationMs: 12},
		{Name: "task_ingestion_diagnose", Status: ragtool.CallStatusFailed, Summary: "task not found", DurationMs: 34},
	})

	if len(repo.created) != 2 {
		t.Fatalf("expected 2 tool call trace nodes, got %d", len(repo.created))
	}
	if repo.created[0].ParentNodeID != "tool_workflow" || repo.created[0].Depth != 2 {
		t.Fatalf("unexpected parent/depth: %+v", repo.created[0])
	}
	if repo.created[0].NodeID != "tool_01" || repo.created[1].NodeID != "tool_02" {
		t.Fatalf("unexpected tool node ids: %+v", repo.created)
	}
	if repo.created[0].NodeName != "document_query" || repo.created[1].NodeName != "task_ingestion_diagnose" {
		t.Fatalf("unexpected node names: %+v", repo.created)
	}
	if repo.created[1].ErrorMessage != "task not found" {
		t.Fatalf("expected failed tool error message to be persisted, got %q", repo.created[1].ErrorMessage)
	}
	if repo.created[0].DurationMs == nil || *repo.created[0].DurationMs != 12 {
		t.Fatalf("unexpected first duration: %+v", repo.created[0].DurationMs)
	}
	if repo.created[1].StartTime == nil || repo.created[1].EndTime == nil || !repo.created[1].EndTime.After(*repo.created[1].StartTime) {
		t.Fatalf("expected second node to have increasing timestamps: %+v", repo.created[1])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal extra data: %v", err)
	}
	if payload["summary"] != "matched doc-1" {
		t.Fatalf("unexpected summary payload: %+v", payload)
	}
}

func TestRecordAgentWorkflowTraceNodesUsesDatabaseSafeNames(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	now := time.Date(2026, 5, 10, 14, 48, 0, 0, time.UTC)
	tracer := NewChatTracer(nil, repo)
	tracer.now = func() time.Time { return now }

	tracer.recordAgentWorkflowTraceNodes(context.Background(), "trace-1", ragtool.WorkflowResult{
		TraceMeta: ragtool.WorkflowTraceMeta{
			Capability:          ragtool.CapabilitySearch,
			ExecutionMode:       ragtool.ExecutionModeReadOnly,
			RiskLevel:           ragtool.RiskLevelLow,
			ApprovalRequirement: ragtool.ApprovalRequirementNone,
			EvidenceSources:     []string{ragtool.EvidenceSourceKnowledgeBase, ragtool.EvidenceSourceExternalWeb},
		},
		Rounds: []ragtool.RoundSummary{
			{
				Round:               1,
				Done:                true,
				Reasoning:           "enough evidence",
				ExecutionMode:       "parallel",
				WallClockDurationMs: 10,
				ToolCallCount:       1,
				TotalToolDurationMs: 12,
				Calls: []ragtool.CallSummary{
					{
						CallID:     "round_1_call_01",
						Round:      1,
						Sequence:   1,
						Name:       "document_ingestion_diagnose",
						Status:     ragtool.CallStatusSuccess,
						Summary:    "doc failed",
						DurationMs: 12,
					},
				},
			},
		},
	})

	if len(repo.created) != 3 {
		t.Fatalf("expected 3 trace nodes, got %d", len(repo.created))
	}
	if repo.created[0].NodeID != "agt_round_01" || repo.created[0].NodeType != "agt_round" {
		t.Fatalf("unexpected round node: %+v", repo.created[0])
	}
	if repo.created[2].NodeID != "agt_obs_01" || repo.created[2].NodeType != "agt_obs" {
		t.Fatalf("unexpected observation node: %+v", repo.created[2])
	}
	if len(repo.created[0].NodeType) > 16 || len(repo.created[2].NodeType) > 16 {
		t.Fatal("expected trace node types to stay within varchar(16)")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("unmarshal round extra data: %v", err)
	}
	if payload["executionMode"] != "parallel" {
		t.Fatalf("expected executionMode=parallel, got %+v", payload)
	}
	if payload["capability"] != ragtool.CapabilitySearch {
		t.Fatalf("expected capability=search, got %+v", payload)
	}
	if payload["workflowMode"] != ragtool.ExecutionModeReadOnly {
		t.Fatalf("expected workflowMode=read_only, got %+v", payload)
	}
	if payload["wallClockDurationMs"] != float64(10) {
		t.Fatalf("expected wallClockDurationMs=10, got %+v", payload)
	}
}
