package chat

import (
	"context"
	"strings"
	"testing"

	ragcitation "local/rag-project/internal/app/rag/core/citation"
	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

func TestPrepareChatRegistersAndRendersCitations(t *testing.T) {
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", Text: "内容A", Metadata: map[string]any{"document_title": "标题A"}},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve, func(_ *RagChatDeps, opts *RagChatOptions) {
		opts.CitationEnabled = true
	})
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if !strings.Contains(prepared.retrieveResult.KnowledgeContext, `id="c1"`) {
		t.Fatalf("knowledge context missing c1 handle: %s", prepared.retrieveResult.KnowledgeContext)
	}
	if prepared.state.citation == nil {
		t.Fatal("state.citation should be set")
	}
	ref, ok := prepared.state.citation.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" || ref.DocumentTitle != "标题A" {
		t.Fatalf("chunk not registered correctly: %+v, %v", ref, ok)
	}
}

func TestPrepareChatSkipsCitationRenderWhenDisabled(t *testing.T) {
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", Text: "内容A"},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve)
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if strings.Contains(prepared.retrieveResult.KnowledgeContext, "retrieval") {
		t.Fatalf("citation disabled should keep plain context: %s", prepared.retrieveResult.KnowledgeContext)
	}
	if prepared.state.citation != nil {
		t.Fatal("state.citation should be nil when disabled")
	}
}

func TestPrepareChatCompactsHistoryCitations(t *testing.T) {
	history := []convention.ChatMessage{
		convention.AssistantMessage(`根据 <kb doc="标题" chunk_id="chunk-a" /> 说明。`),
	}
	retrieve := &retrieveServiceStub{
		result: ragretrieve.Result{
			Chunks: []convention.RetrievedChunk{
				{ID: "chunk-a", Text: "内容A"},
			},
		},
	}
	service, _ := newPrepareChatTestService(t, ragrewrite.Result{NeedRetrieval: true}, nil, retrieve, func(deps *RagChatDeps, opts *RagChatOptions) {
		opts.CitationEnabled = true
		deps.HistoryService = memoryServiceStub{history: history}
	})
	prepared, err := service.prepareChat(context.Background(), RagChatInput{
		UserID:           "user-1",
		Question:         "问题",
		KnowledgeBaseIDs: []string{"kb-1"},
	})
	if err != nil {
		t.Fatalf("prepareChat() error = %v", err)
	}
	if len(prepared.history) != 1 || !strings.Contains(prepared.history[0].Content, `<ref id="c1"/>`) {
		t.Fatalf("history not compacted: %+v", prepared.history)
	}
	if strings.Contains(prepared.history[0].Content, "chunk-a") {
		t.Fatalf("durable id leaked in history: %s", prepared.history[0].Content)
	}
}

func TestRenderThenExpandRoundTrip(t *testing.T) {
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", Text: "内容", Metadata: map[string]any{"document_title": "标题"}},
	}
	rendered := ragcitation.RenderKnowledgeContext(registry, chunks)
	if !strings.Contains(rendered, `id="c1"`) {
		t.Fatalf("render missing handle: %s", rendered)
	}
	expanded := registry.ExpandText(`答案是 <ref id="c1"/>。`, true)
	if !strings.Contains(expanded, `<kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" />`) {
		t.Fatalf("expand round trip failed: %s", expanded)
	}
}

func TestProtocolPromptInjectedWhenEnabled(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{CitationEnabled: true})
	promptCtx := ragprompt.Context{
		Question:         "问题",
		KnowledgeContext: "<retrieval>…</retrieval>",
		CitationProtocol: ragcitation.ProtocolPrompt(true),
	}
	messages, err := service.promptService.BuildMessages(promptCtx)
	if err != nil {
		t.Fatalf("BuildMessages() error = %v", err)
	}
	var found bool
	for _, m := range messages {
		if m.Role == convention.SystemRole && strings.Contains(m.Content, "<ref id=\"cN\"/>") {
			found = true
		}
	}
	if !found {
		t.Fatalf("citation protocol not injected: %+v", messages)
	}
}

func TestApplyRetrieveContextBudgetUsesCitationRenderer(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		CitationEnabled:   true,
		ChatContextBudget: ChatContextBudgetOptions{RetrieveTokens: 1000},
	})
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: strings.Repeat("内容", 500)},
	}
	rendered := ragcitation.RenderKnowledgeContext(registry, chunks)
	result := service.applyRetrieveContextBudget(
		context.Background(),
		"trace-1",
		ragretrieve.Result{Chunks: chunks, KnowledgeContext: rendered},
		registry,
	)
	if !strings.Contains(result.KnowledgeContext, `<chunk id="c1"`) {
		t.Fatalf("budget render missing handle: %s", result.KnowledgeContext)
	}
}
