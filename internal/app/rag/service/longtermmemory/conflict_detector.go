package longtermmemory

import (
	"context"
	"strings"
	"time"
	"unicode"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/exception"
)

func detectMemoryConflict(
	ctx context.Context,
	repo port.MemoryItemRepository,
	now func() time.Time,
	decision GateDecision,
	candidate domain.MemoryItem,
) (ConflictResolution, error) {
	if repo == nil {
		return ConflictResolution{}, exception.NewServiceException("memory item repository is required", nil)
	}
	if decision.Spec == nil || strings.TrimSpace(decision.Input.CanonicalKey) == "" {
		return ConflictResolution{
			Action:          GateDecisionCreate,
			CreateCandidate: &candidate,
		}, nil
	}

	active, err := repo.List(ctx, port.MemoryItemListFilter{
		UserID:        strings.TrimSpace(decision.Input.UserID),
		ScopeTypes:    []string{decision.Input.ScopeType},
		ScopeIDs:      normalizeScopeIDs(decision.Input.ScopeType, decision.Input.ScopeID),
		CanonicalKeys: []string{decision.Input.CanonicalKey},
		Statuses:      []string{domain.MemoryStatusActive},
		ListOptions: port.ListOptions{
			Limit: 8,
		},
	})
	if err != nil {
		return ConflictResolution{}, exception.NewServiceException("failed to load active memory items", err)
	}

	if decision.Input.MemoryType == domain.MemoryTypeFeedback {
		for idx := range active {
			if memoryItemsEquivalent(active[idx], candidate) {
				return ConflictResolution{
					Action:   GateDecisionIgnore,
					Existing: &active[idx],
				}, nil
			}
		}
		return ConflictResolution{
			Action:          GateDecisionCreate,
			CreateCandidate: &candidate,
		}, nil
	}

	if decision.Spec.Cardinality == MemoryCardinalitySingle {
		if len(active) == 0 {
			return ConflictResolution{
				Action:          GateDecisionCreate,
				CreateCandidate: &candidate,
			}, nil
		}
		existing := active[0]
		if memoryItemsEquivalent(existing, candidate) {
			updated := refreshExistingMemory(existing, candidate, now())
			return ConflictResolution{
				Action:          GateDecisionUpdate,
				Existing:        &existing,
				UpdatedExisting: &updated,
			}, nil
		}
		superseded := markMemorySuperseded(existing, candidate.UpdatedBy, now())
		created := candidate
		created.SupersedesID = existing.ID
		return ConflictResolution{
			Action:          GateDecisionCreate,
			Existing:        &existing,
			UpdatedExisting: &superseded,
			CreateCandidate: &created,
		}, nil
	}

	for idx := range active {
		if memoryItemsEquivalent(active[idx], candidate) {
			updated := refreshExistingMemory(active[idx], candidate, now())
			return ConflictResolution{
				Action:          GateDecisionMerge,
				Existing:        &active[idx],
				UpdatedExisting: &updated,
			}, nil
		}
	}
	return ConflictResolution{
		Action:          GateDecisionCreate,
		CreateCandidate: &candidate,
	}, nil
}

func normalizeScopeIDs(scopeType string, scopeID string) []string {
	if strings.TrimSpace(scopeType) != domain.MemoryScopeKB || strings.TrimSpace(scopeID) == "" {
		return nil
	}
	return []string{strings.TrimSpace(scopeID)}
}

func memoryItemsEquivalent(left domain.MemoryItem, right domain.MemoryItem) bool {
	if comparableMemoryValue(left.ValueType, left.ValueJSON) != "" && comparableMemoryValue(right.ValueType, right.ValueJSON) != "" {
		return comparableMemoryValue(left.ValueType, left.ValueJSON) == comparableMemoryValue(right.ValueType, right.ValueJSON)
	}
	if comparableDisplayValue(left.DisplayValue) != "" && comparableDisplayValue(right.DisplayValue) != "" {
		return comparableDisplayValue(left.DisplayValue) == comparableDisplayValue(right.DisplayValue)
	}
	return comparableDisplayValue(left.Content) == comparableDisplayValue(right.Content)
}

func comparableMemoryValue(valueType string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch normalizeValueType(valueType) {
	case domain.MemoryValueTypeEnum, domain.MemoryValueTypeBoolean:
		return comparableDisplayValue(strings.ToLower(value))
	case domain.MemoryValueTypeJSON:
		return collapseInnerWhitespace(value)
	default:
		return comparableDisplayValue(value)
	}
}

func comparableDisplayValue(value string) string {
	return strings.ToLower(collapseInnerWhitespace(value))
}

func collapseInnerWhitespace(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r)
	}), " ")
}
