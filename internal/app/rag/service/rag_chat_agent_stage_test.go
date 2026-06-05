package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentapp "local/rag-project/internal/app/agent"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	ragtool "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
)

type agentRuntimeServiceStub struct {
	runDetailedFn         func(context.Context, agentapp.Request) (agentapp.RunResponse, error)
	resumeAfterApprovalFn func(context.Context, agentapp.ResumeApprovalRequest) (agentapp.RunResponse, error)
}

func (s agentRuntimeServiceStub) RunDetailed(ctx context.Context, req agentapp.Request) (agentapp.RunResponse, error) {
	if s.runDetailedFn != nil {
		return s.runDetailedFn(ctx, req)
	}
	return agentapp.RunResponse{}, nil
}

func (s agentRuntimeServiceStub) ResumeAfterApproval(ctx context.Context, req agentapp.ResumeApprovalRequest) (agentapp.RunResponse, error) {
	if s.resumeAfterApprovalFn != nil {
		return s.resumeAfterApprovalFn(ctx, req)
	}
	return agentapp.RunResponse{}, nil
}

func TestRagChatServiceAgentRuntimeChatAwaitingApproval(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(_ context.Context, req agentapp.Request) (agentapp.RunResponse, error) {
			if !req.Options.RequireApproval {
				t.Fatalf("expected require approval to be forwarded")
			}
			return agentapp.RunResponse{
				Outcome: agentapp.RunOutcome{
					Status:       agentapp.RunStatusAwaitingApproval,
					CheckpointID: "cp-1",
					Approval: &agentapp.ApprovalPending{
						Required:       true,
						Status:         "pending",
						CheckpointID:   "cp-1",
						CapabilityName: "web_fetch",
						CanApprove:     true,
						CanReject:      true,
					},
				},
			}, nil
		},
	})

	sink := &fallbackSinkStub{}
	err := service.Chat(context.Background(), RagChatInput{
		Question:        "fetch example.com",
		UserID:          "user-1",
		UseAgentRuntime: true,
		RequireApproval: true,
	}, sink)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(sink.agentOutcomes) != 1 || sink.agentOutcomes[0].Status != agentapp.RunStatusAwaitingApproval {
		t.Fatalf("expected awaiting approval outcome, got %+v", sink.agentOutcomes)
	}
	if len(sink.approvalPending) != 1 || sink.approvalPending[0].CheckpointID != "cp-1" {
		t.Fatalf("expected approval payload, got %+v", sink.approvalPending)
	}
	if sink.finishCalls != 0 {
		t.Fatalf("expected no finish event, got %d", sink.finishCalls)
	}
	if sink.doneCalls != 1 {
		t.Fatalf("expected done once, got %d", sink.doneCalls)
	}
}

func TestRagChatServiceSelectsAgentRuntimeInToolStageDiagnosticMode(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: false}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(_ context.Context, req agentapp.Request) (agentapp.RunResponse, error) {
			if req.Question != "doc_fail_01 why failed" {
				t.Fatalf("unexpected question: %q", req.Question)
			}
			return agentapp.RunResponse{
				Response: agentapp.Response{
					Query:   "doc_fail_01 why failed",
					Summary: "agent diagnosis summary",
				},
				Outcome: agentapp.RunOutcome{
					Status: agentapp.RunStatusCompleted,
				},
			}, nil
		},
	})
	service.SetAgentRuntimeMode(ragChatAgentModeDiagnostic)

	result, err := service.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "doc_fail_01 why failed", UserID: "u1"},
		nil,
		"",
		"",
		ragrewrite.Result{NeedRetrieval: false},
		ragretrieve.Result{},
		false,
		"trace-1",
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage() error = %v", err)
	}
	if result.backend != "agent_runtime" {
		t.Fatalf("expected agent backend, got %+v", result)
	}
	if result.agentRun == nil || result.agentRun.Outcome.Status != agentapp.RunStatusCompleted {
		t.Fatalf("expected completed agent run, got %+v", result.agentRun)
	}
	if result.result.Context == "" || result.result.AnswerGuidance == "" {
		t.Fatalf("expected workflow result to carry agent prompt data, got %+v", result.result)
	}
}

