package convention

import "testing"

func TestChatRequestValidate(t *testing.T) {
	temp := 0.7
	topP := 0.9
	topK := 20
	maxTokens := 512
	thinking := true
	enableTools := false

	req := ChatRequest{
		Messages:    []ChatMessage{UserMessage("hello")},
		Temperature: &temp,
		TopP:        &topP,
		TopK:        &topK,
		MaxTokens:   &maxTokens,
		Thinking:    &thinking,
		EnableTools: &enableTools,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !req.ThinkingEnabled() {
		t.Fatal("ThinkingEnabled should be true")
	}
	if req.ToolsEnabled() {
		t.Fatal("ToolsEnabled should be false")
	}
	if !req.HasTemperature() || !req.HasTopP() || !req.HasTopK() || !req.HasMaxTokens() {
		t.Fatal("expected all optional numeric fields to be present")
	}
}

func TestChatRequestValidate_Invalid(t *testing.T) {
	temp := 2.5
	req := ChatRequest{
		Messages:    []ChatMessage{UserMessage("hello")},
		Temperature: &temp,
	}

	if err := req.Validate(); err == nil {
		t.Fatal("Validate should reject invalid temperature")
	}
}

func TestChatRequestValidate_MessagesRequired(t *testing.T) {
	req := ChatRequest{}
	if err := req.Validate(); err == nil {
		t.Fatal("Validate should require messages")
	}
}
