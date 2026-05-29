package recall

import (
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/rag/domain"
)

func TestProjectFactMemoryChunksBuildsChunkIDsTextAndMetadata(t *testing.T) {
	t.Parallel()

	items := []memoryRecallProjection{
		{
			item: domain.MemoryItem{
				ID:           "mem-1",
				ScopeType:    domain.MemoryScopeKB,
				ScopeID:      "kb-1",
				Namespace:    "project",
				MemoryType:   domain.MemoryTypeKnowledge,
				Category:     "project",
				CanonicalKey: "project.messaging.main_bus",
				DisplayValue: "Removed",
			},
			summary:        "Main bus removed",
			detail:         "RocketMQ has been removed",
			keywordMatched: true,
			vectorMatched:  true,
			keywordScore:   9,
			vectorScore:    0.75,
			finalScore:     88,
		},
	}

	chunks := projectFactMemoryChunks(items)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.ID != factMemoryChunkPrefix+"mem-1" {
		t.Fatalf("chunk ID = %q", chunk.ID)
	}
	if chunk.KnowledgeBaseID != "kb-1" {
		t.Fatalf("KnowledgeBaseID = %q", chunk.KnowledgeBaseID)
	}
	if !strings.Contains(chunk.Text, "Main bus removed") || !strings.Contains(chunk.Text, "Detail: RocketMQ has been removed") {
		t.Fatalf("unexpected chunk text: %q", chunk.Text)
	}
	if chunk.Metadata["source"] != ragretrieve.ChannelMemoryFact {
		t.Fatalf("unexpected source metadata: %+v", chunk.Metadata)
	}
	if chunk.Metadata["contribution_kind"] != memoryContributionHybrid {
		t.Fatalf("unexpected contribution metadata: %+v", chunk.Metadata)
	}
}

func TestRenderFactMemorySectionReflectsScopeAndCanonicalKey(t *testing.T) {
	t.Parallel()

	got := renderFactMemorySection(domain.MemoryItem{
		ScopeType:    domain.MemoryScopeKB,
		CanonicalKey: "project.messaging.main_bus",
	})
	if got != "Fact Memory > KB Scoped > project.messaging.main_bus" {
		t.Fatalf("renderFactMemorySection() = %q", got)
	}
}