func TestRagChatServiceRunAgentToolWorkflowStage_ProjectsToolStageContext(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(_ context.Context, req agentapp.Request) (agentapp.RunResponse, error) {
			if req.ToolStage == nil {
				t.Fatal("expected tool-stage context to be projected")
			}
			if req.ToolStage.ConversationID != "conv-1" {
				t.Fatalf("unexpected conversation id: %+v", req.ToolStage)
			}
			if req.ToolStage.RewrittenQuestion != "doc_fail_01 why failed rewritten" {
				t.Fatalf("unexpected rewritten question: %+v", req.ToolStage)
			}
			if len(req.ToolStage.KnowledgeBaseIDs) != 1 || req.ToolStage.KnowledgeBaseIDs[0] != "kb-1" {
				t.Fatalf("unexpected knowledge base ids: %+v", req.ToolStage)
			}
			if req.ToolStage.KnowledgeContext != "retrieved context" {
				t.Fatalf("unexpected knowledge context: %+v", req.ToolStage)
			}
			if len(req.ToolStage.SearchChannels) != 2 {
				t.Fatalf("unexpected search channels: %+v", req.ToolStage)
			}
			if req.ToolStage.HistorySummary == "" {
				t.Fatalf("expected history summary, got %+v", req.ToolStage)
			}
			if req.ToolStage.MemoryContext != "memory context for agent bridge" {
				t.Fatalf("unexpected memory context: %+v", req.ToolStage)
			}
			if req.ToolStage.SessionContext != "session context for agent bridge" {
				t.Fatalf("unexpected session context: %+v", req.ToolStage)
			}
			return agentapp.RunResponse{
				Response: agentapp.Response{
					Query:   req.ToolStage.RewrittenQuestion,
					Summary: "agent diagnosis summary",
				},
				Outcome: agentapp.RunOutcome{
					Status: agentapp.RunStatusCompleted,
				},
				Journal: []agentstate.RuntimeEvent{
					agentstate.NewRuntimeEvent("trace-1", "search", agentstate.EventTypeCapabilityStart, "search web for rewritten question"),
					agentstate.NewRuntimeEvent("trace-1", "search", agentstate.EventTypeCapabilityResult, "found 3 web results"),
					agentstate.NewRuntimeEvent("trace-1", "fetch", agentstate.EventTypeCapabilityStart, "fetch top urls"),
					agentstate.NewRuntimeEvent("trace-1", "fetch", agentstate.EventTypeCapabilityResult, "fetched 2 pages"),
				},
			}, nil
		},
	})
	service.SetAgentRuntimeMode(ragChatAgentModeAlways)

	sink := &fallbackSinkStub{}
	result, err := service.runToolWorkflowStage(
		context.Background(),
		RagChatInput{
			ConversationID:   "conv-1",
			UserID:           "u1",
			Question:         "doc_fail_01 why failed",
			KnowledgeBaseIDs: []string{"kb-1"},
		},
		[]convention.ChatMessage{
			convention.UserMessage("previous diagnostic question"),
			convention.AssistantMessage("previous diagnostic answer"),
		},
		"memory context for agent bridge",
		"session context for agent bridge",
		ragrewrite.Result{
			RewrittenQuestion: "doc_fail_01 why failed rewritten",
			SubQuestions:      []string{"indexer failed?", "vector store health"},
			NeedRetrieval:     true,
		},
		ragretrieve.Result{
			KnowledgeContext: "retrieved context",
			SearchChannels:   []string{"vector_global", "keyword"},
		},
		false,
		"trace-1",
		sink,
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage() error = %v", err)
	}
	if result.backend != "agent_runtime" || result.agentRun == nil {
		t.Fatalf("expected agent runtime backend, got %+v", result)
	}
	if len(result.result.Calls) != 2 {
		t.Fatalf("expected projected tool calls, got %+v", result.result.Calls)
	}
	if result.result.Calls[0].Name != "查询中" || result.result.Calls[1].Name != "拉取中" {
		t.Fatalf("expected human-readable tool names, got %+v", result.result.Calls)
	}
	if len(sink.toolStarts) != 2 || len(sink.toolResults) != 2 {
		t.Fatalf("expected projected tool SSE events, got starts=%+v results=%+v", sink.toolStarts, sink.toolResults)
	}
	if sink.toolStarts[0].Name != "查询中" || sink.toolResults[1].Name != "拉取中" {
		t.Fatalf("expected projected tool event names, got starts=%+v results=%+v", sink.toolStarts, sink.toolResults)
	}
}

