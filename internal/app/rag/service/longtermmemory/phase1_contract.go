package longtermmemory

import (
	"context"
	"sort"
	"strings"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/exception"
)

const (
	DefaultPreferenceCandidatePageSize = 20
	MaxPreferenceCandidatePageSize     = 100
)

var phase1PreferenceCanonicalKeys = map[string]struct{}{
	"behavior.avoid":                      {},
	"response.language":                   {},
	"workflow.troubleshooting.first_step": {},
}

type PreferenceCandidate struct {
	ID               string  `json:"id,omitempty"`
	ScopeType        string  `json:"scopeType"`
	MemoryType       string  `json:"memoryType"`
	CanonicalKey     string  `json:"canonicalKey"`
	Summary          string  `json:"summary"`
	Content          string  `json:"content"`
	SourceMessageID  string  `json:"sourceMessageId"`
	ExtractionMethod string  `json:"extractionMethod"`
	Confidence       float64 `json:"confidence"`
	Status           string  `json:"status"`
}

type ListPreferenceCandidatesInput struct {
	UserID   string
	Page     int
	PageSize int
}

type PreferenceCandidatePageResult struct {
	Items    []PreferenceCandidate
	Total    int
	Page     int
	PageSize int
}

type DecidePreferenceCandidateInput struct {
	UserID      string
	CandidateID string
}

type PreferenceCandidateService interface {
	ListPendingPreferenceCandidates(ctx context.Context, input ListPreferenceCandidatesInput) (PreferenceCandidatePageResult, error)
	ConfirmPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error)
	RejectPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error)
}

type PreferenceCandidateContractService struct {
	delegate PreferenceCandidateService
}

func NewPreferenceCandidateContractService(delegate PreferenceCandidateService) *PreferenceCandidateContractService {
	return &PreferenceCandidateContractService{delegate: delegate}
}

func Phase1PreferenceCanonicalKeys() []string {
	keys := make([]string, 0, len(phase1PreferenceCanonicalKeys))
	for key := range phase1PreferenceCanonicalKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func NormalizePendingPreferenceCandidate(candidate PreferenceCandidate) (PreferenceCandidate, error) {
	normalized := candidate
	normalized.ScopeType = normalizeCandidateScopeType(normalized.ScopeType)
	normalized.MemoryType = normalizeCandidateMemoryType(normalized.MemoryType)
	normalized.CanonicalKey = normalizeCandidateCanonicalKey(normalized.CanonicalKey)
	normalized.Summary = strings.TrimSpace(normalized.Summary)
	normalized.Content = strings.TrimSpace(normalized.Content)
	normalized.SourceMessageID = strings.TrimSpace(normalized.SourceMessageID)
	normalized.ExtractionMethod = strings.TrimSpace(normalized.ExtractionMethod)
	normalized.Status = normalizeCandidateStatus(normalized.Status, domain.MemoryStatusPending)
	if err := validatePreferenceCandidate(normalized, map[string]struct{}{
		domain.MemoryStatusPending: {},
	}); err != nil {
		return PreferenceCandidate{}, err
	}
	return normalized, nil
}

func ValidatePreferenceCandidate(candidate PreferenceCandidate) error {
	return validatePreferenceCandidate(candidate, map[string]struct{}{
		domain.MemoryStatusPending:  {},
		domain.MemoryStatusActive:   {},
		domain.MemoryStatusRejected: {},
	})
}

func (s *PreferenceCandidateContractService) ListPendingPreferenceCandidates(ctx context.Context, input ListPreferenceCandidatesInput) (PreferenceCandidatePageResult, error) {
	normalized, err := normalizeListPreferenceCandidatesInput(input)
	if err != nil {
		return PreferenceCandidatePageResult{}, err
	}
	if s == nil || s.delegate == nil {
		return PreferenceCandidatePageResult{
			Items:    []PreferenceCandidate{},
			Total:    0,
			Page:     normalized.Page,
			PageSize: normalized.PageSize,
		}, nil
	}
	result, err := s.delegate.ListPendingPreferenceCandidates(ctx, normalized)
	if err != nil {
		return PreferenceCandidatePageResult{}, err
	}
	result.Page = normalized.Page
	result.PageSize = normalized.PageSize
	for _, item := range result.Items {
		if err := ValidatePreferenceCandidate(item); err != nil {
			return PreferenceCandidatePageResult{}, err
		}
		if strings.TrimSpace(item.Status) != domain.MemoryStatusPending {
			return PreferenceCandidatePageResult{}, exception.NewServiceException("pending preference candidate list returned non-pending status", nil)
		}
	}
	return result, nil
}

func (s *PreferenceCandidateContractService) ConfirmPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	candidate, err := s.requireDecisionCandidate(ctx, input, true)
	if err != nil {
		return PreferenceCandidate{}, err
	}
	if strings.TrimSpace(candidate.Status) != domain.MemoryStatusActive {
		return PreferenceCandidate{}, exception.NewServiceException("confirmed preference candidate must return active status", nil)
	}
	return candidate, nil
}

