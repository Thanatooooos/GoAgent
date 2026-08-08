package citation

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

func TestRenderKnowledgeContextCarriesHandles(t *testing.T) {
	r := NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", ChunkIndex: 3, Text: "内容A", Metadata: map[string]any{"section": "背景"}},
		{ID: "chunk-b", DocumentID: "doc-b", KnowledgeBaseID: "kb-b", Text: "内容B"},
	}
	rendered := RenderKnowledgeContext(r, chunks)
	for _, want := range []string{`<retrieval type="knowledge">`, `id="c1"`, `id="c2"`, "内容A", "内容B", `section="背景"`, `index="3"`} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q:\n%s", want, rendered)
		}
	}
	if _, ok := r.ResolveChunk("c1"); !ok {
		t.Fatal("render should register chunks so they are resolvable")
	}
}

func TestRenderKnowledgeContextEmpty(t *testing.T) {
	if got := RenderKnowledgeContext(NewRegistry(), nil); got != "" {
		t.Fatalf("empty render = %q, want empty", got)
	}
}

func TestRenderKnowledgeContextEscapesText(t *testing.T) {
	r := NewRegistry()
	rendered := RenderKnowledgeContext(r, []convention.RetrievedChunk{
		{ID: "chunk-a", Text: "a<b>&c"},
	})
	if !strings.Contains(rendered, "a&lt;b&gt;&amp;c") {
		t.Fatalf("text not escaped: %s", rendered)
	}
}

func TestRenderKnowledgeContextWithBudgetTruncates(t *testing.T) {
	r := NewRegistry()
	chunks := []convention.RetrievedChunk{
		{ID: "chunk-a", Text: strings.Repeat("x", 200)},
		{ID: "chunk-b", Text: "短"},
	}
	const budget = 100
	rendered, stats := RenderKnowledgeContextWithBudget(r, chunks, budget, tokenbudget.RuneEstimator{})
	if stats.CandidateChunks != 2 {
		t.Fatalf("candidate chunks = %d, want 2", stats.CandidateChunks)
	}
	if stats.RetainedChunks == 0 {
		t.Fatal("budget render retained zero chunks")
	}
	if stats.RetainedChunks >= stats.CandidateChunks {
		t.Fatalf("first chunk too long, second should be dropped: retained=%d candidate=%d", stats.RetainedChunks, stats.CandidateChunks)
	}
	if stats.TokensAfter > budget {
		t.Fatalf("budget exceeded: tokensAfter=%d budget=%d", stats.TokensAfter, budget)
	}
	if !strings.Contains(rendered, `id="c1"`) {
		t.Fatalf("handle must survive truncation: %s", rendered)
	}
	if !strings.HasSuffix(rendered, "</retrieval>") {
		t.Fatalf("render must close retrieval tag: %s", rendered)
	}
	if !stats.Truncated {
		t.Fatal("expected truncated=true when chunks are dropped")
	}
}

func TestRenderKnowledgeContextWithBudgetEnvelopeTooLarge(t *testing.T) {
	r := NewRegistry()
	chunks := []convention.RetrievedChunk{{ID: "chunk-a", Text: "x"}}
	rendered, stats := RenderKnowledgeContextWithBudget(r, chunks, 10, tokenbudget.RuneEstimator{})
	if rendered != "" {
		t.Fatalf("envelope-only budget must render empty, got %q", rendered)
	}
	if stats.RetainedChunks != 0 {
		t.Fatalf("retained chunks = %d, want 0", stats.RetainedChunks)
	}
	if !stats.Truncated {
		t.Fatal("expected truncated=true when the envelope cannot fit")
	}
}

func TestRenderKnowledgeContextWithBudgetNilRegistry(t *testing.T) {
	rendered, stats := RenderKnowledgeContextWithBudget(nil, []convention.RetrievedChunk{{ID: "chunk-a", Text: "x"}}, 100, tokenbudget.RuneEstimator{})
	if rendered != "" {
		t.Fatalf("nil registry must render empty, got %q", rendered)
	}
	if stats.CandidateChunks != 1 {
		t.Fatalf("candidate chunks = %d, want 1", stats.CandidateChunks)
	}
}
