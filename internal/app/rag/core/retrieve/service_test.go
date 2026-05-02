package retrieve

import (
	"strings"
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestBuildKnowledgeContext(t *testing.T) {
	context := BuildKnowledgeContext([]convention.RetrievedChunk{
		{Text: "A"},
		{Text: "B"},
	})

	if !strings.Contains(context, "[1] A") || !strings.Contains(context, "[2] B") {
		t.Fatalf("unexpected context: %q", context)
	}
}
