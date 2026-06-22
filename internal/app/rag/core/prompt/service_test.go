package prompt

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestServiceBuildMessages(t *testing.T) {
	service := NewService(nil)

	messages, err := service.BuildMessages(Context{
		Question:         "What is RAG?",
		MemoryContext:    "[scope=global type=preference] answer in Chinese",
		SessionContext:   "[1] earlier log excerpt",
		KnowledgeContext: "[1] retrieval augmented generation",
		ToolContext:      "document_query: matched doc-1",
		WorkflowPolicy:   "capability: knowledge\nexecution: read_only",
		AnswerGuidance:   "Lead with the answer, then provide evidence.",
		History: []convention.ChatMessage{
			convention.UserMessage("previous"),
		},
	})
	if err != nil {
		t.Fatalf("BuildMessages returned error: %v", err)
	}
	if len(messages) != 9 {
		t.Fatalf("expected 9 messages, got %d", len(messages))
	}
	if messages[0].Role != convention.SystemRole {
		t.Fatalf("expected system prompt message, got %s", messages[0].Role)
	}
	if messages[1].Content != "## Long-Term Memory\nUse these persistent user- or knowledge-base-specific memories when they are relevant to the current question. If the current user request explicitly conflicts with a recalled preference, follow the current user request.\n[scope=global type=preference] answer in Chinese" {
		t.Fatalf("unexpected memory context message: %q", messages[1].Content)
	}
	if messages[2].Content != "## 会话上下文片段\n[1] earlier log excerpt" {
		t.Fatalf("unexpected session context message: %q", messages[2].Content)
	}
	if messages[8].Content != "What is RAG?" {
		t.Fatalf("unexpected final question: %q", messages[8].Content)
	}
}
