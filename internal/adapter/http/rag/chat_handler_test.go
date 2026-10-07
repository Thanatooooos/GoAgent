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
	"local/rag-project/internal/middleware"
)

type thinkingAdmissionSpy struct {
	accessChatStub
	input  conversationruntime.ChatInput
	called bool
}

func (s *thinkingAdmissionSpy) Admit(_ context.Context, input conversationruntime.ChatInput) (conversationruntime.ChatInput, error) {
	s.input, s.called = input, true
	return conversationruntime.ChatInput{}, errors.New("stop after inspecting admission")
}

func TestChatParsesDeepThinkingPerRequest(t *testing.T) {
	for _, item := range []struct {
		name, method, url, body string
		want, invalid           bool
	}{
		{"enabled", "POST", "/chat", `{"question":"hello","deepThinking":true}`, true, false},
		{"disabled", "POST", "/chat", `{"question":"hello","deepThinking":false}`, false, false},
		{"omitted", "POST", "/chat", `{"question":"hello"}`, false, false},
		{"invalid-json", "POST", "/chat", `{"question":"hello","deepThinking":"true"}`, false, true},
		{"get-enabled", "GET", "/chat?question=hello&deepThinking=true", "", true, false},
		{"get-disabled", "GET", "/chat?question=hello&deepThinking=false", "", false, false},
		{"get-invalid", "GET", "/chat?question=hello&deepThinking=maybe", "", false, true},
	} {
		t.Run(item.name, func(t *testing.T) {
			chat := &thinkingAdmissionSpy{}
			h := NewHandler(nil, nil, nil, nil, nil, nil)
			h.SetRuntimeChat(chat)
			router := gin.New()
			router.Use(middleware.ErrorHandlerMiddleware(), func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: "owner"}) })
			router.Handle(item.method, "/chat", h.Chat)
			req := httptest.NewRequest(item.method, item.url, strings.NewReader(item.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if item.invalid {
				if chat.called || response.Code == http.StatusOK {
					t.Fatalf("invalid input admitted: %+v code=%d", chat, response.Code)
				}
			} else if !chat.called || chat.input.DeepThinking != item.want {
				t.Fatalf("admission=%+v want thinking=%t", chat, item.want)
			}
		})
	}
}

func TestContinueOffset(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want int
		bad  bool
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: "12", want: 12},
		{raw: "-1", bad: true},
		{raw: "nope", bad: true},
	} {
		got, err := continueOffset(test.raw)
		if test.bad {
			if err == nil {
				t.Fatalf("continueOffset(%q) error = nil", test.raw)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Fatalf("continueOffset(%q) = %d, %v; want %d, nil", test.raw, got, err, test.want)
		}
	}
}

func TestHasExplicitMemoryMutationIntent(t *testing.T) {
	for _, test := range []struct {
		question string
		want     bool
	}{
		{question: "请记住：讲 CSS 时给完整 HTML", want: true},
		{question: "我更喜欢从架构权衡开始解释", want: true},
		{question: "以后这个项目会用 Go", want: false},
		{question: "我正在调研 Redis", want: false},
	} {
		if got := hasExplicitMemoryMutationIntent(test.question); got != test.want {
			t.Fatalf("hasExplicitMemoryMutationIntent(%q) = %v, want %v", test.question, got, test.want)
		}
	}
}
