package writeback

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/app/rag/service/longtermmemory/extraction"
	fwlog "local/rag-project/internal/framework/log"
)

type extractorStub struct {
	result    extraction.ExtractResult
	panicWith any
	calls     int
	lastInput extraction.ExtractInput
}

func (s *extractorStub) Extract(input extraction.ExtractInput) extraction.ExtractResult {
	s.calls++
	s.lastInput = input
	if s.panicWith != nil {
		panic(s.panicWith)
	}
	return s.result
}

type persisterStub struct {
	ok        bool
	calls     int
	lastCtx   context.Context
	lastInput longtermmemory.PersistPreferenceCandidateInput
}

func (s *persisterStub) TryPersistPendingPreferenceCandidate(ctx context.Context, input longtermmemory.PersistPreferenceCandidateInput) (longtermmemory.PreferenceCandidate, bool) {
	s.calls++
	s.lastCtx = ctx
	s.lastInput = input
	if !s.ok {
		return longtermmemory.PreferenceCandidate{}, false
	}
	candidate := input.Candidate
	candidate.ID = "cand-1"
	candidate.Status = domain.MemoryStatusPending
	return candidate, true
}

func TestCapturePreferenceCandidateSyncSkipsOneOffWithoutTrigger(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil)

	result := service.capturePreferenceCandidateSync(context.Background(), Input{
		UserID:          "user-1",
		Message:         "17*23",
		SourceMessageID: "msg-1",
	})

	if !result.Skipped {
		t.Fatalf("expected skipped result, got %+v", result)
	}
	if result.SkipReason != extraction.SkipReasonCalculation {
		t.Fatalf("expected calculation skip reason, got %+v", result)
	}
	if result.Failed || result.Rejected || result.Persisted {
		t.Fatalf("expected pure skip result, got %+v", result)
	}
}

func TestCapturePreferenceCandidateSyncPersistsPendingCandidate(t *testing.T) {
	t.Parallel()

	extractor := &extractorStub{
		result: extraction.ExtractResult{
			Candidate: &extraction.StructuredPreferenceCandidate{
				ScopeType:    domain.MemoryScopeGlobal,
				MemoryType:   domain.MemoryTypePreference,
				CanonicalKey: "response.language",
				Summary:      "Answer in Chinese by default.",
				Content:      "Use Chinese by default.",
				Confidence:   0.94,
			},
		},
	}
	persister := &persisterStub{ok: true}
	service := NewService(extractor, persister, nil)

	result := service.capturePreferenceCandidateSync(context.Background(), Input{
		UserID:          "user-1",
		Message:         "Please answer in Chinese by default from now on.",
		SourceMessageID: "msg-1",
	})

	if !result.Persisted {
		t.Fatalf("expected persisted result, got %+v", result)
	}
	if result.Candidate == nil {
		t.Fatalf("expected persisted candidate, got %+v", result)
	}
	if extractor.calls != 1 {
		t.Fatalf("expected one extraction call, got %d", extractor.calls)
	}
	if persister.calls != 1 {
		t.Fatalf("expected one persist call, got %d", persister.calls)
	}
	if persister.lastInput.UserID != "user-1" {
		t.Fatalf("expected user id to be forwarded, got %+v", persister.lastInput)
	}
	if persister.lastInput.Candidate.CanonicalKey != "response.language" {
		t.Fatalf("expected canonical key to be forwarded, got %+v", persister.lastInput.Candidate)
	}
	if persister.lastInput.Candidate.SourceMessageID != "msg-1" {
		t.Fatalf("expected source message id to be forwarded, got %+v", persister.lastInput.Candidate)
	}
	if persister.lastInput.Candidate.ExtractionMethod != domain.MemoryExtractionMethodLLM {
		t.Fatalf("expected llm extraction method, got %+v", persister.lastInput.Candidate)
	}
}