func TestRagChatServiceFallsBackToLegacyToolWorkflowWhenAgentToolStageFails(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: false}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(context.Context, agentapp.Request) (agentapp.RunResponse, error) {
			return agentapp.RunResponse{}, &agentapp.ServiceError{
				Code:      "agent_backend_unavailable",
				Message:   "agent backend unavailable",
				Kind:      agentapp.ErrorKindUnavailable,
				Retryable: true,
			}
		},
	})
	service.SetAgentRuntimeMode(ragChatAgentModeAlways)
	workflow := &toolWorkflowStub{
		result: ragtool.WorkflowResult{
			Used:           true,
			Context:        "legacy tool context",
			AnswerGuidance: "legacy answer guidance",
		},
	}
	service.SetToolWorkflow(workflow)

	sink := &fallbackSinkStub{}
	result, err := service.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "doc_fail_01 why failed", UserID: "u1"},
		nil,
		"",
		"",
		ragrewrite.Result{NeedRetrieval: false},
		ragretrieve.Result{},
		false,
		"trace-1",
		sink,
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage() error = %v", err)
	}
	if result.backend != "tool_workflow" {
		t.Fatalf("expected legacy backend after fallback, got %+v", result)
	}
	if result.fallbackFrom != "agent_runtime" {
		t.Fatalf("expected fallbackFrom=agent_runtime, got %+v", result)
	}
	if result.agentError == nil || result.agentError.Kind != agentapp.ErrorKindUnavailable {
		t.Fatalf("expected projected agent error to be retained, got %+v", result.agentError)
	}
	if result.result.Context != "legacy tool context" || result.result.AnswerGuidance != "legacy answer guidance" {
		t.Fatalf("expected legacy workflow result to be returned, got %+v", result.result)
	}
	if workflow.input.Question != "doc_fail_01 why failed" {
		t.Fatalf("expected legacy workflow to run during fallback, got %+v", workflow.input)
	}
	if len(sink.agentErrors) != 0 {
		t.Fatalf("expected no outward agent service error when fallback succeeds, got %+v", sink.agentErrors)
	}
}

func TestRagChatServiceEmitsAgentServiceErrorWhenAgentToolStageFailsWithoutLegacyFallback(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: false}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(context.Context, agentapp.Request) (agentapp.RunResponse, error) {
			return agentapp.RunResponse{}, errors.New("agent backend unavailable")
		},
	})
	service.SetAgentRuntimeMode(ragChatAgentModeAlways)

	sink := &fallbackSinkStub{}
	result, err := service.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "doc_fail_01 why failed", UserID: "u1"},
		nil,
		"",
		"",
		ragrewrite.Result{NeedRetrieval: false},
		ragretrieve.Result{},
		false,
		"trace-1",
		sink,
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage() error = %v", err)
	}
	if result.backend != "agent_runtime" {
		t.Fatalf("expected agent backend without legacy fallback, got %+v", result)
	}
	if result.agentError == nil || result.agentError.Kind != agentapp.ErrorKindInternal {
		t.Fatalf("expected internal projected agent error, got %+v", result.agentError)
	}
	if !result.result.Degraded {
		t.Fatalf("expected degraded workflow result, got %+v", result.result)
	}
	if len(sink.agentErrors) != 1 {
		t.Fatalf("expected one outward agent service error, got %+v", sink.agentErrors)
	}
	if sink.agentErrors[0].Kind != agentapp.ErrorKindInternal || sink.agentErrors[0].Message != "agent backend unavailable" {
		t.Fatalf("unexpected outward agent service error: %+v", sink.agentErrors[0])
	}
}

func TestWorkflowResultFromAgentRunRequestsExternalSourceDisclosure(t *testing.T) {
	result := workflowResultFromAgentRun(agentapp.RunResponse{
		Response: agentapp.Response{
			Query:   "Golang context",
			Summary: "context controls cancellation and deadlines",
			Results: []agentsearch.SearchResultItem{
				{
					Title: "Go Concurrency Patterns: Context",
					URL:   "https://go.dev/blog/context",
				},
			},
			Pages: []agentfetch.PageResult{
				{
					URL:  "https://pkg.go.dev/context",
					Text: "Package context defines the Context type...",
				},
			},
		},
		Outcome: agentapp.RunOutcome{
			Status: agentapp.RunStatusCompleted,
		},
	})

	if !strings.Contains(result.AnswerGuidance, "`来源` section") {
		t.Fatalf("expected source disclosure instruction, got %q", result.AnswerGuidance)
	}
	if !strings.Contains(result.Context, "Agent sources:") {
		t.Fatalf("expected source context, got %q", result.Context)
	}
	if !strings.Contains(result.Context, "https://go.dev/blog/context") || !strings.Contains(result.Context, "https://pkg.go.dev/context") {
		t.Fatalf("expected source urls in tool context, got %q", result.Context)
	}
}

