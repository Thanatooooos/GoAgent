package retrieve

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

func TestBuildKnowledgeContextWithinBudgetKeepsHighestRankedChunks(t *testing.T) {
	chunks := []convention.RetrievedChunk{
		{ID: "high", Text: "high ranked evidence", Score: 0.9},
		{ID: "low", Text: "low ranked evidence", Score: 0.2},
	}

	contextText, stats := BuildKnowledgeContextWithinBudget(
		chunks,
		12,
		tokenbudget.FixedEstimator(8),
	)
	if !strings.Contains(contextText, "high ranked evidence") {
		t.Fatalf("context = %q, want highest-ranked chunk", contextText)
	}
	if strings.Contains(contextText, "low ranked evidence") {
		t.Fatalf("context = %q, low-ranked chunk should be dropped", contextText)
	}
	if !stats.Truncated || stats.RetainedChunks != 1 {
		t.Fatalf("stats = %+v, want truncated with one retained chunk", stats)
	}
}

func TestBuildKnowledgeContextWithinBudgetTruncatesOversizedChunk(t *testing.T) {
	chunks := []convention.RetrievedChunk{
		{ID: "large", DocumentID: "doc-1", Text: strings.Repeat("x", 40), Score: 0.9},
	}
	contextText, stats := BuildKnowledgeContextWithinBudget(chunks, 20, tokenbudget.RuneEstimator{})
	if !stats.Truncated {
		t.Fatalf("stats = %+v, want truncated", stats)
	}
	if tokens := (tokenbudget.RuneEstimator{}).EstimateTokens(contextText); tokens > 20 {
		t.Fatalf("context tokens = %d, want <= 20; context=%q", tokens, contextText)
	}
	if !strings.Contains(contextText, "doc-1") && !strings.Contains(contextText, "large") {
		t.Fatalf("context = %q, want source identifier retained", contextText)
	}
}
