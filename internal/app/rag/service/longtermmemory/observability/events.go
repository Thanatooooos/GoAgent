package observability

import "local/rag-project/internal/app/rag/cachemetrics"

const (
	CacheKindLongTermMemory = "long_term_memory"

	LayerExtraction = "extraction"
	LayerLifecycle  = "lifecycle"
	LayerRecall     = "recall"

	OutcomePreFilterSkipped              = "prefilter_skipped"
	OutcomeExtractionAttempted           = "extraction_attempted"
	OutcomePostFilterRejected            = "postfilter_rejected"
	OutcomePendingPersisted              = "pending_persisted"
	OutcomeCandidateConfirmed            = "candidate_confirmed"
	OutcomeCandidateRejected             = "candidate_rejected"
	OutcomePreferenceRecalled            = "preference_recalled"
	OutcomeRecallOverriddenByCurrentTurn = "recall_overridden_by_current_turn"
)

func Record(metrics *cachemetrics.Service, layer string, outcome string) {
	if metrics == nil {
		return
	}
	metrics.Record(CacheKindLongTermMemory, layer, outcome)
}
