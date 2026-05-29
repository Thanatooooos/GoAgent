package traceinsight

import (
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestParseLongTermMemoryNode(t *testing.T) {
	summary := ParseLongTermMemoryNode(&domain.RagTraceNode{
		NodeID: "long_term_memory",
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
	if summary == nil {
		t.Fatal("expected long-term memory summary")
	}
	if summary.SelectedCount != 2 || summary.RuleCacheLayer != "request" || summary.FactCacheLayer != "redis" {
		t.Fatalf("unexpected long-term memory summary: %+v", summary)
	}
	if summary.Summary == "" {
		t.Fatalf("expected rendered summary, got %+v", summary)
	}
}

func TestParseSessionRecallNode(t *testing.T) {
	summary := ParseSessionRecallNode(&domain.RagTraceNode{
		NodeID: "session_recall",
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
	if summary == nil {
		t.Fatal("expected session recall summary")
	}
	if summary.ExcerptCount != 1 || summary.CacheLayer != "conversation" || summary.SelectedChunkIDs[0] != "chunk-1" {
		t.Fatalf("unexpected session recall summary: %+v", summary)
	}
	if summary.Summary == "" {
		t.Fatalf("expected rendered summary, got %+v", summary)
	}
}

func TestParseLongTermMemoryPayloadDoesNotMisreadSessionRecall(t *testing.T) {
	summary := ParseLongTermMemoryNode(&domain.RagTraceNode{
		NodeID: "session_recall",
		ExtraData: `{
			"candidateCount": 4,
			"excerptCount": 1,
			"recallFingerprint": "fp-1"
		}`,
	})
	if summary != nil {
		t.Fatalf("expected no long-term memory summary, got %+v", summary)
	}
}
