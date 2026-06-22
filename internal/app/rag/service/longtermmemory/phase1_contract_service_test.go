package longtermmemory

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestPreferenceCandidateContractServiceConfirmRequiresActiveResult(t *testing.T) {
	t.Parallel()

	service := NewPreferenceCandidateContractService(preferenceCandidateContractServiceStub{
		confirmFn: func(context.Context, DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
			return validContractPreferenceCandidate(domain.MemoryStatusPending), nil
		},
	})

	_, err := service.ConfirmPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-1",
	})
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("expected active status contract error, got %v", err)
	}
}

func TestPreferenceCandidateContractServiceRejectRequiresRejectedResult(t *testing.T) {
	t.Parallel()

	service := NewPreferenceCandidateContractService(preferenceCandidateContractServiceStub{
		rejectFn: func(context.Context, DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
			return validContractPreferenceCandidate(domain.MemoryStatusActive), nil
		},
	})

	_, err := service.RejectPreferenceCandidate(context.Background(), DecidePreferenceCandidateInput{
		UserID:      "user-1",
		CandidateID: "cand-1",
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejected status contract error, got %v", err)
	}
}

type preferenceCandidateContractServiceStub struct {
	listFn    func(context.Context, ListPreferenceCandidatesInput) (PreferenceCandidatePageResult, error)
	confirmFn func(context.Context, DecidePreferenceCandidateInput) (PreferenceCandidate, error)
	rejectFn  func(context.Context, DecidePreferenceCandidateInput) (PreferenceCandidate, error)
}

func (s preferenceCandidateContractServiceStub) ListPendingPreferenceCandidates(ctx context.Context, input ListPreferenceCandidatesInput) (PreferenceCandidatePageResult, error) {
	if s.listFn != nil {
		return s.listFn(ctx, input)
	}
	return PreferenceCandidatePageResult{}, nil
}

func (s preferenceCandidateContractServiceStub) ConfirmPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	if s.confirmFn != nil {
		return s.confirmFn(ctx, input)
	}
	return PreferenceCandidate{}, nil
}

func (s preferenceCandidateContractServiceStub) RejectPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	if s.rejectFn != nil {
		return s.rejectFn(ctx, input)
	}
	return PreferenceCandidate{}, nil
}

func validContractPreferenceCandidate(status string) PreferenceCandidate {
	return PreferenceCandidate{
		ID:               "cand-1",
		ScopeType:        domain.MemoryScopeGlobal,
		MemoryType:       domain.MemoryTypePreference,
		CanonicalKey:     "response.language",
		Summary:          "以后默认用中文回答",
		Content:          "以后默认用中文回答",
		SourceMessageID:  "msg-1",
		ExtractionMethod: domain.MemoryExtractionMethodLLM,
		Confidence:       0.94,
		Status:           status,
	}
}
