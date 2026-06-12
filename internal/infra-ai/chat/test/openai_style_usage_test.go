package test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

func TestOpenAIStyleChatClientParsesUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		if len(body) == 0 {
			t.Fatal("expected non-empty request body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"hello"}}],
			"usage":{"prompt_tokens":12,"completion_tokens":3,"total_tokens":15}
		}`))
	}))
	defer srv.Close()

	client := chat.NewBaiLianChatClient(srv.Client())
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("hi")},
	}
	target := model.ModelTarget{
		Id: "test-model",
		Candidate: config.ModelCandidate{
			Provider: client.Provider(),
			Model:    "qwen-test",
		},
		Provider: config.ProviderConfig{
			Url:       srv.URL,
			ApiKey:    "test-key",
			Endpoints: map[string]string{"chat": "/chat/completions"},
		},
	}

	content, usage, err := client.ChatWithUsage(request, target)
	if err != nil {
		t.Fatalf("ChatWithUsage() error = %v", err)
	}
	if content != "hello" {
		t.Fatalf("unexpected content: %q", content)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 3 || usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}
