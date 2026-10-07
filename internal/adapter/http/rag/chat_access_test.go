package rag

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/stream"
	"local/rag-project/internal/middleware"
)

type accessChatStub struct {
	allowed          bool
	err              error
	cancels, replays int
}

func (s *accessChatStub) Admit(context.Context, conversationruntime.ChatInput) (conversationruntime.ChatInput, error) {
	return conversationruntime.ChatInput{}, errors.New("admission failed")
}
func (s *accessChatStub) Chat(context.Context, conversationruntime.ChatInput, conversationruntime.EventSink) (conversationruntime.ChatResult, error) {
	panic("unexpected model execution")
}
func (s *accessChatStub) AuthorizeTask(context.Context, string, string) (bool, error) {
	return s.allowed, s.err
}
func (s *accessChatStub) CancelTask(string) bool { s.cancels++; return true }
func (s *accessChatStub) Replay(ctx context.Context, _ string, task string, sink conversationruntime.EventSink) (bool, error) {
	s.replays++
	if err := sink.Append(ctx, conversationruntime.JournalEntry{EventType: conversationruntime.EventAnswerDelta, Detail: "private answer"}); err != nil {
		return false, err
	}
	return true, sink.Append(ctx, conversationruntime.JournalEntry{EventType: conversationruntime.EventCompleted})
}

type accessStreamSpy struct {
	stream.StreamManager
	reads, writes int
}

func (s *accessStreamSpy) GetEvents(ctx context.Context, id string, offset int) ([]stream.StreamEvent, int, error) {
	s.reads++
	return s.StreamManager.GetEvents(ctx, id, offset)
}
func (s *accessStreamSpy) AppendEvent(ctx context.Context, id string, event stream.StreamEvent) error {
	s.writes++
	return s.StreamManager.AppendEvent(ctx, id, event)
}

func TestChatExecutionAccessPrecedesCacheReplayAndCancellation(t *testing.T) {
	for _, hot := range []bool{false, true} {
		for _, action := range []string{"continue", "stop"} {
			for _, access := range []string{"owner", "other-user", "deleted", "work", "unknown", "database-error", "anonymous"} {
				t.Run(action+"/"+access+map[bool]string{true: "/hot", false: "/cold"}[hot], func(t *testing.T) {
					m := stream.NewMemoryStreamManager()
					if hot {
						_ = m.AppendEvent(context.Background(), "task", stream.StreamEvent{Name: "message", Data: []byte(`{"delta":"private answer"}`)})
						_ = m.AppendEvent(context.Background(), "task", stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})
					}
					spy := &accessStreamSpy{StreamManager: m}
					chat := &accessChatStub{allowed: access == "owner"}
					if access == "database-error" {
						chat.err = errors.New("database unavailable")
					}
					h := NewHandler(nil, nil, nil, nil, nil, spy)
					h.SetRuntimeChat(chat)
					h.SetRuntimeReplay(chat)
					r := gin.New()
					r.Use(middleware.ErrorHandlerMiddleware(), func(c *gin.Context) {
						if access != "anonymous" {
							contextx.Set(c, &contextx.LoginUser{UserID: access})
						}
					})
					r.GET("/continue", h.ContinueChat)
					r.POST("/stop", h.StopChat)
					method := http.MethodGet
					if action == "stop" {
						method = http.MethodPost
					}
					w := httptest.NewRecorder()
					r.ServeHTTP(w, httptest.NewRequest(method, "/"+action+"?taskId=task", nil))
					if access != "owner" {
						if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "private answer") || spy.reads != 0 || spy.writes != 0 || chat.cancels != 0 || chat.replays != 0 {
							t.Fatalf("unauthorized side effects: status=%d body=%s reads=%d writes=%d chat=%+v", w.Code, w.Body.String(), spy.reads, spy.writes, chat)
						}
					} else {
						if w.Code != http.StatusOK {
							t.Fatalf("owner rejected: %d %s", w.Code, w.Body.String())
						}
						if action == "continue" && !strings.Contains(w.Body.String(), "private answer") {
							t.Fatal("owner lost replay")
						}
						if action == "stop" && (chat.cancels != 1 || spy.writes != 1) {
							t.Fatal("owner stop not executed")
						}
					}
				})
			}
		}
	}
}

func TestChatAdmissionFailureDoesNotExposeStream(t *testing.T) {
	spy := &accessStreamSpy{StreamManager: stream.NewMemoryStreamManager()}
	h := NewHandler(nil, nil, nil, nil, nil, spy)
	h.SetRuntimeChat(&accessChatStub{})
	r := gin.New()
	r.Use(middleware.ErrorHandlerMiddleware(), func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: "owner"}) })
	r.GET("/chat", h.Chat)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/chat?question=hello", nil))
	if w.Code == http.StatusOK || spy.reads != 0 || spy.writes != 0 || strings.Contains(w.Body.String(), "event: meta") {
		t.Fatalf("failed admission exposed stream: %d %s %+v", w.Code, w.Body.String(), spy)
	}
}
