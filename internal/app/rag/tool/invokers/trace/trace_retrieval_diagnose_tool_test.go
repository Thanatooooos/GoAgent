package builtin

import (
	"strings"
	"testing"

	ragdomain "local/rag-project/internal/app/rag/domain"
)

func TestDiagnoseTraceRetrievalUsesMemoryEvidenceWhenRetrieveIsEmpty(t *testing.T) {
	conclusion, confidence, evidence, suggestions, focusNode, focusReason := diagnoseTraceRetrieval(
		ragdomain.RagTraceRun{TraceID: "trace-1", Status: "success"},
		[]ragdomain.RagTraceNode{
			{
				NodeID:   "retrieve",
				NodeType: "retrieve",
				Status:   "success",
				ExtraData: `{
					"chunkCount": 0,
					"topScore": 0,
					"searchMode": "hybrid"
				}`,
			},
			{
				NodeID:   "long_term_memory",
				NodeType: "memory",
				Status:   "success",
				ExtraData: `{
					"candidateCount": 3,
					"selectedCount": 2,
					"ruleCount": 1,
					"factCandidateCount": 2,
					"factSelectedCount": 1,
					"ruleCacheLayer": "request",
					"factCacheLayer": "redis"
				}`,
			},
		},
	)

	if confidence != diagnosisConfidenceMedium {
		t.Fatalf("expected medium confidence, got %q", confidence)
	}
	if !strings.Contains(conclusion, "memory recall still contributed prompt context") {
		t.Fatalf("expected memory-aware conclusion, got %q", conclusion)
	}
	if focusNode != "retrieve" || focusReason != "chunkCount=0 with memory recall context" {
		t.Fatalf("unexpected focus node/reason: %s %s", focusNode, focusReason)
	}
	if len(suggestions) == 0 {
		t.Fatalf("expected follow-up suggestions, got %+v", suggestions)
	}
	joinedEvidence := strings.Join(evidence, "\n")
	if !strings.Contains(joinedEvidence, "longTermMemory.selectedCount=2") {
		t.Fatalf("expected long-term memory evidence, got %+v", evidence)
	}
}

func TestDiagnoseTraceRetrievalDetectsDegradedMemoryTrace(t *testing.T) {
	conclusion, confidence, evidence, _, focusNode, focusReason := diagnoseTraceRetrieval(
		ragdomain.RagTraceRun{TraceID: "trace-1", Status: "success"},
		[]ragdomain.RagTraceNode{
			{
				NodeID:   "retrieve",
				NodeType: "retrieve",
				Status:   "success",
				ExtraData: `{
					"chunkCount": 5,
					"topScore": 0.82,
					"searchMode": "hybrid"
				}`,
			},
			{
				NodeID:   "session_recall",
				NodeType: "memory",
				Status:   "success",
				ExtraData: `{
					"candidateCount": 2,
					"excerptCount": 1,
					"topScore": 0.77,
					"cacheLayer": "fallback",
					"embeddingCacheLayer": "request",
					"recomputeReason": "fingerprint_unavailable;conversation_cache_miss"
				}`,
			},
		},
	)

	if confidence != diagnosisConfidenceMedium {
		t.Fatalf("expected medium confidence, got %q", confidence)
	}
	if !strings.Contains(conclusion, "degraded memory recall path") {
		t.Fatalf("expected degraded-memory conclusion, got %q", conclusion)
	}
	if focusNode != "session_recall" || focusReason != "memory recall degraded" {
		t.Fatalf("unexpected focus node/reason: %s %s", focusNode, focusReason)
	}
	if !strings.Contains(strings.Join(evidence, "\n"), "sessionRecall.cacheLayer=fallback") {
		t.Fatalf("expected session recall degradation evidence, got %+v", evidence)
	}
}
