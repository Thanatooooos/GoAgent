package rag_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	raghttp "local/rag-project/internal/adapter/http/rag"
	agentapp "local/rag-project/internal/app/agent"
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/framework/contextx"
)

type chatServiceStub struct {
	chatFn   func(context.Context, ragservice.RagChatInput, ragservice.RagChatEventSink) error
	resumeFn func(context.Context, ragservice.RagChatApprovalResumeInput, ragservice.RagChatEventSink) error
}

func (s chatServiceStub) Chat(ctx context.Context, input ragservice.RagChatInput, sink ragservice.RagChatEventSink) error {
	if s.chatFn != nil {
		return s.chatFn(ctx, input, sink)
	}
	return nil
}

func (s chatServiceStub) ResumeAfterApproval(ctx context.Context, input ragservice.RagChatApprovalResumeInput, sink ragservice.RagChatEventSink) error {
	if s.resumeFn != nil {
		return s.resumeFn(ctx, input, sink)
	}
	return nil
}

func (s chatServiceStub) CancelTask(string) bool {
	return true
}

func TestChatHandlerForwardsChatInputAndStreamsOutcome(t *testing.T) {
	var captured ragservice.RagChatInput
	router := newChatRouter(chatServiceStub{
		chatFn: func(_ context.Context, input ragservice.RagChatInput, sink ragservice.RagChatEventSink) error {
			captured = input
			if err := sink.SendAgentOutcome(ragservice.RagChatAgentOutcomePayload{Status: agentapp.RunStatusAwaitingApproval, CheckpointID: "cp-10"}); err != nil {
				return err
			}
			return sink.SendDone()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/chat?question=hello&requireApproval=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if captured.UseAgentRuntime {
		t.Fatalf("expected handler not to force agent runtime path, got %+v", captured)
	}
	if !captured.RequireApproval {
		t.Fatalf("expected require approval flag to be forwarded, got %+v", captured)
	}
	if !strings.Contains(rec.Body.String(), "event: agent_outcome") ||
		!strings.Contains(rec.Body.String(), "event: agent_status") ||
		!strings.Contains(rec.Body.String(), "\"type\":\"outcome\"") ||
		!strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("expected SSE outcome and done events, got %s", rec.Body.String())
	}
}

func TestChatHandlerResumeAfterApprovalBindsRequestAndStreamsApprovalPayload(t *testing.T) {
	var captured ragservice.RagChatApprovalResumeInput
	router := newChatRouter(chatServiceStub{
		resumeFn: func(_ context.Context, input ragservice.RagChatApprovalResumeInput, sink ragservice.RagChatEventSink) error {
			captured = input
			if err := sink.SendApprovalPending(ragservice.RagChatApprovalPendingPayload{
				Required:     true,
				Status:       "pending",
				CheckpointID: "cp-11",
				CanApprove:   true,
				CanReject:    true,
			}); err != nil {
				return err
			}
			return sink.SendDone()
		},
	})

	body := `{"conversationId":"conv-1","question":"hello","checkpointId":"cp-11","decision":"approved","decisionNote":"looks safe"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ragent/rag/v3/chat/approval/resume", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if captured.CheckpointID != "cp-11" || captured.Decision != "approved" || captured.ConversationID != "conv-1" {
		t.Fatalf("unexpected resume input: %+v", captured)
	}
	if !strings.Contains(rec.Body.String(), "event: approval_pending") ||
		!strings.Contains(rec.Body.String(), "event: agent_status") ||
		!strings.Contains(rec.Body.String(), "\"type\":\"approval_pending\"") ||
		!strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("expected approval_pending and done SSE events, got %s", rec.Body.String())
	}
}

func TestChatHandlerStreamsAgentServiceErrorAsLegacyAndUnifiedEvents(t *testing.T) {
	router := newChatRouter(chatServiceStub{
		chatFn: func(_ context.Context, _ ragservice.RagChatInput, sink ragservice.RagChatEventSink) error {
			if err := sink.SendAgentServiceError(ragservice.RagChatAgentServiceErrorPayload{
				Code:      agentapp.ErrorCodeApprovalSessionNotFound,
				Message:   "approval session not found",
				Kind:      agentapp.ErrorKindNotFound,
				Retryable: false,
			}); err != nil {
				return err
			}
			return sink.SendDone()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ragent/rag/v3/chat?question=resume+it", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "event: agent_service_error") ||
		!strings.Contains(rec.Body.String(), "event: agent_status") ||
		!strings.Contains(rec.Body.String(), "\"type\":\"service_error\"") ||
		!strings.Contains(rec.Body.String(), "\"code\":\"approval_session_not_found\"") {
		t.Fatalf("expected legacy and unified agent service error events, got %s", rec.Body.String())
	}
}

func newChatRouter(chatService chatServiceStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		contextx.Set(c, &contextx.LoginUser{UserID: "user-1"})
		c.Next()
	})
	group := router.Group("/api/ragent")
	raghttp.RegisterRoutes(group, nil, nil, nil, nil, chatService, nil, nil)
	return router
}
