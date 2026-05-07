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
		ToolContext:      "document_query: matched doc-1",
		AnswerGuidance:   "先给结论，再给证据和建议。",
		History: []convention.ChatMessage{
			convention.UserMessage("previous"),
		},
	})
	if err != nil {
		t.Fatalf("BuildMessages returned error: %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(messages))
	}
	if messages[0].Role != convention.SystemRole {
		t.Fatalf("expected system prompt message, got %s", messages[0].Role)
	}
	if messages[1].Role != convention.SystemRole {
		t.Fatalf("expected knowledge context message, got %s", messages[1].Role)
	}
	if messages[2].Role != convention.SystemRole {
		t.Fatalf("expected tool context message, got %s", messages[2].Role)
	}
	if messages[3].Role != convention.SystemRole {
		t.Fatalf("expected answer guidance message, got %s", messages[3].Role)
	}
	if messages[5].Content != "What is RAG?" {
		t.Fatalf("unexpected final question: %q", messages[5].Content)
	}
}
