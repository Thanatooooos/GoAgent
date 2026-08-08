package chat

import (
	"context"
	"strings"
	"testing"

	ragcitation "local/rag-project/internal/app/rag/core/citation"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

func TestApplyRetrieveContextBudgetSkipsRebuildAfterFallback(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		CitationEnabled:   true,
		ChatContextBudget: ChatContextBudgetOptions{RetrieveTokens: 1000},
	})
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: "内容"},
	}
	// Simulate confidence fallback: context cleared, chunks still present.
	result := service.applyRetrieveContextBudget(
		context.Background(),
		"trace-1",
		ragretrieve.Result{Chunks: chunks, KnowledgeContext: ""},
		registry,
	)
	if strings.TrimSpace(result.KnowledgeContext) != "" {
		t.Fatalf("fallback context must stay empty, got %q", result.KnowledgeContext)
	}
}

func TestApplyRetrieveContextBudgetStillRebuildsWhenContextPresent(t *testing.T) {
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		CitationEnabled:   true,
		ChatContextBudget: ChatContextBudgetOptions{RetrieveTokens: 1000},
	})
	registry := ragcitation.NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: "内容"},
	}
	result := service.applyRetrieveContextBudget(
		context.Background(),
		"trace-1",
		ragretrieve.Result{Chunks: chunks, KnowledgeContext: ragcitation.RenderKnowledgeContext(registry, chunks)},
		registry,
	)
	if !strings.Contains(result.KnowledgeContext, `id="c1"`) {
		t.Fatalf("non-fallback context should be rebuilt with handles, got %q", result.KnowledgeContext)
	}
}
