package service

import (
	"context"
	"testing"
	"time"

	agentapp "local/rag-project/internal/app/agent"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
)

func TestRagChatServiceGetPendingApprovalProjectsPayload(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{}, nil, nil, func(deps *RagChatDeps, _ *RagChatOptions) {
		deps.AgentRuntime = agentRuntimeServiceStub{
			getPendingApprovalFn: func(_ context.Context, req agentapp.PendingApprovalLookupRequest) (*agentapp.ApprovalPending, bool, error) {
				if req.ConversationID != "conv-1" || req.UserID != "user-1" {
					t.Fatalf("unexpected lookup request: %+v", req)
				}
				return &agentapp.ApprovalPending{
					Required:     true,
					Status:       "pending",
					CheckpointID: "cp-1",
					Question:     "why did it pause",
					RequestedAt:  time.Unix(1700000000, 0),
				}, true, nil
			},
		}
	})

	pending, err := service.GetPendingApproval(context.Background(), RagChatApprovalPendingQueryInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("GetPendingApproval() error = %v", err)
	}
	if pending == nil || pending.CheckpointID != "cp-1" || pending.Question != "why did it pause" {
		t.Fatalf("unexpected pending payload: %+v", pending)
	}
}

func TestRagChatServiceGetPendingApprovalReturnsNilWhenNotFound(t *testing.T) {
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{}, nil, nil, func(deps *RagChatDeps, _ *RagChatOptions) {
		deps.AgentRuntime = agentRuntimeServiceStub{}
	})

	pending, err := service.GetPendingApproval(context.Background(), RagChatApprovalPendingQueryInput{
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("GetPendingApproval() error = %v", err)
	}
	if pending != nil {
		t.Fatalf("expected nil pending payload, got %+v", pending)
	}
}
