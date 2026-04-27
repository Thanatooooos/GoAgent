package test

import (
	"testing"

	"local/rag-project/internal/infra-ai/chat"
)

func TestParseOpenAIStyleSseLine_BlankLine(t *testing.T) {
	event, err := chat.ParseOpenAIStyleSseLine("   ", false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if event.HasContent() || event.HasReasoning() || event.Completed {
		t.Fatalf("expected empty event, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_Done(t *testing.T) {
	event, err := chat.ParseOpenAIStyleSseLine("data: [DONE]", false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if !event.Completed {
		t.Fatalf("expected completed event, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_DeltaContent(t *testing.T) {
	line := `data: {"choices":[{"delta":{"content":"hello"}}]}`

	event, err := chat.ParseOpenAIStyleSseLine(line, false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if event.Content != "hello" {
		t.Fatalf("expected content hello, got %+v", event)
	}
	if event.HasReasoning() {
		t.Fatalf("did not expect reasoning, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_MessageFallback(t *testing.T) {
	line := `{"choices":[{"message":{"content":"full-message"}}]}`

	event, err := chat.ParseOpenAIStyleSseLine(line, false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if event.Content != "full-message" {
		t.Fatalf("expected message content, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_ReasoningEnabled(t *testing.T) {
	line := `data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}`

	event, err := chat.ParseOpenAIStyleSseLine(line, true)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if event.Reasoning != "thinking" {
		t.Fatalf("expected reasoning thinking, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_ReasoningDisabled(t *testing.T) {
	line := `data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}`

	event, err := chat.ParseOpenAIStyleSseLine(line, false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if event.HasReasoning() {
		t.Fatalf("expected reasoning to be disabled, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_FinishReasonMarksCompleted(t *testing.T) {
	line := `data: {"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`

	event, err := chat.ParseOpenAIStyleSseLine(line, false)
	if err != nil {
		t.Fatalf("ParseOpenAIStyleSseLine returned error: %v", err)
	}
	if !event.Completed {
		t.Fatalf("expected completed event, got %+v", event)
	}
}

func TestParseOpenAIStyleSseLine_InvalidJSON(t *testing.T) {
	if _, err := chat.ParseOpenAIStyleSseLine("data: not-json", false); err == nil {
		t.Fatal("expected invalid json to return error")
	}
}
