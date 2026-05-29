package builtin

import (
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/traceinsight"
)

type traceLongTermMemorySummary = traceinsight.LongTermMemorySummary
type traceSessionRecallSummary = traceinsight.SessionRecallSummary

func appendTraceMemoryEvidence(evidence []string, longTerm *traceLongTermMemorySummary, session *traceSessionRecallSummary) []string {
	if longTerm != nil {
		evidence = append(evidence,
			fmt.Sprintf("longTermMemory.selectedCount=%d", longTerm.SelectedCount),
			fmt.Sprintf("longTermMemory.candidateCount=%d", longTerm.CandidateCount),
			fmt.Sprintf("longTermMemory.ruleCount=%d", longTerm.RuleCount),
			fmt.Sprintf("longTermMemory.factSelectedCount=%d", longTerm.FactSelectedCount),
			fmt.Sprintf("longTermMemory.factCandidateCount=%d", longTerm.FactCandidateCount),
		)
		if longTerm.RuleCacheLayer != "" {
			evidence = append(evidence, fmt.Sprintf("longTermMemory.ruleCacheLayer=%s", longTerm.RuleCacheLayer))
		}
		if longTerm.FactCacheLayer != "" {
			evidence = append(evidence, fmt.Sprintf("longTermMemory.factCacheLayer=%s", longTerm.FactCacheLayer))
		}
		if longTerm.EmbeddingCacheLayer != "" {
			evidence = append(evidence, fmt.Sprintf("longTermMemory.embeddingCacheLayer=%s", longTerm.EmbeddingCacheLayer))
		}
		if longTerm.RecomputeReason != "" {
			evidence = append(evidence, fmt.Sprintf("longTermMemory.recomputeReason=%s", longTerm.RecomputeReason))
		}
		if longTerm.Truncated {
			evidence = append(evidence, "longTermMemory.truncated=true")
		}
	}
	if session != nil {
		evidence = append(evidence,
			fmt.Sprintf("sessionRecall.excerptCount=%d", session.ExcerptCount),
			fmt.Sprintf("sessionRecall.candidateCount=%d", session.CandidateCount),
		)
		if session.TopScore > 0 {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.topScore=%.4f", session.TopScore))
		}
		if session.CacheLayer != "" {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.cacheLayer=%s", session.CacheLayer))
		}
		if session.EmbeddingCacheLayer != "" {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.embeddingCacheLayer=%s", session.EmbeddingCacheLayer))
		}
		if session.RecomputeReason != "" {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.recomputeReason=%s", session.RecomputeReason))
		}
		if session.TruncatedBy != "" {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.truncatedBy=%s", session.TruncatedBy))
		}
		if session.SkippedPerMessageLimit > 0 {
			evidence = append(evidence, fmt.Sprintf("sessionRecall.skippedPerMessageLimit=%d", session.SkippedPerMessageLimit))
		}
	}
	return evidence
}

func traceMemoryRecallSelectedCount(longTerm *traceLongTermMemorySummary, session *traceSessionRecallSummary) int {
	total := 0
	if longTerm != nil {
		total += longTerm.SelectedCount
	}
	if session != nil {
		total += session.ExcerptCount
	}
	return total
}

func hasDegradedMemoryTrace(longTerm *traceLongTermMemorySummary, session *traceSessionRecallSummary) bool {
	return hasDegradedLongTermMemoryTrace(longTerm) || hasDegradedSessionRecallTrace(session)
}

func degradedMemoryTraceNodeID(longTerm *traceLongTermMemorySummary, session *traceSessionRecallSummary) string {
	if hasDegradedSessionRecallTrace(session) {
		return "session_recall"
	}
	if hasDegradedLongTermMemoryTrace(longTerm) {
		return "long_term_memory"
	}
	return ""
}

func hasDegradedLongTermMemoryTrace(summary *traceLongTermMemorySummary) bool {
	return summary != nil && summary.IsDegraded()
}

func hasDegradedSessionRecallTrace(summary *traceSessionRecallSummary) bool {
	return summary != nil && summary.IsDegraded()
}

func renderTraceCountSummary(counts map[string]int, keys []string) string {
	return traceinsight.RenderCountSummary(counts, keys)
}

func renderTraceMemoryCacheSummary(ruleLayer string, factLayer string, embeddingLayer string) string {
	return traceinsight.RenderMemoryCacheSummary(ruleLayer, factLayer, embeddingLayer)
}

func renderTraceSessionCacheSummary(cacheLayer string, embeddingLayer string) string {
	return traceinsight.RenderSessionCacheSummary(cacheLayer, embeddingLayer)
}

func normalizeTraceNodeID(value string) string {
	return strings.TrimSpace(value)
}