func (s *PreferenceCandidateContractService) RejectPreferenceCandidate(ctx context.Context, input DecidePreferenceCandidateInput) (PreferenceCandidate, error) {
	candidate, err := s.requireDecisionCandidate(ctx, input, false)
	if err != nil {
		return PreferenceCandidate{}, err
	}
	if strings.TrimSpace(candidate.Status) != domain.MemoryStatusRejected {
		return PreferenceCandidate{}, exception.NewServiceException("rejected preference candidate must return rejected status", nil)
	}
	return candidate, nil
}

func (s *PreferenceCandidateContractService) requireDecisionCandidate(ctx context.Context, input DecidePreferenceCandidateInput, approve bool) (PreferenceCandidate, error) {
	normalized, err := normalizeDecidePreferenceCandidateInput(input)
	if err != nil {
		return PreferenceCandidate{}, err
	}
	if s == nil || s.delegate == nil {
		return PreferenceCandidate{}, exception.NewServiceException("preference candidate lifecycle is not configured", nil)
	}
	var candidate PreferenceCandidate
	if approve {
		candidate, err = s.delegate.ConfirmPreferenceCandidate(ctx, normalized)
	} else {
		candidate, err = s.delegate.RejectPreferenceCandidate(ctx, normalized)
	}
	if err != nil {
		return PreferenceCandidate{}, err
	}
	if err := ValidatePreferenceCandidate(candidate); err != nil {
		return PreferenceCandidate{}, err
	}
	return candidate, nil
}

func normalizeListPreferenceCandidatesInput(input ListPreferenceCandidatesInput) (ListPreferenceCandidatesInput, error) {
	normalized := input
	normalized.UserID = strings.TrimSpace(normalized.UserID)
	if normalized.UserID == "" {
		return ListPreferenceCandidatesInput{}, exception.NewClientException("user id is required", nil)
	}
	if normalized.Page <= 0 {
		normalized.Page = 1
	}
	if normalized.PageSize <= 0 {
		normalized.PageSize = DefaultPreferenceCandidatePageSize
	}
	if normalized.PageSize > MaxPreferenceCandidatePageSize {
		normalized.PageSize = MaxPreferenceCandidatePageSize
	}
	return normalized, nil
}

func normalizeDecidePreferenceCandidateInput(input DecidePreferenceCandidateInput) (DecidePreferenceCandidateInput, error) {
	normalized := input
	normalized.UserID = strings.TrimSpace(normalized.UserID)
	normalized.CandidateID = strings.TrimSpace(normalized.CandidateID)
	if normalized.UserID == "" {
		return DecidePreferenceCandidateInput{}, exception.NewClientException("user id is required", nil)
	}
	if normalized.CandidateID == "" {
		return DecidePreferenceCandidateInput{}, exception.NewClientException("preference candidate id is required", nil)
	}
	return normalized, nil
}

func validatePreferenceCandidate(candidate PreferenceCandidate, allowedStatuses map[string]struct{}) error {
	scopeType := normalizeCandidateScopeType(candidate.ScopeType)
	if scopeType != domain.MemoryScopeGlobal {
		return exception.NewClientException("phase 1 preference candidate requires global scope", nil)
	}
	memoryType := normalizeCandidateMemoryType(candidate.MemoryType)
	if memoryType != domain.MemoryTypePreference {
		return exception.NewClientException("phase 1 preference candidate requires preference memory type", nil)
	}
	canonicalKey := normalizeCandidateCanonicalKey(candidate.CanonicalKey)
	if _, ok := phase1PreferenceCanonicalKeys[canonicalKey]; !ok {
		return exception.NewClientException("unsupported phase 1 preference canonical key", nil)
	}
	status := normalizeCandidateStatus(candidate.Status, "")
	if _, ok := allowedStatuses[status]; !ok {
		return exception.NewClientException("unsupported phase 1 preference candidate status", nil)
	}
	if strings.TrimSpace(candidate.Summary) == "" {
		return exception.NewClientException("preference candidate summary is required", nil)
	}
	if strings.TrimSpace(candidate.Content) == "" {
		return exception.NewClientException("preference candidate content is required", nil)
	}
	if strings.TrimSpace(candidate.SourceMessageID) == "" {
		return exception.NewClientException("preference candidate source message id is required", nil)
	}
	if strings.TrimSpace(candidate.ExtractionMethod) == "" {
		return exception.NewClientException("preference candidate extraction method is required", nil)
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		return exception.NewClientException("preference candidate confidence must be between 0 and 1", nil)
	}
	return nil
}

func normalizeCandidateScopeType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryScopeGlobal
	}
	return value
}

func normalizeCandidateMemoryType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return domain.MemoryTypePreference
	}
	return value
}

func normalizeCandidateCanonicalKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeCandidateStatus(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}
