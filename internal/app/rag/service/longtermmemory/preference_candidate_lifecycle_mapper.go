package longtermmemory

import (
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/service/longtermmemory/governance"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

const (
	preferenceCandidateSystemActor    = "system"
	behaviorAvoidActiveLimitPhase1    = 10
	preferenceCandidateQuotaExceeded  = "preference candidate quota exceeded"
)

type PersistPreferenceCandidateInput struct {
	UserID    string
	Candidate PreferenceCandidate
}

func normalizePersistPreferenceCandidateInput(input PersistPreferenceCandidateInput) (string, PreferenceCandidate, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return "", PreferenceCandidate{}, exception.NewClientException("user id is required", nil)
	}
	candidate, err := NormalizePendingPreferenceCandidate(input.Candidate)
	if err != nil {
		return "", PreferenceCandidate{}, err
	}
	return userID, candidate, nil
}

func buildPendingPreferenceMemoryItem(userID string, candidate PreferenceCandidate, now time.Time) (domain.MemoryItem, error) {
	saveInput := governance.NormalizeSaveExplicitMemoryInput(SaveExplicitMemoryInput{
		UserID:           userID,
		ScopeType:        candidate.ScopeType,
		MemoryType:       candidate.MemoryType,
		CanonicalKey:     candidate.CanonicalKey,
		SourceMessageID:  candidate.SourceMessageID,
		Content:          candidate.Content,
		Summary:          candidate.Summary,
		ExtractionMethod: candidate.ExtractionMethod,
	})
	if _, err := governance.EvaluateExplicitMemoryGate(saveInput); err != nil {
		return domain.MemoryItem{}, err
	}

	id, err := nextPreferenceCandidateID()
	if err != nil {
		return domain.MemoryItem{}, err
	}
	return domain.MemoryItem{
		ID:               id,
		UserID:           userID,
		ScopeType:        saveInput.ScopeType,
		ScopeID:          saveInput.ScopeID,
		Namespace:        saveInput.Namespace,
		MemoryType:       saveInput.MemoryType,
		Category:         saveInput.Category,
		CanonicalKey:     saveInput.CanonicalKey,
		ValueType:        saveInput.ValueType,
		ValueJSON:        saveInput.ValueJSON,
		DisplayValue:     saveInput.DisplayValue,
		SourceMessageID:  saveInput.SourceMessageID,
		Content:          saveInput.Content,
		Summary:          saveInput.Summary,
		Confidence:       candidate.Confidence,
		Importance:       saveInput.Importance,
		Status:           domain.MemoryStatusPending,
		ExtractionMethod: saveInput.ExtractionMethod,
		CreatedBy:        preferenceCandidateSystemActor,
		UpdatedBy:        preferenceCandidateSystemActor,
		CreateTime:       now,
		UpdateTime:       now,
	}, nil
}

func mapMemoryItemToPreferenceCandidate(item domain.MemoryItem) (PreferenceCandidate, error) {
	candidate := PreferenceCandidate{
		ID:               strings.TrimSpace(item.ID),
		ScopeType:        strings.TrimSpace(item.ScopeType),
		MemoryType:       strings.TrimSpace(item.MemoryType),
		CanonicalKey:     strings.TrimSpace(item.CanonicalKey),
		Summary:          strings.TrimSpace(item.Summary),
		Content:          strings.TrimSpace(item.Content),
		SourceMessageID:  strings.TrimSpace(item.SourceMessageID),
		ExtractionMethod: strings.TrimSpace(item.ExtractionMethod),
		Confidence:       item.Confidence,
		Status:           strings.TrimSpace(item.Status),
	}
	if err := ValidatePreferenceCandidate(candidate); err != nil {
		return PreferenceCandidate{}, err
	}
	return candidate, nil
}

func buildCandidateGateDecision(item domain.MemoryItem) (governance.GateDecision, error) {
	normalized := governance.NormalizeSaveExplicitMemoryInput(SaveExplicitMemoryInput{
		UserID:           item.UserID,
		ScopeType:        item.ScopeType,
		ScopeID:          item.ScopeID,
		Namespace:        item.Namespace,
		MemoryType:       item.MemoryType,
		Category:         item.Category,
		CanonicalKey:     item.CanonicalKey,
		ValueType:        item.ValueType,
		ValueJSON:        item.ValueJSON,
		DisplayValue:     item.DisplayValue,
		SourceMessageID:  item.SourceMessageID,
		Content:          item.Content,
		Summary:          item.Summary,
		Importance:       item.Importance,
		ExtractionMethod: item.ExtractionMethod,
		ExpiresAt:        item.ExpiresAt,
	})
	return governance.EvaluateExplicitMemoryGate(normalized)
}

func activatePendingPreferenceItem(item domain.MemoryItem, userID string, now time.Time, supersedesID string) domain.MemoryItem {
	item.Status = domain.MemoryStatusActive
	item.SupersedesID = strings.TrimSpace(supersedesID)
	item.UpdatedBy = strings.TrimSpace(userID)
	item.UpdateTime = now
	item.LastConfirmedAt = &now
	return item
}

func rejectPendingPreferenceItem(item domain.MemoryItem, userID string, now time.Time) domain.MemoryItem {
	item.Status = domain.MemoryStatusRejected
	item.UpdatedBy = strings.TrimSpace(userID)
	item.UpdateTime = now
	return item
}

func supersedeActivePreferenceItem(item domain.MemoryItem, userID string, now time.Time) domain.MemoryItem {
	item.Status = domain.MemoryStatusSuperseded
	item.UpdatedBy = strings.TrimSpace(userID)
	item.UpdateTime = now
	return item
}

func mergeConfirmedPreferenceIntoExisting(existing domain.MemoryItem, candidate domain.MemoryItem, userID string, now time.Time) domain.MemoryItem {
	existing.LastConfirmedAt = &now
	existing.UpdatedBy = strings.TrimSpace(userID)
	existing.UpdateTime = now
	if len(strings.TrimSpace(candidate.Summary)) > len(strings.TrimSpace(existing.Summary)) {
		existing.Summary = candidate.Summary
	}
	if len(strings.TrimSpace(candidate.Content)) > len(strings.TrimSpace(existing.Content)) {
		existing.Content = candidate.Content
	}
	if len(strings.TrimSpace(candidate.DisplayValue)) > len(strings.TrimSpace(existing.DisplayValue)) {
		existing.DisplayValue = candidate.DisplayValue
	}
	if len(strings.TrimSpace(candidate.ValueJSON)) > len(strings.TrimSpace(existing.ValueJSON)) {
		existing.ValueJSON = candidate.ValueJSON
	}
	if candidate.Importance > existing.Importance {
		existing.Importance = candidate.Importance
	}
	if strings.TrimSpace(existing.Namespace) == "" {
		existing.Namespace = candidate.Namespace
	}
	if strings.TrimSpace(existing.Category) == "" {
		existing.Category = candidate.Category
	}
	if strings.TrimSpace(existing.ValueType) == "" {
		existing.ValueType = candidate.ValueType
	}
	if strings.TrimSpace(existing.ExtractionMethod) == "" {
		existing.ExtractionMethod = candidate.ExtractionMethod
	}
	return existing
}

func nextPreferenceCandidateID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate preference candidate id", err)
	}
	return fmt.Sprintf("%d", id), nil
}
