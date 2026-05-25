package governance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	memorytypes "local/rag-project/internal/app/rag/service/longtermmemory/types"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/exception"
)

func SaveExplicitMemoryWithRepo(
	ctx context.Context,
	repo port.MemoryItemRepository,
	input memorytypes.SaveExplicitMemoryInput,
	now func() time.Time,
) (domain.MemoryItem, error) {
	if repo == nil {
		return domain.MemoryItem{}, exception.NewServiceException("memory item repository is required", nil)
	}
	if now == nil {
		now = time.Now
	}

	normalized := NormalizeSaveExplicitMemoryInput(input)
	decision, err := EvaluateExplicitMemoryGate(normalized)
	if err != nil {
		return domain.MemoryItem{}, err
	}

	candidate := BuildMemoryItemCandidate(normalized, now())
	resolution, err := DetectMemoryConflict(ctx, repo, now, decision, candidate)
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

func BuildMemoryItemCandidate(input NormalizedSaveInput, now time.Time) domain.MemoryItem {
	summary := input.Summary
	if summary == "" {
		summary = summarizeMemoryText(input.Content, memorytypes.DefaultMemorySummaryRunes)
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

func nextMemoryItemID() (string, error) {
	id, err := distributedid.NextID()
	if err != nil {
		return "", exception.NewServiceException("failed to generate memory item id", err)
	}
	return fmt.Sprintf("%d", id), nil
}

func summarizeMemoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if value == "" {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = memorytypes.DefaultMemorySummaryRunes
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}
