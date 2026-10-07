package conversation

import (
	"context"
	"testing"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
)

func TestMessageSourcesCapturesReferencedSourcesInOrder(t *testing.T) {
	content := `见 <kb doc="A &amp; B" chunk_id="chunk-1" kb_id="kb-1" /> 和 <web url="https://example.com/a" title="Site" />；再见 <kb doc="A &amp; B" chunk_id="chunk-1" />。`
	sources := messageSources("assistant", content)
	if len(sources) != 2 || sources[0].Type != "kb" || sources[0].Title != "A & B" || sources[0].ChunkID != "chunk-1" || sources[0].KnowledgeBaseID != "kb-1" || sources[1].Type != "web" || sources[1].URL != "https://example.com/a" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestMessageSourcesIgnoresUserContentAndInvalidTags(t *testing.T) {
	content := `<kb doc="x" /> <web url="javascript:alert(1)" />`
	if sources := messageSources("assistant", content); len(sources) != 0 {
		t.Fatalf("invalid sources = %#v", sources)
	}
	if sources := messageSources("user", `<kb chunk_id="chunk-1" />`); len(sources) != 0 {
		t.Fatalf("user sources = %#v", sources)
	}
}

func TestAddMessagePersistsAssistantSources(t *testing.T) {
	var saved domain.ConversationMessage
	service := NewMessageService(nil, conversationMessageRepoServiceStub{
		createFn: func(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
			saved = message
			return message, nil
		},
		listFn: func(context.Context, port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
			return nil, nil
		},
	}, nil, nil)
	_, err := service.AddMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1", UserID: "u1", Role: convention.AssistantRole,
		Content: `Answer <kb doc="Guide" chunk_id="chunk-1" kb_id="kb-1" />`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Sources) != 1 || saved.Sources[0].ChunkID != "chunk-1" || saved.Sources[0].Title != "Guide" {
		t.Fatalf("saved sources = %#v", saved.Sources)
	}
}

func TestSummarizedAssistantPreservesSourcesFromOriginalBody(t *testing.T) {
	service := NewMessageService(nil, nil, nil, nil)
	service.messageRepo = conversationMessageRepoServiceStub{}
	const original = `查询过程。结论 <kb doc="Guide" chunk_id="chunk-1" kb_id="kb-1" />`
	service.SetContentProcessor(summaryProcessorStub{result: ProcessedConversationMessageContent{
		Content: "短摘要", RawContent: original, ContentSummary: "短摘要", IsSummarized: true,
	}})
	message, _, err := service.PrepareMessage(context.Background(), AddConversationMessageInput{
		ConversationID: "c1", UserID: "u1", Role: convention.AssistantRole, Content: original,
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "短摘要" || message.DisplayContent() != original || len(message.Sources) != 1 || message.Sources[0].ChunkID != "chunk-1" {
		t.Fatalf("message=%#v", message)
	}
}
