package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestLoadSourceMessagesBuildsOrderedConversation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "prompt-history.json")
	payload := map[string]any{
		"results": []map[string]any{
			{"question": "Q1", "answer": "A1"},
			{"question": "Q2", "answer": "A2"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	messages, err := loadSourceMessages(inputPath)
	if err != nil {
		t.Fatalf("loadSourceMessages() error = %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Q1" {
		t.Fatalf("messages[0] = %+v, want user Q1", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "A1" {
		t.Fatalf("messages[1] = %+v, want assistant A1", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "Q2" {
		t.Fatalf("messages[2] = %+v, want user Q2", messages[2])
	}
	if messages[3].Role != "assistant" || messages[3].Content != "A2" {
		t.Fatalf("messages[3] = %+v, want assistant A2", messages[3])
	}
}

func TestLoadSourceMessagesRejectsMissingPairs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "prompt-history.json")
	payload := map[string]any{
		"results": []map[string]any{
			{"question": "Q1", "answer": ""},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := loadSourceMessages(inputPath); err == nil {
		t.Fatal("loadSourceMessages() expected error for empty answer")
	}
}

func TestFixedModelChatServiceUsesConfiguredModel(t *testing.T) {
	t.Parallel()

	base := &chatServiceStub{response: `{"schema_version":1,"goal":"g","active_priorities":["p1"]}`}
	svc := fixedModelChatService{base: base, modelID: "qwen-max-test"}

	jsonMode := true
	_, err := svc.ChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("hi")},
		JSONMode: &jsonMode,
	})
	if err != nil {
		t.Fatalf("ChatWithRequest() error = %v", err)
	}
	if base.lastModelID != "qwen-max-test" {
		t.Fatalf("lastModelID = %q, want qwen-max-test", base.lastModelID)
	}
}

func TestBuildOutputArtifactIncludesSummaryFields(t *testing.T) {
	t.Parallel()

	artifact := buildOutputArtifact(outputEnvelope{
		ModelID:        "qwen-max-test",
		SourceMessages: 20,
		Structured: map[string]any{
			"goal": "design rag",
		},
		Rendered: "目标：design rag",
		Raw:      `{"goal":"design rag"}`,
		Validation: map[string]any{
			"accepted": true,
		},
	})

	if artifact["model_id"] != "qwen-max-test" {
		t.Fatalf("model_id = %v, want qwen-max-test", artifact["model_id"])
	}
	if artifact["rendered_summary"] == "" {
		t.Fatal("rendered_summary expected non-empty")
	}
}

type chatServiceStub struct {
	response    string
	lastModelID string
}

func (s *chatServiceStub) Chat(string) (string, error) { return s.response, nil }

func (s *chatServiceStub) ChatWithRequest(convention.ChatRequest) (string, error) {
	return s.response, nil
}

func (s *chatServiceStub) ChatWithModel(_ convention.ChatRequest, modelID string) (string, error) {
	s.lastModelID = modelID
	return s.response, nil
}

func (s *chatServiceStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (s *chatServiceStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func TestMarshalValidationResultRoundTripsFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(raghistory.SummaryValidationResult{
		Accepted: true,
		Reason:   "ok",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["Accepted"] != true {
		t.Fatalf("Accepted = %v, want true", got["Accepted"])
	}
}
