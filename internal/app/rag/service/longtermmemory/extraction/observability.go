package extraction

import (
	"local/rag-project/internal/app/rag/cachemetrics"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func EvaluateObservedPreFilter(metrics *cachemetrics.Service, input PreFilterInput) PreFilterResult {
	result := EvaluatePreFilter(input)
	if result.Skip {
		longtermmemoryobs.Record(metrics, longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomePreFilterSkipped)
	}
	return result
}

type ObservedLLMPreferenceExtractor struct {
	inner   *LLMPreferenceExtractor
	metrics *cachemetrics.Service
}

func NewObservedLLMPreferenceExtractor(chatService aichat.LLMService, metrics *cachemetrics.Service) *ObservedLLMPreferenceExtractor {
	return &ObservedLLMPreferenceExtractor{
		inner:   NewLLMPreferenceExtractor(chatService),
		metrics: metrics,
	}
}

func (e *ObservedLLMPreferenceExtractor) Extract(input ExtractInput) ExtractResult {
	if e != nil {
		longtermmemoryobs.Record(e.metrics, longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomeExtractionAttempted)
	}
	if e == nil || e.inner == nil {
		return ExtractResult{
			Failed:        true,
			FailureReason: FailureReasonLLMCall,
		}
	}
	return e.inner.Extract(input)
}

func ApplyObservedPreferencePostFilter(metrics *cachemetrics.Service, candidate StructuredPreferenceCandidate) PostFilterResult {
	result := ApplyPreferencePostFilter(candidate)
	if result.Rejected {
		longtermmemoryobs.Record(metrics, longtermmemoryobs.LayerExtraction, longtermmemoryobs.OutcomePostFilterRejected)
	}
	return result
}