func TestCapturePreferenceCandidateSyncReturnsExtractorFailureWithoutPersisting(t *testing.T) {
	t.Parallel()

	extractor := &extractorStub{
		result: extraction.ExtractResult{
			Failed:        true,
			FailureReason: extraction.FailureReasonInvalidJSON,
		},
	}
	persister := &persisterStub{ok: true}
	service := NewService(extractor, persister, nil)

	result := service.capturePreferenceCandidateSync(context.Background(), Input{
		UserID:          "user-1",
		Message:         "Please answer in Chinese by default from now on.",
		SourceMessageID: "msg-1",
	})

	if !result.Failed {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if result.FailureReason != extraction.FailureReasonInvalidJSON {
		t.Fatalf("expected extraction failure reason, got %+v", result)
	}
	if persister.calls != 0 {
		t.Fatalf("expected no persistence attempt, got %d", persister.calls)
	}
}

func TestCapturePreferenceCandidateSyncReturnsPostFilterRejectionWithoutPersisting(t *testing.T) {
	t.Parallel()

	extractor := &extractorStub{
		result: extraction.ExtractResult{
			Candidate: &extraction.StructuredPreferenceCandidate{
				ScopeType:    domain.MemoryScopeGlobal,
				MemoryType:   domain.MemoryTypePreference,
				CanonicalKey: "response.language",
				Summary:      "Answer in Chinese by default.",
				Content:      "Use Chinese by default.",
				Confidence:   0.79,
			},
		},
	}
	persister := &persisterStub{ok: true}
	service := NewService(extractor, persister, nil)

	result := service.capturePreferenceCandidateSync(context.Background(), Input{
		UserID:          "user-1",
		Message:         "Please answer in Chinese by default from now on.",
		SourceMessageID: "msg-1",
	})

	if !result.Rejected {
		t.Fatalf("expected rejected result, got %+v", result)
	}
	if result.RejectionReason != extraction.RejectionReasonLowConfidence {
		t.Fatalf("expected low confidence rejection, got %+v", result)
	}
	if persister.calls != 0 {
		t.Fatalf("expected no persistence attempt, got %d", persister.calls)
	}
}

func TestCapturePreferenceCandidateSyncReturnsPersistenceFailure(t *testing.T) {
	t.Parallel()

	extractor := &extractorStub{
		result: extraction.ExtractResult{
			Candidate: &extraction.StructuredPreferenceCandidate{
				ScopeType:    domain.MemoryScopeGlobal,
				MemoryType:   domain.MemoryTypePreference,
				CanonicalKey: "behavior.avoid",
				Summary:      "Avoid large refactors first.",
				Content:      "Do not jump into large refactors first.",
				Confidence:   0.91,
			},
		},
	}
	persister := &persisterStub{ok: false}
	service := NewService(extractor, persister, nil)

	result := service.capturePreferenceCandidateSync(context.Background(), Input{
		UserID:          "user-1",
		Message:         "Please do not jump into large refactors first.",
		SourceMessageID: "msg-1",
	})

	if !result.Failed {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if result.FailureReason != FailureReasonPersistFailed {
		t.Fatalf("expected persistence failure reason, got %+v", result)
	}
	if result.Persisted {
		t.Fatalf("expected persistence failure, got %+v", result)
	}
}

func TestCapturePreferenceCandidateFailOpenOnPanic(t *testing.T) {
	t.Parallel()

	service := NewService(&extractorStub{panicWith: "boom"}, &persisterStub{ok: true}, nil)

	service.CapturePreferenceCandidate(context.Background(), Input{
		UserID:          "user-1",
		Message:         "Please answer in Chinese by default from now on.",
		SourceMessageID: "msg-1",
	})
}

func TestCapturePreferenceCandidateSyncLogsSkipAndPersistence(t *testing.T) {
	t.Parallel()

	t.Run("skip", func(t *testing.T) {
		core, observed := observer.New(zap.InfoLevel)
		ctx := fwlog.BindLogger(context.Background(), zap.New(core).Sugar())
		service := NewService(nil, nil, nil)

		result := service.capturePreferenceCandidateSync(ctx, Input{
			UserID:          "user-1",
			Message:         "17*23",
			SourceMessageID: "msg-1",
		})
		if !result.Skipped {
			t.Fatalf("expected skipped result, got %+v", result)
		}

		entries := observed.All()
		if len(entries) != 2 {
			t.Fatalf("expected 2 log entries, got %d", len(entries))
		}
		if entries[0].Message != "long-term memory writeback started" {
			t.Fatalf("unexpected start log: %+v", entries[0])
		}
		if entries[1].Message != "long-term memory writeback skipped" {
			t.Fatalf("unexpected skip log: %+v", entries[1])
		}
		if entries[1].ContextMap()["skip_reason"] != extraction.SkipReasonCalculation {
			t.Fatalf("unexpected skip log context: %+v", entries[1].ContextMap())
		}
	})

	t.Run("persisted", func(t *testing.T) {
		core, observed := observer.New(zap.InfoLevel)
		ctx := fwlog.BindLogger(context.Background(), zap.New(core).Sugar())
		extractor := &extractorStub{
			result: extraction.ExtractResult{
				Candidate: &extraction.StructuredPreferenceCandidate{
					ScopeType:    domain.MemoryScopeGlobal,
					MemoryType:   domain.MemoryTypePreference,
					CanonicalKey: "response.language",
					Summary:      "Answer in Chinese by default.",
					Content:      "Use Chinese by default.",
					Confidence:   0.94,
				},
			},
		}
		persister := &persisterStub{ok: true}
		service := NewService(extractor, persister, nil)

		result := service.capturePreferenceCandidateSync(ctx, Input{
			UserID:          "user-1",
			Message:         "Please answer in Chinese by default from now on.",
			SourceMessageID: "msg-1",
		})
		if !result.Persisted {
			t.Fatalf("expected persisted result, got %+v", result)
		}

		entries := observed.All()
		if len(entries) != 3 {
			t.Fatalf("expected 3 log entries, got %d", len(entries))
		}
		if entries[1].Message != "long-term memory candidate extracted" {
			t.Fatalf("unexpected extracted log: %+v", entries[1])
		}
		if entries[2].Message != "long-term memory candidate persisted" {
			t.Fatalf("unexpected persisted log: %+v", entries[2])
		}
		contextMap := entries[2].ContextMap()
		if contextMap["candidate_id"] != "cand-1" || contextMap["canonical_key"] != "response.language" {
			t.Fatalf("unexpected persisted log context: %+v", contextMap)
		}
	})
}
