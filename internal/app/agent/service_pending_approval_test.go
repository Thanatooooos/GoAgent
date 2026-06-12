package agent

import (
	"context"
	"strings"
	"testing"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	searchprovider "local/rag-project/internal/app/agent/search/provider"
	agentstate "local/rag-project/internal/app/agent/state"
)

func TestServiceGetPendingApprovalTracksConversationLookup(t *testing.T) {
	searchService := agentsearch.NewService(stubRuntimeProvider{
		search: func(query string) ([]searchprovider.SearchResult, error) {
			return []searchprovider.SearchResult{
				{
					Title:   "Restricted",
					URL:     "https://restricted.example/doc",
					Snippet: "needs approval",
					Domain:  "restricted.example",
				},
			}, nil
		},
	}, nil)
	searchHandle, err := agentsearch.NewCapability(searchService)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}

	attempt := 0
	fetchHandle, err := agentfetch.NewCapability(stubFetchFlow{
		fetch: func(_ context.Context, urls []string) (agentfetch.Output, error) {
			attempt++
			if attempt == 1 {
				return agentfetch.Output{
					Summary:       "fetch requires approval",
					Degraded:      true,
					DegradeReason: "provider requires approval",
					ErrorMessage:  "permission denied by upstream provider",
					Pages: []agentfetch.PageResult{
						{URL: urls[0], ErrorMessage: "403 forbidden"},
					},
				}, nil
			}
			return agentfetch.Output{
				Summary: "fetched approved content",
				Pages: []agentfetch.PageResult{
					{URL: urls[0], Text: "approved readable evidence"},
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}

	service := newTestAgentService(t, searchHandle, fetchHandle)
	initial, err := service.RunDetailed(context.Background(), Request{
		Question: "runtime approval flow",
		UserID:   "user-1",
		ToolStage: &ToolStageContext{
			ConversationID: "conv-1",
		},
		Options: RequestOptions{
			RequireApproval: true,
			OutputMode:      agentstate.OutputModeFinalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunDetailed() error = %v", err)
	}
	if initial.Outcome.Status != RunStatusAwaitingApproval || initial.Outcome.Approval == nil {
		t.Fatalf("expected awaiting approval outcome, got %+v", initial.Outcome)
	}

	pending, ok, err := service.GetPendingApproval(context.Background(), PendingApprovalLookupRequest{
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("GetPendingApproval() error = %v", err)
	}
	if !ok || pending == nil {
		t.Fatalf("expected pending approval lookup to resolve, got ok=%v pending=%+v", ok, pending)
	}
	if pending.CheckpointID != initial.Outcome.CheckpointID || pending.Question != "runtime approval flow" {
		t.Fatalf("unexpected pending approval payload: %+v", pending)
	}

	_, err = service.ResumeAfterApproval(context.Background(), ResumeApprovalRequest{
		CheckpointID: initial.Outcome.CheckpointID,
		Decision:     ApprovalDecisionApproved,
		DecisionNote: "approved for retry",
	})
	if err != nil {
		t.Fatalf("ResumeAfterApproval() error = %v", err)
	}

	pending, ok, err = service.GetPendingApproval(context.Background(), PendingApprovalLookupRequest{
		ConversationID: "conv-1",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("GetPendingApproval(after resume) error = %v", err)
	}
	if ok || pending != nil {
		t.Fatalf("expected pending approval lookup to clear after resume, got ok=%v pending=%+v", ok, pending)
	}
}

func TestServiceGetPendingApprovalReturnsNotFoundForUnknownConversation(t *testing.T) {
	service := newTestAgentService(t, mustSearchHandle(t), mustFetchHandle(t))

	pending, ok, err := service.GetPendingApproval(context.Background(), PendingApprovalLookupRequest{
		ConversationID: "conv-missing",
		UserID:         "user-1",
	})
	if err != nil {
		t.Fatalf("GetPendingApproval() error = %v", err)
	}
	if ok || pending != nil {
		t.Fatalf("expected no pending approval, got ok=%v pending=%+v", ok, pending)
	}
	if strings.TrimSpace(service.runtimeName) == "" {
		t.Fatalf("expected helper service to stay initialized")
	}
}
