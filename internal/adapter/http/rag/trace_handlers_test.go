package rag

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestToRagTraceNodeVOIncludesMemoryRecallSummary(t *testing.T) {
	node := toRagTraceNodeVO(domain.RagTraceNode{
		TraceID:  "trace-1",
		NodeID:   "long_term_memory",
		NodeType: "memory",
		NodeName: "long_term_memory",
		Status:   "success",
		ExtraData: `{
			"used": true,
			"candidateCount": 3,
			"selectedCount": 2,
			"ruleCount": 1,
			"factCandidateCount": 2,
			"factSelectedCount": 1,
			"ruleCacheLayer": "request",
			"factCacheLayer": "redis",
			"embeddingCacheLayer": "request",
			"recomputeReason": "fact_cache_miss",
			"sourceCounts": {"keyword": 2, "vector": 1},
			"contributionCounts": {"hybrid": 1, "keyword_only": 1},
			"memoryIds": ["mem-1", "mem-2"]
		}`,
	})

	if node.MemoryRecall == nil {
		t.Fatalf("expected memoryRecall summary, got %+v", node)
	}
	if node.MemoryRecall.SelectedCount != 2 || node.MemoryRecall.FactSelectedCount != 1 {
		t.Fatalf("unexpected memoryRecall payload: %+v", node.MemoryRecall)
	}
	if node.SessionRecall != nil {
		t.Fatalf("expected no sessionRecall payload, got %+v", node.SessionRecall)
	}
}

func TestToRagTraceNodeVOIncludesSessionRecallSummary(t *testing.T) {
	node := toRagTraceNodeVO(domain.RagTraceNode{
		TraceID:  "trace-1",
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
	})

	if node.SessionRecall == nil {
		t.Fatalf("expected sessionRecall summary, got %+v", node)
	}
	if node.SessionRecall.ExcerptCount != 1 || node.SessionRecall.CacheLayer != "conversation" {
		t.Fatalf("unexpected sessionRecall payload: %+v", node.SessionRecall)
	}
	if node.MemoryRecall != nil {
		t.Fatalf("expected no memoryRecall payload, got %+v", node.MemoryRecall)
	}
}
