package writeback

import (
	"context"
	"strings"

	"local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/app/rag/service/longtermmemory/extraction"
	longtermmemoryobs "local/rag-project/internal/app/rag/service/longtermmemory/observability"
	"local/rag-project/internal/framework/log"
)

const (
	FailureReasonNone             = ""
	FailureReasonDependencyMissed = "dependency_missing"
	FailureReasonPersistFailed    = "persist_failed"
)

type Extractor interface {
	Extract(input extraction.ExtractInput) extraction.ExtractResult
}

type LifecyclePersister interface {
	TryPersistPendingPreferenceCandidate(ctx context.Context, input longtermmemory.PersistPreferenceCandidateInput) (longtermmemory.PreferenceCandidate, bool)
}

type Input struct {
	UserID          string
	Message         string
	SourceMessageID string
}

type CaptureResult struct {
	Candidate       *longtermmemory.PreferenceCandidate
	Skipped         bool
	SkipReason      string
	Rejected        bool
	RejectionReason string
	Failed          bool
	FailureReason   string
	Persisted       bool
}

type Service struct {
	extractor Extractor
	persister LifecyclePersister
	metrics   *cachemetrics.Service
}

func NewService(extractor Extractor, persister LifecyclePersister, metrics *cachemetrics.Service) *Service {
	return &Service{
		extractor: extractor,
		persister: persister,
		metrics:   metrics,
	}
}

func (s *Service) CapturePreferenceCandidate(ctx context.Context, input Input) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.FromContext(ctx).Warnw(
				"long-term memory writeback panicked",
				"subsystem", "long_term_memory",
				"user_id", strings.TrimSpace(input.UserID),
				"source_message_id", strings.TrimSpace(input.SourceMessageID),
				"recovered", recovered,
			)
		}
	}()

	_ = s.capturePreferenceCandidateSync(ctx, input)
}

func (s *Service) capturePreferenceCandidateSync(ctx context.Context, input Input) CaptureResult {
	userID := strings.TrimSpace(input.UserID)
	sourceMessageID := strings.TrimSpace(input.SourceMessageID)
	message := strings.TrimSpace(input.Message)
	longtermmemoryobs.LogWritebackStarted(ctx, userID, sourceMessageID, len(message))

	var metrics *cachemetrics.Service
	if s != nil {
		metrics = s.metrics
	}

	preFilter := extraction.EvaluateObservedPreFilter(metrics, extraction.PreFilterInput{
		Message: message,
	})
	if preFilter.Skip {
		longtermmemoryobs.LogWritebackSkipped(ctx, userID, sourceMessageID, preFilter.SkipReason)
		return CaptureResult{
			Skipped:    true,
			SkipReason: preFilter.SkipReason,
		}
	}

	if s == nil || s.extractor == nil || s.persister == nil {
		longtermmemoryobs.LogWritebackFailed(ctx, userID, sourceMessageID, FailureReasonDependencyMissed)
		return CaptureResult{
			Failed:        true,
			FailureReason: FailureReasonDependencyMissed,
		}
	}

	extracted := s.extractor.Extract(extraction.ExtractInput{
		Message: message,
	})
	if extracted.Failed {
		longtermmemoryobs.LogWritebackFailed(ctx, userID, sourceMessageID, extracted.FailureReason)
		return CaptureResult{
			Failed:        true,
			FailureReason: extracted.FailureReason,
		}
	}
	if extracted.Rejected {
		longtermmemoryobs.LogWritebackRejected(ctx, userID, sourceMessageID, "", extracted.RejectionReason, 0)
		return CaptureResult{
			Rejected:        true,
			RejectionReason: extracted.RejectionReason,
		}
	}
	if extracted.Candidate == nil {
		longtermmemoryobs.LogWritebackFailed(ctx, userID, sourceMessageID, FailureReasonDependencyMissed)
		return CaptureResult{
			Failed:        true,
			FailureReason: FailureReasonDependencyMissed,
		}
	}
	longtermmemoryobs.LogWritebackExtracted(ctx, userID, sourceMessageID, extracted.Candidate.CanonicalKey, extracted.Candidate.Confidence)

	postFilter := extraction.ApplyObservedPreferencePostFilter(s.metrics, *extracted.Candidate)
	if postFilter.Rejected {
		longtermmemoryobs.LogWritebackRejected(
			ctx,
			userID,
			sourceMessageID,
			extracted.Candidate.CanonicalKey,
			postFilter.RejectionReason,
			extracted.Candidate.Confidence,
		)
		return CaptureResult{
			Rejected:        true,
			RejectionReason: postFilter.RejectionReason,
		}
	}
	if postFilter.Candidate == nil {
		longtermmemoryobs.LogWritebackFailed(ctx, userID, sourceMessageID, FailureReasonDependencyMissed)
		return CaptureResult{
			Failed:        true,
			FailureReason: FailureReasonDependencyMissed,
		}
	}

	candidate := longtermmemory.PreferenceCandidate{
		ScopeType:        postFilter.Candidate.ScopeType,
		MemoryType:       postFilter.Candidate.MemoryType,
		CanonicalKey:     postFilter.Candidate.CanonicalKey,
		Summary:          postFilter.Candidate.Summary,
		Content:          postFilter.Candidate.Content,
		SourceMessageID:  sourceMessageID,
		ExtractionMethod: domain.MemoryExtractionMethodLLM,
		Confidence:       postFilter.Candidate.Confidence,
		Status:           domain.MemoryStatusPending,
	}
	persistedCandidate, ok := s.persister.TryPersistPendingPreferenceCandidate(ctx, longtermmemory.PersistPreferenceCandidateInput{
		UserID:    userID,
		Candidate: candidate,
	})
	if !ok {
		longtermmemoryobs.LogWritebackFailed(ctx, userID, sourceMessageID, FailureReasonPersistFailed)
		return CaptureResult{
			Candidate:     &candidate,
			Failed:        true,
			FailureReason: FailureReasonPersistFailed,
		}
	}
	longtermmemoryobs.LogWritebackPersisted(
		ctx,
		userID,
		sourceMessageID,
		persistedCandidate.ID,
		persistedCandidate.CanonicalKey,
		persistedCandidate.Confidence,
	)

	return CaptureResult{
		Candidate: &persistedCandidate,
		Persisted: true,
	}
}