func TestRagChatServiceDoesNotSelectAgentRuntimeWhenModeOff(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: false}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(context.Context, agentapp.Request) (agentapp.RunResponse, error) {
			t.Fatal("agent runtime should not be called when mode is off")
			return agentapp.RunResponse{}, nil
		},
	})
	workflow := &toolWorkflowStub{
		result: ragtool.WorkflowResult{Used: true, Context: "legacy tool context"},
	}
	service.SetToolWorkflow(workflow)
	service.SetAgentRuntimeMode(ragChatAgentModeOff)

	result, err := service.runToolWorkflowStage(
		context.Background(),
		RagChatInput{Question: "doc_fail_01 why failed", UserID: "u1"},
		nil,
		"",
		"",
		ragrewrite.Result{NeedRetrieval: false},
		ragretrieve.Result{},
		false,
		"trace-1",
		&fallbackSinkStub{},
	)
	if err != nil {
		t.Fatalf("runToolWorkflowStage() error = %v", err)
	}
	if result.backend != "tool_workflow" {
		t.Fatalf("expected legacy tool backend, got %+v", result)
	}
	if workflow.input.Question != "doc_fail_01 why failed" {
		t.Fatalf("expected legacy workflow to run, got %+v", workflow.input)
	}
}

func TestRagChatServiceResumeAfterApprovalCompletes(t *testing.T) {
	service, createdMessage := newPrepareChatTestService(t, ragrewrite.Result{}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		resumeAfterApprovalFn: func(_ context.Context, req agentapp.ResumeApprovalRequest) (agentapp.RunResponse, error) {
			if req.CheckpointID != "cp-2" {
				t.Fatalf("unexpected checkpoint id: %q", req.CheckpointID)
			}
			if req.Decision != agentapp.ApprovalDecisionApproved {
				t.Fatalf("unexpected decision: %q", req.Decision)
			}
			return agentapp.RunResponse{
				Response: agentapp.Response{
					Summary: "approval accepted summary",
				},
				Outcome: agentapp.RunOutcome{
					Status: agentapp.RunStatusCompleted,
				},
			}, nil
		},
	})

	sink := &fallbackSinkStub{}
	err := service.ResumeAfterApproval(context.Background(), RagChatApprovalResumeInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
		Question:       "why did it fail",
		CheckpointID:   "cp-2",
		Decision:       agentapp.ApprovalDecisionApproved,
	}, sink)
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}
	if len(sink.agentOutcomes) != 1 || sink.agentOutcomes[0].Status != agentapp.RunStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", sink.agentOutcomes)
	}
	if sink.finishCalls != 1 || sink.doneCalls != 1 {
		t.Fatalf("expected finish and done once, got finish=%d done=%d", sink.finishCalls, sink.doneCalls)
	}
	if createdMessage.Content != "approval accepted summary" {
		t.Fatalf("expected assistant message content to be persisted, got %+v", createdMessage)
	}
}

func TestRagChatServiceAgentRuntimeChatProjectsServiceError(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{}, nil, nil)
	service.SetAgentRuntimeService(agentRuntimeServiceStub{
		runDetailedFn: func(context.Context, agentapp.Request) (agentapp.RunResponse, error) {
			return agentapp.RunResponse{}, &agentapp.ServiceError{
				Code:      agentapp.ErrorCodeApprovalSessionNotFound,
				Message:   "approval session not found",
				Kind:      agentapp.ErrorKindNotFound,
				Retryable: false,
			}
		},
	})

	sink := &fallbackSinkStub{}
	err := service.Chat(context.Background(), RagChatInput{
		Question:        "resume this",
		UserID:          "user-1",
		UseAgentRuntime: true,
	}, sink)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(sink.agentErrors) != 1 {
		t.Fatalf("expected one agent service error event, got %+v", sink.agentErrors)
	}
	if sink.agentErrors[0].Code != agentapp.ErrorCodeApprovalSessionNotFound || sink.agentErrors[0].Kind != agentapp.ErrorKindNotFound {
		t.Fatalf("unexpected projected agent error: %+v", sink.agentErrors[0])
	}
	if sink.doneCalls != 1 || sink.errorCalls != 1 {
		t.Fatalf("expected error and done once, got error=%d done=%d", sink.errorCalls, sink.doneCalls)
	}
}
