package prompt

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestServiceBuildMessages(t *testing.T) {
	service := NewService(nil)

	messages, err := service.BuildMessages(Context{
		Question:         "What is RAG?",
		KnowledgeContext: "[1] retrieval augmented generation",
		History: []convention.ChatMessage{
			convention.UserMessage("previous"),
		},
	})
	if err != nil {
		t.Fatalf("BuildMessages returned error: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	if messages[0].Role != convention.SystemRole {
		t.Fatalf("expected system prompt message, got %s", messages[0].Role)
	}
	if messages[1].Role != convention.SystemRole {
		t.Fatalf("expected knowledge context message, got %s", messages[1].Role)
	}
	if messages[3].Content != "What is RAG?" {
		t.Fatalf("unexpected final question: %q", messages[3].Content)
	}
}
