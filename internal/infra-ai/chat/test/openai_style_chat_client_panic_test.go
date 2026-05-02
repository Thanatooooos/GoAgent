package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

type panicOnContentCallback struct {
	err  error
	done chan struct{}
}

func (p *panicOnContentCallback) OnContent(string) {
	panic("boom")
}

func (p *panicOnContentCallback) OnThinking(string) {}

func (p *panicOnContentCallback) OnComplete() {}

func (p *panicOnContentCallback) OnError(err error) {
	p.err = err
	select {
	case p.done <- struct{}{}:
	default:
	}
}

func TestOpenAIStyleChatClientStreamChatRecoversCallbackPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			f.Flush()
		}
	}))
	defer srv.Close()

	client := chat.NewOpenAIStyleChatClient("test-provider", srv.Client())
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-model"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"chat": "/chat"},
		},
	}
	req := convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("ping")},
	}
	callback := &panicOnContentCallback{done: make(chan struct{}, 1)}

	_, err := client.StreamChat(req, callback, target)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	select {
	case <-callback.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback error")
	}
	if callback.err == nil {
		t.Fatal("expected callback error after panic")
	}
}
