package test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

type recordingStreamCallback struct {
	mu        sync.Mutex
	content   []string
	thinking  []string
	completed bool
	err       error
	done      chan struct{}
}

func newRecordingStreamCallback() *recordingStreamCallback {
	return &recordingStreamCallback{
		done: make(chan struct{}, 1),
	}
}

func (r *recordingStreamCallback) OnContent(content string) {
	r.mu.Lock()
	r.content = append(r.content, content)
	r.mu.Unlock()
}

func (r *recordingStreamCallback) OnThinking(content string) {
	r.mu.Lock()
	r.thinking = append(r.thinking, content)
	r.mu.Unlock()
}

func (r *recordingStreamCallback) OnComplete() {
	r.mu.Lock()
	r.completed = true
	r.mu.Unlock()
	select {
	case r.done <- struct{}{}:
	default:
	}
}

func (r *recordingStreamCallback) OnError(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
	select {
	case r.done <- struct{}{}:
	default:
	}
}

func TestOpenAIStyleChatClientChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", req.Method)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request failed: %v", err)
		}
		if payload["model"] != "demo-model" {
			t.Fatalf("unexpected model payload: %+v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
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

	content, err := client.Chat(req, target)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if content != "hello" {
		t.Fatalf("Chat returned %q, want hello", content)
	}
}

func TestOpenAIStyleChatClientStreamChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n"))
			f.Flush()
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			f.Flush()
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
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
	thinking := true
	req := convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("ping")},
		Thinking: &thinking,
	}
	callback := newRecordingStreamCallback()

	handle, err := client.StreamChat(req, callback, target)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if handle == nil {
		t.Fatal("expected non-nil cancellation handle")
	}

	select {
	case <-callback.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream completion")
	}

	callback.mu.Lock()
	defer callback.mu.Unlock()
	if callback.err != nil {
		t.Fatalf("unexpected callback error: %v", callback.err)
	}
	if !callback.completed {
		t.Fatal("expected stream completion")
	}
	if len(callback.thinking) != 1 || callback.thinking[0] != "think" {
		t.Fatalf("unexpected thinking events: %+v", callback.thinking)
	}
	if len(callback.content) != 1 || callback.content[0] != "hello" {
		t.Fatalf("unexpected content events: %+v", callback.content)
	}
}
