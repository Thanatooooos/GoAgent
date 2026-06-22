package extraction

import (
	"testing"

	"local/rag-project/internal/app/rag/cachemetrics"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
)

func TestEvaluateObservedPreFilterRecordsSkipEvent(t *testing.T) {
	t.Parallel()

	metrics := cachemetrics.NewService()

	result := EvaluateObservedPreFilter(metrics, PreFilterInput{
		Message: "hello",
	})

	if !result.Skip {
		t.Fatalf("expected pre-filter skip result, got %+v", result)
	}
	assertMetricsEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomePreFilterSkipped, 1)
}

func TestObservedLLMPreferenceExtractorRecordsAttemptEvent(t *testing.T) {
	t.Parallel()

	metrics := cachemetrics.NewService()
	extractor := NewObservedLLMPreferenceExtractor(&stubLLMService{
		response: `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"default to Chinese","content":"Chinese","confidence":0.94}`,
	}, metrics)

	result := extractor.Extract(ExtractInput{Message: "以后默认用中文回答"})

	if result.Candidate == nil {
		t.Fatalf("expected candidate, got %+v", result)
	}
	assertMetricsEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomeExtractionAttempted, 1)
}

func TestApplyObservedPreferencePostFilterRecordsRejectEvent(t *testing.T) {
	t.Parallel()

	metrics := cachemetrics.NewService()

	result := ApplyObservedPreferencePostFilter(metrics, StructuredPreferenceCandidate{
		ScopeType:    "global",
		MemoryType:   "preference",
		CanonicalKey: "response.language",
		Summary:      "today use Chinese",
		Content:      "today use Chinese",
		Confidence:   0.79,
	})

	if !result.Rejected {
		t.Fatalf("expected rejected post-filter result, got %+v", result)
	}
	assertMetricsEventCount(t, metrics.Snapshot(), longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomePostFilterRejected, 1)
}

func assertMetricsEventCount(t *testing.T, snapshot cachemetrics.MetricsSnapshot, layer string, outcome string, want int64) {
	t.Helper()

	for _, event := range snapshot.Events {
		if event.CacheKind != longtermmemoryobs.CacheKindLongTermMemory {
			continue
		}
		if event.Layer == layer && event.Outcome == outcome {
			if event.Count != want {
				t.Fatalf("event %s/%s count=%d want=%d snapshot=%+v", layer, outcome, event.Count, want, snapshot.Events)
			}
			return
		}
	}
	if want == 0 {
		return
	}
	t.Fatalf("missing event %s/%s in snapshot=%+v", layer, outcome, snapshot.Events)
}
