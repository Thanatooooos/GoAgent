package longtermmemory

import (
	"context"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/exception"
)

func (s *MemoryService) saveExplicitMemoryWithRepo(
	ctx context.Context,
	repo port.MemoryItemRepository,
	input SaveExplicitMemoryInput,
) (domain.MemoryItem, error) {
	normalized := normalizeSaveExplicitMemoryInput(input)
	decision, err := evaluateExplicitMemoryGate(normalized)
	if err != nil {
		return domain.MemoryItem{}, err
	}

	candidate := s.buildMemoryItemCandidate(normalized)
	resolution, err := detectMemoryConflict(ctx, repo, s.now, decision, candidate)
	if err != nil {
		return domain.MemoryItem{}, err
	}

	switch resolution.Action {
	case GateDecisionUpdate, GateDecisionMerge:
		if resolution.UpdatedExisting == nil {
			return domain.MemoryItem{}, exception.NewServiceException("updated memory item is required", nil)
		}
		updated, err := repo.Update(ctx, *resolution.UpdatedExisting)
		if err != nil {
			return domain.MemoryItem{}, exception.NewServiceException("failed to update memory item", err)
		}
		return updated, nil
	case GateDecisionIgnore:
		if resolution.Existing != nil {
			return *resolution.Existing, nil
		}
		return domain.MemoryItem{}, nil
	case GateDecisionPending:
		return domain.MemoryItem{}, nil
	default:
		if resolution.UpdatedExisting != nil {
			if _, err := repo.Update(ctx, *resolution.UpdatedExisting); err != nil {
				return domain.MemoryItem{}, exception.NewServiceException("failed to supersede memory item", err)
			}
		}
		if resolution.CreateCandidate == nil {
			return domain.MemoryItem{}, exception.NewServiceException("create candidate is required", nil)
		}
		id, err := nextMemoryItemID()
		if err != nil {
			return domain.MemoryItem{}, err
		}
		toCreate := *resolution.CreateCandidate
		toCreate.ID = id
		created, err := repo.Create(ctx, toCreate)
		if err != nil {
			return domain.MemoryItem{}, exception.NewServiceException("failed to create memory item", err)
		}
		return created, nil
	}
}

func (s *MemoryService) buildMemoryItemCandidate(input normalizedSaveInput) domain.MemoryItem {
	now := s.now()
	summary := input.Summary
	if summary == "" {
		summary = summarizeMemoryText(input.Content, defaultMemorySummaryRunes)
	}
	return domain.MemoryItem{
		UserID:           input.UserID,
		ScopeType:        input.ScopeType,
		ScopeID:          input.ScopeID,
		Namespace:        input.Namespace,
		MemoryType:       input.MemoryType,
		Category:         input.Category,
		CanonicalKey:     input.CanonicalKey,
		ValueType:        normalizeValueType(input.ValueType),
		ValueJSON:        input.ValueJSON,
		DisplayValue:     input.DisplayValue,
		SourceMessageID:  input.SourceMessageID,
		Content:          input.Content,
		Summary:          summary,
		Confidence:       1,
		Importance:       input.Importance,
		Status:           domain.MemoryStatusActive,
		LastConfirmedAt:  &now,
		ExpiresAt:        input.ExpiresAt,
		ExtractionMethod: input.ExtractionMethod,
		CreatedBy:        input.UserID,
		UpdatedBy:        input.UserID,
		CreateTime:       now,
		UpdateTime:       now,
	}
}
