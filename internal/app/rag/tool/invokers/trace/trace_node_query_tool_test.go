package builtin

import (
	"strings"
	"testing"

	ragdomain "local/rag-project/internal/app/rag/domain"
)

func TestSummarizeTraceNodesIncludesMemoryRecallSummary(t *testing.T) {
	items := summarizeTraceNodes([]ragdomain.RagTraceNode{
		{
			NodeID:   "long_term_memory",
			NodeType: "memory",
			NodeName: "long_term_memory",
			Status:   "success",
			ExtraData: `{
				"candidateCount": 3,
				"selectedCount": 2,
				"truncated": false,
				"sourceCounts": {"keyword": 2, "vector": 1},
				"contributionCounts": {"hybrid": 1, "keyword_only": 1},
				"memoryIds": ["mem-1", "mem-2"]
			}`,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected one summarized node, got %+v", items)
	}
	summary, _ := items[0]["summary"].(string)
	if !strings.Contains(summary, "selected 2/3 memories") || !strings.Contains(summary, "keyword=2") {
		t.Fatalf("expected human summary, got %+v", items[0])
	}
	memoryRecall, ok := items[0]["memoryRecall"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured memoryRecall payload, got %+v", items[0])
	}
	if memoryRecall["selectedCount"] != 2 || memoryRecall["candidateCount"] != 3 {
		t.Fatalf("unexpected memory recall counts: %+v", memoryRecall)
	}
}
