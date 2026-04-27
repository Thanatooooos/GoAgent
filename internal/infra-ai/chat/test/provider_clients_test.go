package test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

func TestProviderConstructors(t *testing.T) {
	httpClient := &http.Client{}

	bailian := chat.NewBaiLianChatClient(httpClient)
	if bailian.Provider() != "bailian" {
		t.Fatalf("unexpected bailian provider: %s", bailian.Provider())
	}

	siliconflow := chat.NewSiliconFlowChatClient(httpClient)
	if siliconflow.Provider() != "siliconflow" {
		t.Fatalf("unexpected siliconflow provider: %s", siliconflow.Provider())
	}

	ollama := chat.NewOllamaChatClient(httpClient)
	if ollama.Provider() != "ollama" {
		t.Fatalf("unexpected ollama provider: %s", ollama.Provider())
	}
}

func TestProviderClientOptions(t *testing.T) {
	headersCalled := false
	customizerCalled := false
	parserCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Test") != "1" {
			t.Fatalf("expected custom header, got %q", req.Header.Get("X-Test"))
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if string(body) == "" {
			t.Fatal("expected non-empty request body")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			f.Flush()
		}
	}))
	defer srv.Close()

	client := chat.NewBaiLianChatClient(
		srv.Client(),
		chat.WithHeaderBuilder(func(target model.ModelTarget) http.Header {
			headersCalled = true
			h := make(http.Header)
			h.Set("X-Test", "1")
			return h
		}),
		chat.WithBodyCustomizer(func(body map[string]any, req convention.ChatRequest, target model.ModelTarget) {
			customizerCalled = true
			body["x_test"] = true
		}),
		chat.WithStreamParser(func(line string, reasoningEnabled bool) (chat.ParsedEvent, error) {
			parserCalled = true
			return chat.ParsedEvent{Completed: true}, nil
		}),
	)

	req := convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("hello")},
	}
	target := model.ModelTarget{
		Candidate: config.ModelCandidate{Model: "demo-model"},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "secret",
			Endpoints: map[string]string{"chat": "/chat"},
		},
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

	if !headersCalled || !customizerCalled || !parserCalled {
		t.Fatalf("expected all custom hooks to be invoked: headers=%v customizer=%v parser=%v", headersCalled, customizerCalled, parserCalled)
	}
}
