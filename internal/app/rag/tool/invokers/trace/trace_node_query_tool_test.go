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
				"ruleCount": 1,
				"factCandidateCount": 2,
				"factSelectedCount": 1,
				"ruleCacheLayer": "request",
				"factCacheLayer": "redis",
				"embeddingCacheLayer": "request",
				"recomputeReason": "fact_cache_miss",
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
	if memoryRecall["ruleCacheLayer"] != "request" || memoryRecall["factCacheLayer"] != "redis" {
		t.Fatalf("expected cache-layer details, got %+v", memoryRecall)
	}
	if memoryRecall["factSelectedCount"] != 1 || memoryRecall["recomputeReason"] != "fact_cache_miss" {
		t.Fatalf("expected fact selection details, got %+v", memoryRecall)
	}
}

func TestSummarizeTraceNodesIncludesSessionRecallSummary(t *testing.T) {
	items := summarizeTraceNodes([]ragdomain.RagTraceNode{
		{
			NodeID:   "session_recall",
			NodeType: "memory",
			NodeName: "session_recall",
			Status:   "success",
			ExtraData: `{
				"used": true,
				"candidateCount": 4,
				"excerptCount": 1,
				"topScore": 0.91,
				"cacheLayer": "conversation",
				"embeddingCacheLayer": "request",
				"recallFingerprint": "fp-1",
				"recomputeReason": "conversation_cache_miss",
				"truncatedBy": "max_prompt_tokens",
				"skippedPerMessageLimit": 1,
				"selectedHits": [{"messageId": "msg-1", "sourceChunkId": "chunk-1"}]
			}`,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected one summarized node, got %+v", items)
	}
	summary, _ := items[0]["summary"].(string)
	if !strings.Contains(summary, "recalled 1/4 excerpts") || !strings.Contains(summary, "truncatedBy=max_prompt_tokens") {
		t.Fatalf("expected session recall summary, got %+v", items[0])
	}
	sessionRecall, ok := items[0]["sessionRecall"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured sessionRecall payload, got %+v", items[0])
	}
	if sessionRecall["excerptCount"] != 1 || sessionRecall["cacheLayer"] != "conversation" {
		t.Fatalf("unexpected session recall summary: %+v", sessionRecall)
	}
	if sessionRecall["recomputeReason"] != "conversation_cache_miss" {
		t.Fatalf("expected recompute reason, got %+v", sessionRecall)
	}
}
