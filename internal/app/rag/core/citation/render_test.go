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
		{ID: "chunk-a", Text: strings.Repeat("很长的内容", 200)},
		{ID: "chunk-b", Text: "短的"},
	}
	rendered, stats := RenderKnowledgeContextWithBudget(r, chunks, 30, tokenbudget.NewDefaultEstimator())
	if stats.RetainedChunks == 0 {
		t.Fatal("budget render retained zero chunks")
	}
	if !strings.Contains(rendered, "</retrieval>") {
		t.Fatalf("budget render must close retrieval tag: %s", rendered)
	}
	if stats.CandidateChunks != 2 {
		t.Fatalf("candidate chunks = %d, want 2", stats.CandidateChunks)
	}
}
